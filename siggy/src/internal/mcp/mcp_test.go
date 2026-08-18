package mcp

import (
	"bufio"
	"strconv"
	"strings"
	"testing"
)

func TestReadHeaders(t *testing.T) {
	raw := "Content-Length: 2\r\n\r\n{}"
	n, err := readHeaders(bufio.NewReader(strings.NewReader(raw)))
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("My Server!"); got != "my_server_" {
		t.Fatalf("got %q", got)
	}
}

func TestContentLengthParse(t *testing.T) {
	body := `{"ok":true}`
	raw := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	n, err := readHeaders(bufio.NewReader(strings.NewReader(raw)))
	if err != nil || n != len(body) {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
