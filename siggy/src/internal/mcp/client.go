package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"siggy/src/internal/version"
)

type Client struct {
	name       string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	mu         sync.Mutex
	id         atomic.Int64
	pending    map[int64]chan rpcResp
	readerDone chan struct{}
}

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func Start(ctx context.Context, name, command string, args, env []string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append(os.Environ(), env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{
		name:       name,
		cmd:        cmd,
		stdin:      stdin,
		pending:    map[int64]chan rpcResp{},
		readerDone: make(chan struct{}),
	}
	go c.readLoop(stdout)
	if _, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "siggy", "version": version.Value},
	}); err != nil {
		_ = c.Close()
		return nil, err
	}
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Name() string { return c.name }

func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

func (c *Client) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var parsed any
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", err
	}
	raw, err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": parsed})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	_ = c.stdin.Close()
	c.mu.Unlock()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.cmd != nil {
		return c.cmd.Wait()
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.id.Add(1)
	ch := make(chan rpcResp, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.write(rpcReq{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) notify(method string, params any) error {
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	return c.writeRaw(raw)
}

func (c *Client) write(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.writeRaw(raw)
}

func (c *Client) writeRaw(raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n%s", len(raw), raw)
	return err
}

func (c *Client) readLoop(r io.Reader) {
	defer close(c.readerDone)
	br := bufio.NewReader(r)
	for {
		n, err := readHeaders(br)
		if err != nil {
			return
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(br, body); err != nil {
			return
		}
		var resp rpcResp
		if err := json.Unmarshal(body, &resp); err != nil || resp.ID == nil {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[*resp.ID]
		if ok {
			delete(c.pending, *resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

func readHeaders(r *bufio.Reader) (int, error) {
	length := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		if line == "\r\n" || line == "\n" {
			if length == 0 {
				return 0, fmt.Errorf("missing Content-Length")
			}
			return length, nil
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "content-length:") {
			v := strings.TrimSpace(line[len("Content-Length:"):])
			v = strings.TrimSpace(strings.TrimSuffix(v, "\r\n"))
			v = strings.TrimSpace(strings.TrimSuffix(v, "\n"))
			n, err := strconv.Atoi(v)
			if err != nil {
				return 0, err
			}
			length = n
		}
	}
}
