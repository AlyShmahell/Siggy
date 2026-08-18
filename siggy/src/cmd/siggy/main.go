package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"siggy/src/internal/config"
	"siggy/src/internal/graph"
	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/loop"
	"siggy/src/internal/mcp"
	"siggy/src/internal/subagent"
	"siggy/src/internal/tools"
	"siggy/src/internal/tui"
	"siggy/src/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "siggy:", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	prompt := flag.String("p", "", "headless prompt (no TUI)")
	yes := flag.Bool("yes", false, "auto-approve write/shell/network tools")
	resume := flag.String("resume", "", "resume session id")
	plan := flag.Bool("plan", false, "start in plan mode")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Value)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AutoApprove = *yes
	if *plan {
		cfg.Mode = string(harness.ModePlan)
	}

	h, err := harness.New(cfg.Workspace, cfg.Home, cfg.AutoApprove)
	if err != nil {
		return err
	}
	defer h.Session.Close()
	h.Mode = harness.ParseMode(cfg.Mode)

	if *resume != "" {
		sess, err := harness.OpenSession(cfg.Home, *resume)
		if err != nil {
			return err
		}
		_ = h.Session.Close()
		h.Session = sess
	}

	prov := cfg.Active()
	var client llm.Client
	if os.Getenv("SIGGY_FAKE_LLM") == "1" {
		client = &llm.Scripted{Steps: []llm.ScriptedStep{{Text: "fake llm ready"}}}
	} else {
		if prov.APIKey == "" && !strings.Contains(prov.URL, "localhost") && !strings.Contains(prov.URL, "127.0.0.1") {
			fmt.Fprintln(os.Stderr, "warning: provider API key is empty")
		}
		client = llm.NewHTTP(prov.URL, prov.APIKey, cfg.Model)
	}

	reg := tools.Builtins(h, nil)
	mgr := &subagent.Manager{Parent: h, Client: client, Tools: reg}
	reg.Register(tools.NewDelegate(mgr))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	clients, err := mcp.Register(ctx, cfg.MCP, reg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: mcp: %v\n", err)
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

	var g *graph.Graph
	if *resume != "" {
		g = graph.FromSession(client, reg, h)
	} else {
		g = graph.New(client, reg, h, "")
	}

	if *prompt != "" {
		return runHeadless(ctx, g, *prompt, *yes)
	}
	return tui.Run(g, h, cfg)
}

func runHeadless(ctx context.Context, g *graph.Graph, prompt string, yes bool) error {
	return g.Run(ctx, prompt, func(ev loop.Event) {
		switch ev.Kind {
		case loop.KindText:
			fmt.Fprint(os.Stdout, ev.Text)
		case loop.KindToolStart:
			fmt.Fprintf(os.Stderr, "\n▸ %s %s\n", ev.Tool, ev.Args)
		case loop.KindToolEnd:
			fmt.Fprintf(os.Stderr, "%s\n", ev.Text)
		case loop.KindApproval:
			d := harness.Deny
			if yes {
				d = harness.AllowOnce
			}
			if ev.Approval != nil {
				select {
				case ev.Approval.Reply <- d:
				default:
				}
			}
			if !yes {
				fmt.Fprintf(os.Stderr, "denied %s (pass --yes to allow)\n", ev.Tool)
			}
		case loop.KindError:
			if ev.Err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", ev.Err)
			}
		case loop.KindDone:
			fmt.Fprintln(os.Stdout)
		}
	})
}
