package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPStreamTextAndTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"there\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"c1\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"a\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := NewHTTP(srv.URL, "k", "m")
	ch, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	text, calls, err := Collect(ch)
	if err != nil {
		t.Fatal(err)
	}
	if text != "hi there" {
		t.Fatalf("text = %q", text)
	}
	if len(calls) != 1 || calls[0].Name != "read_file" {
		t.Fatalf("calls = %#v", calls)
	}
	var args map[string]string
	if err := json.Unmarshal(calls[0].Args, &args); err != nil || args["path"] != "a" {
		t.Fatalf("args = %s %v", calls[0].Args, err)
	}
}

func TestHTTPStreamUsage(t *testing.T) {
	var sawOpts bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if opts, ok := body["stream_options"].(map[string]any); ok && opts["include_usage"] == true {
			sawOpts = true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := NewHTTP(srv.URL, "k", "m")
	ch, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	var got Usage
	for chunk := range ch {
		if chunk.Usage.Total > 0 {
			got = chunk.Usage
		}
	}
	if !sawOpts {
		t.Fatal("missing stream_options.include_usage")
	}
	if got.Prompt != 12 || got.Completion != 3 || got.Total != 15 {
		t.Fatalf("usage = %#v", got)
	}
}

func TestFake(t *testing.T) {
	s := &Scripted{Steps: []ScriptedStep{{Text: "ok"}}}
	ch, err := s.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := Collect(ch)
	if err != nil || text != "ok" {
		t.Fatalf("%q %v", text, err)
	}
	if s.Ping(context.Background()) != "ok" {
		t.Fatal("scripted ping")
	}
}

func TestHTTPPingModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1"},{"id":"mini"}]}`))
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL, "k", "gpt-4.1")
	if got := c.Ping(context.Background()); got != "ok" {
		t.Fatalf("known model: %q", got)
	}
	c.Model = "made-up"
	if got := c.Ping(context.Background()); got != "err" {
		t.Fatalf("unknown model: %q", got)
	}
}

func TestHTTPPingMissingEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL, "k", "gpt-4.1")
	if got := c.Ping(context.Background()); got != "…" {
		t.Fatalf("404 should be unverified, got %q", got)
	}
}
