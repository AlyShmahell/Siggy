package tui

import (
	"bytes"
	"io"
	"testing"
)

func TestConsumeShiftEnter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"kitty", "\x1b[13;2u", "\n"},
		{"xterm", "\x1b[27;2;13~", "\n"},
		{"tilde", "\x1b[13;2~", "\n"},
		{"cr stays", "hello\r", "hello\r"},
		{"mouse sgr", "\x1b[<0;10;5M", "\x1b[<0;10;5M"},
		{"arrow", "\x1b[A", "\x1b[A"},
		{"mixed", "ab\x1b[13;2ucd", "ab\ncd"},
	}
	for _, tc := range cases {
		out, rest := consumeShiftEnter([]byte(tc.in))
		if len(rest) != 0 {
			t.Fatalf("%s: leftover %q", tc.name, rest)
		}
		if string(out) != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, out, tc.want)
		}
	}
}

func TestConsumeShiftEnterIncomplete(t *testing.T) {
	out, rest := consumeShiftEnter([]byte("\x1b[13;2"))
	if len(out) != 0 || string(rest) != "\x1b[13;2" {
		t.Fatalf("out=%q rest=%q", out, rest)
	}
	out, rest = consumeShiftEnter([]byte("\x1b[13;2u"))
	if string(out) != "\n" || len(rest) != 0 {
		t.Fatalf("complete split: out=%q rest=%q", out, rest)
	}
}

func TestShiftEnterReader(t *testing.T) {
	r := newShiftEnterReader(bytes.NewReader([]byte("x\x1b[13;2uy\r\x1b[<0;1;1M")))
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	want := "x\ny\r\x1b[<0;1;1M"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestShiftEnterReaderSplitCSI(t *testing.T) {
	pr, pw := io.Pipe()
	r := newShiftEnterReader(pr)
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 32)
		n, err := r.Read(buf)
		if err != nil {
			done <- nil
			return
		}
		done <- buf[:n]
	}()
	_, _ = pw.Write([]byte("\x1b[13;2"))
	_, _ = pw.Write([]byte("u"))
	_ = pw.Close()
	got := <-done
	if string(got) != "\n" {
		t.Fatalf("split CSI got %q", got)
	}
}
