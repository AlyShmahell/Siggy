package tui

import (
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

const (
	keyEnable  = "\x1b[>4;2m\x1b[>1u"
	keyDisable = "\x1b[>4;0m\x1b[<u"
)

const (
	kittyUp    = 57352
	kittyDown  = 57353
	kittyRight = 57354
	kittyLeft  = 57355
)

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
	r     io.Reader
	buf   []byte
	out   []byte
	mouse mouseCell
}

type mouseCell struct {
	x, y int
	set  bool
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
			decoded, rest := consumeCSI(s.buf, &s.mouse)
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
	return consumeCSI(buf, nil)
}

func consumeCSI(buf []byte, mouse *mouseCell) (out, rest []byte) {
	i := 0
	for i < len(buf) {
		if buf[i] != 0x1b {
			out = append(out, buf[i])
			i++
			continue
		}
		if i+1 >= len(buf) {
			return out, buf[i:]
		}
		if buf[i+1] != '[' {
			out = append(out, buf[i])
			i++
			continue
		}
		end := i + 2
		if end < len(buf) {
			switch buf[end] {
			case '?', '<', '=', '>':
				end++
			}
		}
		for end < len(buf) && buf[end] >= 0x30 && buf[end] <= 0x3f {
			end++
		}
		if end >= len(buf) {
			return out, buf[i:]
		}
		if buf[end] < 0x40 || buf[end] > 0x7e {
			i = end
			continue
		}
		seq := buf[i : end+1]
		if skipMouseMotion(seq, mouse) {
			i = end + 1
			continue
		}
		out = append(out, rewriteCSI(seq)...)
		i = end + 1
	}
	return out, nil
}

func parseSGRMouse(seq []byte) (btn, x, y int, motion, ok bool) {
	if len(seq) < 8 || seq[0] != 0x1b || seq[1] != '[' || seq[2] != '<' {
		return 0, 0, 0, false, false
	}
	final := seq[len(seq)-1]
	if final != 'M' && final != 'm' {
		return 0, 0, 0, false, false
	}
	params := parseCSIParams(seq[3 : len(seq)-1])
	if len(params) < 3 {
		return 0, 0, 0, false, false
	}
	btn, x, y = params[0], params[1], params[2]
	motion = btn&32 != 0 && btn&64 == 0
	return btn, x, y, motion, true
}

func skipMouseMotion(seq []byte, mouse *mouseCell) bool {
	_, x, y, motion, ok := parseSGRMouse(seq)
	if !ok {
		return false
	}
	if mouse != nil {
		if motion && mouse.set && mouse.x == x && mouse.y == y {
			return true
		}
		mouse.x, mouse.y, mouse.set = x, y, true
	}
	return false
}

func rewriteCSI(seq []byte) []byte {
	if len(seq) < 3 || seq[0] != 0x1b || seq[1] != '[' {
		return seq
	}
	final := seq[len(seq)-1]
	body := seq[2 : len(seq)-1]
	if len(body) > 0 {
		switch body[0] {
		case '?', '<', '=', '>':
			return seq
		}
	}
	params := parseCSIParams(body)
	switch final {
	case 'u':
		if rewritten := rewriteCSIu(params); rewritten != nil {
			return rewritten
		}
	case '~':
		if rewritten := rewriteCSItilde(params); rewritten != nil {
			return rewritten
		}
	}
	return seq
}

func parseCSIParams(body []byte) []int {
	if len(body) == 0 {
		return nil
	}
	parts := strings.Split(string(body), ";")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if i := strings.IndexByte(p, ':'); i >= 0 {
			p = p[:i]
		}
		if p == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

func rewriteCSIu(params []int) []byte {
	if len(params) == 0 {
		return nil
	}
	code := params[0]
	mod := 1
	if len(params) > 1 && params[1] > 0 {
		mod = params[1]
	}
	if code == 13 && (mod-1)&1 != 0 {
		return []byte{'\n'}
	}
	if b, ok := ctrlLetter(code, mod); ok {
		return []byte{b}
	}
	if b := rewriteArrow(code, mod); b != nil {
		return b
	}
	return rewritePrintable(code, mod)
}

func rewriteCSItilde(params []int) []byte {
	if len(params) >= 1 && (params[0] == 200 || params[0] == 201) {
		return nil
	}
	if len(params) < 3 || params[0] != 27 {
		if len(params) >= 1 && params[0] == 13 {
			mod := 1
			if len(params) > 1 && params[1] > 0 {
				mod = params[1]
			}
			if (mod-1)&1 != 0 {
				return []byte{'\n'}
			}
		}
		return nil
	}
	mod := params[1]
	if mod == 0 {
		mod = 1
	}
	code := params[2]
	if code == 13 && (mod-1)&1 != 0 {
		return []byte{'\n'}
	}
	if b, ok := ctrlLetter(code, mod); ok {
		return []byte{b}
	}
	if b := rewriteArrow(code, mod); b != nil {
		return b
	}
	return rewritePrintable(code, mod)
}

func rewritePrintable(code, mod int) []byte {
	if (mod-1)&6 != 0 {
		return nil
	}
	if code < 32 || code == 127 {
		return nil
	}
	r := rune(code)
	if r == utf8.RuneError || !utf8.ValidRune(r) {
		return nil
	}
	return []byte(string(r))
}

func ctrlLetter(code, mod int) (byte, bool) {
	if (mod-1)&4 == 0 {
		return 0, false
	}
	if code >= 'a' && code <= 'z' {
		return byte(code & 31), true
	}
	if code >= 'A' && code <= 'Z' {
		return byte(code & 31), true
	}
	return 0, false
}

func rewriteArrow(code, mod int) []byte {
	var letter byte
	switch code {
	case kittyUp:
		letter = 'A'
	case kittyDown:
		letter = 'B'
	case kittyRight:
		letter = 'C'
	case kittyLeft:
		letter = 'D'
	default:
		return nil
	}
	if mod <= 1 {
		return []byte{0x1b, '[', letter}
	}
	return []byte("\x1b[1;" + strconv.Itoa(mod) + string(letter))
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
