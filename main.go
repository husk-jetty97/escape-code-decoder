// escseq reads terminal escape sequences, either as a literal argument
// using backslash notation or as raw bytes on stdin, and prints a
// line-by-line explanation of what each sequence does.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("usage: escseq 'literal string with \\x1b or \\e escapes'")
		fmt.Println("       ... | escseq          (reads raw bytes from stdin)")
		return
	}

	data, err := readInput()
	if err != nil {
		fmt.Fprintln(os.Stderr, "escseq:", err)
		os.Exit(1)
	}

	for _, e := range decode(data) {
		fmt.Printf("%6d  %-30s %s\n", e.offset, e.raw, e.desc)
	}
}

func readInput() ([]byte, error) {
	if len(os.Args) > 1 {
		return []byte(unescape(strings.Join(os.Args[1:], " "))), nil
	}
	return io.ReadAll(bufio.NewReader(os.Stdin))
}

// unescape turns common backslash notations (\e, \x1b, \033, \n, \t, \r)
// typed on a shell command line into the actual bytes they represent.
// Anything piped in on stdin arrives as real bytes already and skips this.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 'e', 'E':
			b.WriteByte(0x1b)
			i++
		case 'n':
			b.WriteByte('\n')
			i++
		case 'r':
			b.WriteByte('\r')
			i++
		case 't':
			b.WriteByte('\t')
			i++
		case '\\':
			b.WriteByte('\\')
			i++
		case 'x':
			if i+3 < len(s) {
				if n, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
					b.WriteByte(byte(n))
					i += 3
					continue
				}
			}
			b.WriteByte(s[i])
		case '0', '1', '2', '3':
			if i+3 < len(s) {
				if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
					b.WriteByte(byte(n))
					i += 3
					continue
				}
			}
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

type event struct {
	offset int
	raw    string
	desc   string
}

// decode walks the input byte by byte, turning each escape sequence into
// an event and collapsing runs of ordinary text into a single "literal
// text" event so the surrounding context isn't lost.
func decode(data []byte) []event {
	var events []event
	i := 0
	for i < len(data) {
		if data[i] == 0x1b {
			consumed, raw, desc := parseEscape(data[i:])
			events = append(events, event{i, raw, desc})
			i += consumed
			continue
		}
		start := i
		for i < len(data) && data[i] != 0x1b {
			i++
		}
		events = append(events, event{start, quoted(string(data[start:i])), "literal text"})
	}
	return events
}

func quoted(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return fmt.Sprintf("%q", truncate(s, 40))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func parseEscape(b []byte) (int, string, string) {
	if len(b) < 2 {
		return len(b), "ESC", "incomplete escape sequence at end of input"
	}
	switch b[1] {
	case '[':
		return parseCSI(b)
	case ']':
		return parseOSC(b)
	case 'P', 'X', '^', '_':
		return parseStringSeq(b)
	default:
		return parseSimple(b)
	}
}

// parseCSI handles ESC [ params intermediates final, as defined in ECMA-48.
// Parameter bytes are 0x30-0x3f, intermediate bytes are 0x20-0x2f, and the
// sequence ends at the first byte in 0x40-0x7e.
func parseCSI(b []byte) (int, string, string) {
	i := 2
	for i < len(b) && b[i] >= 0x30 && b[i] <= 0x3f {
		i++
	}
	params := string(b[2:i])
	interStart := i
	for i < len(b) && b[i] >= 0x20 && b[i] <= 0x2f {
		i++
	}
	inter := string(b[interStart:i])
	if i >= len(b) {
		return i, "ESC [" + params + inter, "incomplete CSI sequence"
	}
	final := b[i]
	i++
	raw := fmt.Sprintf("ESC [ %s%s%c", params, inter, final)
	return i, raw, describeCSI(params, final)
}

// parseOSC handles ESC ] Ps ; Pt, terminated by BEL or ESC \ (ST).
func parseOSC(b []byte) (int, string, string) {
	i := 2
	for i < len(b) {
		if b[i] == 0x07 {
			body := string(b[2:i])
			return i + 1, fmt.Sprintf("ESC ] %s BEL", body), describeOSC(body)
		}
		if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
			body := string(b[2:i])
			return i + 2, fmt.Sprintf("ESC ] %s ST", body), describeOSC(body)
		}
		i++
	}
	return len(b), "ESC ] " + string(b[2:]), "unterminated OSC sequence"
}

// parseStringSeq handles the other string-introducer sequences (DCS, SOS,
// PM, APC), which all share the "runs until ST" shape as OSC but carry
// device- or application-specific payloads that aren't decoded further.
func parseStringSeq(b []byte) (int, string, string) {
	kind := stringSeqKind[b[1]]
	i := 2
	for i < len(b) {
		if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
			body := string(b[2:i])
			raw := fmt.Sprintf("ESC %c %s ST", b[1], body)
			return i + 2, raw, fmt.Sprintf("%s string (application-specific), data %q", kind, truncate(body, 60))
		}
		i++
	}
	return len(b), fmt.Sprintf("ESC %c %s", b[1], string(b[2:])), fmt.Sprintf("unterminated %s string", kind)
}

// parseSimple handles the two-byte ESC sequences that take no parameters,
// such as ESC 7 (save cursor) or ESC c (full reset).
func parseSimple(b []byte) (int, string, string) {
	raw := fmt.Sprintf("ESC %c", b[1])
	if name, ok := simpleNames[b[1]]; ok {
		return 2, raw, name
	}
	return 2, raw, fmt.Sprintf("escape sequence ESC %c (unrecognized)", b[1])
}
