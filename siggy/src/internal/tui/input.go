package tui

import (
	"bytes"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

const (
	keyEnable  = "\x1b[>4;2m\x1b[>1u"
	keyDisable = "\x1b[>4;0m\x1b[<u"
)

var shiftEnterSeqs = [][]byte{
	[]byte("\x1b[13;2u"),
	[]byte("\x1b[27;2;13~"),
	[]byte("\x1b[13;2~"),
}

func enableEnhancedKeys() tea.Cmd {
	return func() tea.Msg {
		_, _ = os.Stdout.WriteString(keyEnable)
		return nil
	}
}

func disableEnhancedKeys() {
	_, _ = os.Stdout.WriteString(keyDisable)
}

type shiftEnterReader struct {
	r   io.Reader
	buf []byte
	out []byte
}

func newShiftEnterReader(r io.Reader) *shiftEnterReader {
	return &shiftEnterReader{r: r}
}

func (s *shiftEnterReader) Read(p []byte) (int, error) {
	for {
		if len(s.out) > 0 {
			n := copy(p, s.out)
			s.out = s.out[n:]
			return n, nil
		}
		tmp := make([]byte, max(len(p), 64))
		n, err := s.r.Read(tmp)
		if n > 0 {
			s.buf = append(s.buf, tmp[:n]...)
			decoded, rest := consumeShiftEnter(s.buf)
			s.buf = rest
			s.out = append(s.out, decoded...)
		}
		if len(s.out) > 0 {
			continue
		}
		if err != nil {
			if len(s.buf) > 0 {
				s.out = append(s.out, s.buf...)
				s.buf = nil
				continue
			}
			return 0, err
		}
		if n == 0 {
			return 0, nil
		}
	}
}

func consumeShiftEnter(buf []byte) (out, rest []byte) {
	i := 0
	for i < len(buf) {
		if buf[i] != 0x1b {
			out = append(out, buf[i])
			i++
			continue
		}
		if ok, n := matchShiftEnter(buf[i:]); ok {
			out = append(out, '\n')
			i += n
			continue
		}
		if incompleteShiftEnter(buf[i:]) {
			return out, buf[i:]
		}
		out = append(out, buf[i])
		i++
	}
	return out, nil
}

func matchShiftEnter(b []byte) (bool, int) {
	for _, seq := range shiftEnterSeqs {
		if len(b) >= len(seq) && bytes.Equal(b[:len(seq)], seq) {
			return true, len(seq)
		}
	}
	return false, 0
}

func incompleteShiftEnter(b []byte) bool {
	if len(b) == 0 || b[0] != 0x1b {
		return false
	}
	for _, seq := range shiftEnterSeqs {
		if len(b) < len(seq) && bytes.Equal(b, seq[:len(b)]) {
			return true
		}
	}
	return false
}

func programInput() (io.Reader, func()) {
	in := io.Reader(os.Stdin)
	restore := func() {}
	if term.IsTerminal(os.Stdin.Fd()) {
		state, err := term.MakeRaw(os.Stdin.Fd())
		if err == nil {
			restore = func() { _ = term.Restore(os.Stdin.Fd(), state) }
		}
	}
	return newShiftEnterReader(in), restore
}
