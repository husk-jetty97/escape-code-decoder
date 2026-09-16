package main

import (
	"fmt"
	"strconv"
	"strings"
)

var stringSeqKind = map[byte]string{'P': "DCS", 'X': "SOS", '^': "PM", '_': "APC"}

var simpleNames = map[byte]string{
	'7': "DECSC: save cursor position and attributes",
	'8': "DECRC: restore cursor position and attributes",
	'c': "RIS: full terminal reset",
	'D': "IND: index (move down a line, scrolling at the bottom margin)",
	'M': "RI: reverse index (move up a line, scrolling at the top margin)",
	'E': "NEL: next line", 'H': "HTS: set a tab stop at the current column",
	'=': "DECKPAM: enable application keypad mode",
	'>': "DECKPNM: enable normal keypad mode",
	'N': "SS2: single shift to G2 charset for next character",
	'O': "SS3: single shift to G3 charset for next character",
	'Z': "DECID: identify terminal (obsolete, superseded by DA)",
}

// csiFinal covers finals whose meaning doesn't need the params unpacked
// any further than "here they are, for reference".
var csiFinal = map[byte]string{
	'A': "CUU: move cursor up", 'B': "CUD: move cursor down",
	'C': "CUF: move cursor forward", 'D': "CUB: move cursor back",
	'E': "CNL: cursor to start of line, N lines down",
	'F': "CPL: cursor to start of line, N lines up",
	'G': "CHA: move cursor to column",
	'H': "CUP: move cursor to row;column", 'f': "HVP: move cursor to row;column",
	'S': "SU: scroll page up", 'T': "SD: scroll page down",
	'n': "DSR: device status report request",
	's': "SCP: save cursor position", 'u': "RCP: restore cursor position",
	'd': "VPA: move cursor to line",
	'r': "DECSTBM: set scrolling region (top;bottom)",
	'L': "IL: insert blank lines at cursor", 'M': "DL: delete lines at cursor",
	'P': "DCH: delete characters at cursor",
	'@': "ICH: insert blank characters at cursor",
	'X': "ECH: erase characters at cursor",
	'c': "DA: request terminal identity",
	't': "window manipulation (resize, move, minimize, report size, etc.)",
}

var sgrSimple = map[int]string{
	0: "reset all attributes", 1: "bold", 2: "dim", 3: "italic", 4: "underline",
	5: "slow blink", 6: "rapid blink", 7: "reverse video", 8: "conceal", 9: "strikethrough",
	21: "double underline (or not-bold, terminal dependent)",
	22: "normal intensity (not bold, not dim)", 23: "not italic",
	24: "not underlined", 25: "not blinking", 27: "not reversed",
	28: "reveal (not concealed)", 29: "not strikethrough",
	39: "default foreground color", 49: "default background color",
}

var colorNames = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}

var decModes = map[int]string{
	1: "application cursor keys", 7: "auto-wrap", 12: "cursor blinking",
	25: "cursor visible", 1000: "mouse click tracking",
	1002: "mouse button-event tracking", 1003: "mouse any-event tracking",
	1006: "SGR extended mouse coordinates",
	1049: "alternate screen buffer, saving cursor", 2004: "bracketed paste mode",
}

var oscNames = map[string]string{
	"0": "set icon name and window title", "1": "set icon name", "2": "set window title",
	"4": "set or query a color palette entry", "8": "hyperlink (OSC 8) start or end",
	"10": "set default foreground color", "11": "set default background color",
	"52": "clipboard access", "104": "reset a color palette entry to default",
}

// describeCSI dispatches the handful of finals whose params need unpacking
// (erase ranges, modes, SGR) and falls back to a plain lookup for the rest.
func describeCSI(params string, final byte) string {
	switch final {
	case 'm':
		return describeSGR(params)
	case 'h', 'l':
		return describeMode(params, final)
	case 'J':
		return describeErase(params, "screen")
	case 'K':
		return describeErase(params, "line")
	}
	name, ok := csiFinal[final]
	if !ok {
		return fmt.Sprintf("CSI sequence, final byte %q (unrecognized)", string(final))
	}
	if params == "" {
		return name
	}
	return fmt.Sprintf("%s (params: %s)", name, params)
}

func describeErase(params, unit string) string {
	switch params {
	case "", "0":
		return "erase from cursor to end of " + unit
	case "1":
		return "erase from start of " + unit + " to cursor"
	case "2":
		return "erase entire " + unit
	case "3":
		if unit == "screen" {
			return "erase entire screen and scrollback buffer"
		}
	}
	return fmt.Sprintf("erase %s, unrecognized parameter %s", unit, params)
}

func describeMode(params string, final byte) string {
	action := "set"
	if final == 'l' {
		action = "reset"
	}
	private := strings.HasPrefix(params, "?")
	var names []string
	for _, p := range strings.Split(strings.TrimPrefix(params, "?"), ";") {
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		switch {
		case private && decModes[n] != "":
			names = append(names, decModes[n])
		case private:
			names = append(names, fmt.Sprintf("DEC private mode %d", n))
		default:
			names = append(names, fmt.Sprintf("ANSI mode %d", n))
		}
	}
	return fmt.Sprintf("%s mode: %s", action, strings.Join(names, ", "))
}

// describeSGR unpacks a Select Graphic Rendition parameter list. The 38/48
// extended color forms are special-cased since they consume extra
// parameters from the list (either 5;n for 256-color or 2;r;g;b truecolor).
func describeSGR(params string) string {
	if params == "" {
		params = "0"
	}
	parts := strings.Split(params, ";")
	var out []string
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			continue
		}
		switch {
		case n == 38 || n == 48:
			kind := "foreground"
			if n == 48 {
				kind = "background"
			}
			if i+1 >= len(parts) {
				out = append(out, "set "+kind+" color (incomplete)")
				break
			}
			switch parts[i+1] {
			case "5":
				if i+2 < len(parts) {
					out = append(out, fmt.Sprintf("set %s to color %s (256-color palette)", kind, parts[i+2]))
					i += 2
				}
			case "2":
				if i+4 < len(parts) {
					out = append(out, fmt.Sprintf("set %s to rgb(%s,%s,%s)", kind, parts[i+2], parts[i+3], parts[i+4]))
					i += 4
				}
			default:
				out = append(out, "set "+kind+" color (extended, unrecognized form)")
			}
		case n >= 30 && n <= 37:
			out = append(out, "set foreground "+colorNames[n-30])
		case n >= 40 && n <= 47:
			out = append(out, "set background "+colorNames[n-40])
		case n >= 90 && n <= 97:
			out = append(out, "set foreground bright "+colorNames[n-90])
		case n >= 100 && n <= 107:
			out = append(out, "set background bright "+colorNames[n-100])
		default:
			if name, ok := sgrSimple[n]; ok {
				out = append(out, name)
			} else {
				out = append(out, fmt.Sprintf("SGR %d (unrecognized)", n))
			}
		}
	}
	return "SGR: " + strings.Join(out, ", ")
}

func describeOSC(body string) string {
	parts := strings.SplitN(body, ";", 2)
	ps := parts[0]
	pt := ""
	if len(parts) > 1 {
		pt = parts[1]
	}
	name, ok := oscNames[ps]
	if !ok {
		return fmt.Sprintf("OSC %s (unrecognized), data %q", ps, truncate(pt, 60))
	}
	if pt == "" {
		return "OSC: " + name
	}
	return fmt.Sprintf("OSC: %s (%q)", name, truncate(pt, 60))
}
