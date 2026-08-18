package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type reqBody struct {
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
		Name    string `json:"name"`
	} `json:"messages"`
}

func main() {
	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	http.HandleFunc("/chat/completions", handle)
	http.HandleFunc("/v1/chat/completions", handle)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	log.Printf("mock openai on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handle(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body reqBody
	_ = json.Unmarshal(raw, &body)
	user := lastUser(body)
	lastRole, lastName := lastMsg(body)

	var payload string
	switch {
	case lastRole == "tool":
		payload = textChunk("ok: " + lastName)
	case strings.Contains(user, "delegate"):
		payload = toolChunk("c-del", "delegate", `{"agent":"explore","task":"map the workspace"}`)
	case strings.Contains(user, "write"):
		payload = toolChunk("c-w", "write_file", `{"path":"smoke.txt","content":"from-agent"}`)
	default:
		payload = toolChunk("c-ls", "list_dir", `{}`)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte(payload))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

func lastUser(body reqBody) string {
	for i := len(body.Messages) - 1; i >= 0; i-- {
		if body.Messages[i].Role == "user" {
			return stringify(body.Messages[i].Content)
		}
	}
	return ""
}

func lastMsg(body reqBody) (role, name string) {
	if len(body.Messages) == 0 {
		return "", ""
	}
	m := body.Messages[len(body.Messages)-1]
	return m.Role, m.Name
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func textChunk(s string) string {
	esc, _ := json.Marshal(s)
	return "data: {\"choices\":[{\"delta\":{\"content\":" + string(esc) + "},\"finish_reason\":\"stop\"}]}\n\n"
}

func toolChunk(id, name, args string) string {
	escArgs, _ := json.Marshal(args)
	return "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"" + id + "\",\"type\":\"function\",\"function\":{\"name\":\"" + name + "\",\"arguments\":" + string(escArgs) + "}}]},\"finish_reason\":\"tool_calls\"}]}\n\n"
}
