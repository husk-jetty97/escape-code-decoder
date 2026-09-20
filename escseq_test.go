package main

import (
	"strings"
	"testing"
)

func TestUnescape(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`\x1b[31m`, "\x1b[31m"},
		{`\e[31m`, "\x1b[31m"},
		{`\033[31m`, "\x1b[31m"},
		{`a\nb\tc\rd`, "a\nb\tc\rd"},
		{`\\n`, `\n`},
		{`\x1g`, `\x1g`}, // not valid hex, left as-is
		{`\9`, `\9`},     // not a recognized escape, left as-is
	}
	for _, c := range cases {
		if got := unescape(c.in); got != c.want {
			t.Errorf("unescape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// find returns the first event whose desc contains substr, or fails the test.
func find(t *testing.T, events []event, substr string) event {
	t.Helper()
	for _, e := range events {
		if strings.Contains(e.desc, substr) {
			return e
		}
	}
	t.Fatalf("no event with desc containing %q in %+v", substr, events)
	return event{}
}

func TestDecodeCSI(t *testing.T) {
	events := decode([]byte("\x1b[1;31mERROR\x1b[0m"))
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if !strings.Contains(events[0].desc, "bold") || !strings.Contains(events[0].desc, "red") {
		t.Errorf("first event desc = %q, want bold+red", events[0].desc)
	}
	if events[1].desc != "literal text" || events[1].raw != `"ERROR"` {
		t.Errorf("second event = %+v, want literal text ERROR", events[1])
	}
	if !strings.Contains(events[2].desc, "reset all attributes") {
		t.Errorf("third event desc = %q, want reset", events[2].desc)
	}
	if events[0].offset != 0 || events[1].offset != 7 || events[2].offset != 12 {
		t.Errorf("offsets = %d,%d,%d, want 0,7,12", events[0].offset, events[1].offset, events[2].offset)
	}
}

func TestDecodeCSIVariants(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"cursor up", "\x1b[5A", "CUU"},
		{"erase to end of line", "\x1b[K", "erase from cursor to end of line"},
		{"erase entire screen", "\x1b[2J", "erase entire screen"},
		{"set dec private mode", "\x1b[?25h", "cursor visible"},
		{"reset dec private mode", "\x1b[?25l", "cursor visible"},
		{"set ansi mode", "\x1b[4h", "ANSI mode 4"},
		{"256 color foreground", "\x1b[38;5;200m", "256-color palette"},
		{"truecolor background", "\x1b[48;2;10;20;30m", "rgb(10,20,30)"},
		{"unrecognized final", "\x1b[9~", "unrecognized"},
		{"incomplete csi", "\x1b[31", "incomplete CSI"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			events := decode([]byte(c.in))
			if len(events) == 0 || !strings.Contains(events[0].desc, c.want) {
				t.Errorf("decode(%q) = %+v, want desc containing %q", c.in, events, c.want)
			}
		})
	}
}

func TestDecodeSGRResetAction(t *testing.T) {
	events := decode([]byte("\x1b[l"))
	if !strings.Contains(events[0].desc, "reset mode") {
		t.Errorf("desc = %q, want reset mode", events[0].desc)
	}
}

func TestDecodeOSC(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"window title BEL", "\x1b]0;my title\x07", `set icon name and window title ("my title")`},
		{"window title ST", "\x1b]2;other\x1b\\", `set window title ("other")`},
		{"unrecognized ps", "\x1b]999;data\x07", "OSC 999 (unrecognized)"},
		{"unterminated", "\x1b]0;dangling", "unterminated OSC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			events := decode([]byte(c.in))
			if len(events) == 0 {
				t.Fatalf("decode(%q) produced no events", c.in)
			}
			if got := events[0].desc; !strings.Contains(got, c.want) {
				t.Errorf("decode(%q) desc = %q, want containing %q", c.in, got, c.want)
			}
		})
	}
}

func TestDecodeStringSequences(t *testing.T) {
	cases := []struct {
		name, in, wantKind string
	}{
		{"DCS terminated", "\x1bPq data\x1b\\", "DCS string"},
		{"SOS terminated", "\x1bXhello\x1b\\", "SOS string"},
		{"PM terminated", "\x1b^hello\x1b\\", "PM string"},
		{"APC unterminated", "\x1b_hello", "unterminated APC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			events := decode([]byte(c.in))
			e := find(t, events, c.wantKind)
			if e.offset != 0 {
				t.Errorf("offset = %d, want 0", e.offset)
			}
		})
	}
}

func TestDecodeSimpleEscapes(t *testing.T) {
	events := decode([]byte("\x1b7\x1b8\x1bc"))
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if !strings.Contains(events[0].desc, "DECSC") {
		t.Errorf("events[0].desc = %q, want DECSC", events[0].desc)
	}
	if !strings.Contains(events[2].desc, "RIS") {
		t.Errorf("events[2].desc = %q, want RIS", events[2].desc)
	}

	unknown := decode([]byte("\x1bQ"))
	if !strings.Contains(unknown[0].desc, "unrecognized") {
		t.Errorf("unknown escape desc = %q, want unrecognized", unknown[0].desc)
	}
}

func TestDecodeIncompleteAtEnd(t *testing.T) {
	events := decode([]byte("abc\x1b"))
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(events), events)
	}
	if !strings.Contains(events[1].desc, "incomplete escape sequence") {
		t.Errorf("desc = %q, want incomplete escape sequence", events[1].desc)
	}
}

func TestDecodeLiteralTextTruncation(t *testing.T) {
	long := strings.Repeat("x", 50)
	events := decode([]byte(long))
	if !strings.Contains(events[0].raw, "...") {
		t.Errorf("raw = %q, want truncated with ...", events[0].raw)
	}
}
