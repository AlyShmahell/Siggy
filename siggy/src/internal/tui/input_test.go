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
		{"ctrl+a kitty", "\x1b[97;5u", "\x01"},
		{"ctrl+x kitty", "\x1b[120;5u", "\x18"},
		{"ctrl+c kitty", "\x1b[99;5u", "\x03"},
		{"ctrl+v kitty", "\x1b[118;5u", "\x16"},
		{"ctrl+a xterm", "\x1b[27;5;97~", "\x01"},
		{"ctrl+A upper", "\x1b[65;5u", "\x01"},
		{"arrow up", "\x1b[57352u", "\x1b[A"},
		{"arrow down", "\x1b[57353u", "\x1b[B"},
		{"arrow right", "\x1b[57354u", "\x1b[C"},
		{"arrow left", "\x1b[57355u", "\x1b[D"},
		{"shift+up", "\x1b[57352;2u", "\x1b[1;2A"},
		{"cr stays", "hello\r", "hello\r"},
		{"mouse sgr", "\x1b[<0;10;5M", "\x1b[<0;10;5M"},
		{"arrow", "\x1b[A", "\x1b[A"},
		{"bracketed paste", "\x1b[200~sk-test\x1b[201~", "\x1b[200~sk-test\x1b[201~"},
		{"mixed", "ab\x1b[13;2ucd", "ab\ncd"},
		{"space kitty", "\x1b[32u", " "},
		{"space xterm", "\x1b[27;1;32~", " "},
		{"letter kitty", "\x1b[97u", "a"},
		{"hello space world", "hello\x1b[32uworld", "hello world"},
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

func TestConsumeShiftEnterSpaceNotHeld(t *testing.T) {
	out, rest := consumeShiftEnter([]byte("\x1b[ a"))
	if len(rest) != 0 {
		t.Fatalf("rest=%q", rest)
	}
	if string(out) != " a" {
		t.Fatalf("out=%q want %q", out, " a")
	}
}

func TestConsumeShiftEnterSplitSpaceCSI(t *testing.T) {
	out, rest := consumeShiftEnter([]byte("\x1b[32"))
	if len(out) != 0 || string(rest) != "\x1b[32" {
		t.Fatalf("hold space CSI: out=%q rest=%q", out, rest)
	}
	out, rest = consumeShiftEnter([]byte("\x1b[32u"))
	if string(out) != " " || len(rest) != 0 {
		t.Fatalf("complete space CSI: out=%q rest=%q", out, rest)
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

func TestShiftEnterReaderSplitCtrlA(t *testing.T) {
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
	_, _ = pw.Write([]byte("\x1b[97;5"))
	_, _ = pw.Write([]byte("u"))
	_ = pw.Close()
	got := <-done
	if string(got) != "\x01" {
		t.Fatalf("split ctrl+a got %q", got)
	}
}

func TestMouseMotionSameCellDropped(t *testing.T) {
	var mouse mouseCell
	seq := []byte("\x1b[<35;10;5M\x1b[<35;10;5M\x1b[<35;11;5M")
	out, rest := consumeCSI(seq, &mouse)
	if len(rest) != 0 {
		t.Fatalf("rest=%q", rest)
	}
	want := "\x1b[<35;10;5M\x1b[<35;11;5M"
	if string(out) != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestMouseClickNotDropped(t *testing.T) {
	var mouse mouseCell
	seq := []byte("\x1b[<0;10;5M\x1b[<0;10;5M")
	out, rest := consumeCSI(seq, &mouse)
	if len(rest) != 0 {
		t.Fatalf("rest=%q", rest)
	}
	if string(out) != string(seq) {
		t.Fatalf("clicks dropped: %q", out)
	}
}

func TestMouseWheelNotDropped(t *testing.T) {
	var mouse mouseCell
	seq := []byte("\x1b[<64;10;5M\x1b[<64;10;5M\x1b[<65;10;5M")
	out, rest := consumeCSI(seq, &mouse)
	if len(rest) != 0 {
		t.Fatalf("rest=%q", rest)
	}
	if string(out) != string(seq) {
		t.Fatalf("wheel dropped: %q", out)
	}
}
