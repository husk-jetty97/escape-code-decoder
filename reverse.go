package main

import (
	"fmt"
	"sort"
	"strings"
)

// emission is one sequence escseq knows how to explain, kept in the form
// needed to print it back out.
type emission struct {
	seq  string
	desc string
}

// emissions builds the list reverse mode searches. It is derived from the
// same tables the decoder uses so the two directions can't drift apart.
func emissions() []emission {
	var out []emission

	simple := make([]int, 0, len(simpleNames))
	for k := range simpleNames {
		simple = append(simple, int(k))
	}
	sort.Ints(simple)
	for _, k := range simple {
		out = append(out, emission{"\x1b" + string(rune(k)), simpleNames[byte(k)]})
	}

	// Only finals that do something useful with no parameters; ones like
	// DSR or window manipulation are meaningless without an argument.
	for _, f := range "ABCDHsu" {
		out = append(out, emission{"\x1b[" + string(f), csiFinal[byte(f)]})
	}

	for _, e := range []struct{ param, desc string }{
		{"0", "erase from cursor to end of screen"},
		{"1", "erase from start of screen to cursor"},
		{"2", "erase entire screen"},
		{"3", "erase entire screen and scrollback buffer"},
	} {
		out = append(out, emission{"\x1b[" + e.param + "J", e.desc})
	}
	for _, e := range []struct{ param, desc string }{
		{"0", "erase from cursor to end of line"},
		{"1", "erase from start of line to cursor"},
		{"2", "erase entire line"},
	} {
		out = append(out, emission{"\x1b[" + e.param + "K", e.desc})
	}

	attrs := make([]int, 0, len(sgrSimple))
	for n := range sgrSimple {
		attrs = append(attrs, n)
	}
	sort.Ints(attrs)
	for _, n := range attrs {
		out = append(out, emission{fmt.Sprintf("\x1b[%dm", n), "SGR: " + sgrSimple[n]})
	}
	for i, c := range colorNames {
		out = append(out,
			emission{fmt.Sprintf("\x1b[%dm", 30+i), "SGR: set foreground " + c},
			emission{fmt.Sprintf("\x1b[%dm", 40+i), "SGR: set background " + c},
			emission{fmt.Sprintf("\x1b[%dm", 90+i), "SGR: set foreground bright " + c},
			emission{fmt.Sprintf("\x1b[%dm", 100+i), "SGR: set background bright " + c},
		)
	}

	modes := make([]int, 0, len(decModes))
	for n := range decModes {
		modes = append(modes, n)
	}
	sort.Ints(modes)
	for _, n := range modes {
		out = append(out,
			emission{fmt.Sprintf("\x1b[?%dh", n), "set mode: " + decModes[n]},
			emission{fmt.Sprintf("\x1b[?%dl", n), "reset mode: " + decModes[n]},
		)
	}

	out = append(out,
		emission{"\x1b]0;TITLE\x07", "OSC: set icon name and window title (replace TITLE)"},
		emission{"\x1b]2;TITLE\x07", "OSC: set window title (replace TITLE)"},
	)
	return out
}

// reverse returns every emission whose description contains all the words
// in query, ignoring case. A word-by-word match lets "foreground red" find
// "set foreground red" without the user knowing the exact wording.
func reverse(query string) []emission {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	var matches []emission
	for _, e := range emissions() {
		desc := strings.ToLower(e.desc)
		ok := true
		for _, w := range words {
			if !strings.Contains(desc, w) {
				ok = false
				break
			}
		}
		if ok {
			matches = append(matches, e)
		}
	}
	return matches
}

// escapeForDisplay writes the escape byte as \x1b so the output can be
// pasted back into printf, echo -e, or escseq itself. BEL gets the same
// treatment because unescape has no \a.
func escapeForDisplay(seq string) string {
	s := strings.ReplaceAll(seq, "\x1b", `\x1b`)
	return strings.ReplaceAll(s, "\x07", `\x07`)
}
