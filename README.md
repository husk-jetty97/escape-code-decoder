# escseq

A terminal spits out something like this when you cat a binary log or
capture raw output from a program that manages its own screen:

```
^[[1;31mERROR^[[0m: connection refused^[[2K^[[1G
```

You can usually guess `^[[1;31m` sets red text, but the further you get
from plain SGR color codes the less obvious it is. What does `^[[2K` do?
What's `^[]0;` about? Is that a DEC private mode or a plain ANSI one? This
is a small tool that answers exactly that question: given a chunk of bytes
or a literal string, it walks through every escape sequence it finds and
prints, in order, what each one means.

It does not render anything or emulate a terminal. It just explains.

## build

```
go build -o escseq .
```

No dependencies, standard library only.

## usage

Pass a literal string on the command line, using backslash notation for
the escape byte (`\x1b`, `\e`, or `\033` all work):

```
$ escseq '\x1b[1;31mERROR\x1b[0m: connection refused\x1b[2K'
     0  ESC [ 1;31 m                  SGR: bold, set foreground red
     9  "ERROR"                       literal text
    14  ESC [ 0 m                     SGR: reset all attributes
    20  ": connection refused"        literal text
    40  ESC [ 2 K                     erase entire line
```

Or pipe raw bytes in, which is the more useful form once you've captured
real output from something (script, tmux capture-pane, a log file with
control characters still in it):

```
$ cat session.log | escseq
$ printf '\033]0;my title\007' | escseq
     0  ESC ] 0;my title BEL          OSC: set icon name and window title ("my title")
```

Or point it at a file directly with `-f`, which skips the shell pipe:

```
$ escseq -f session.log
```

Coverage: CSI sequences (cursor movement, SGR colors and attributes,
erase, scroll regions, DEC private modes via `?`), OSC sequences (window
title, colors, hyperlinks), the string sequences (DCS/SOS/PM/APC, shown
with their raw payload but not decoded further), and the common two-byte
ESC sequences (save/restore cursor, reset, index).

## what it doesn't do

It doesn't decode terminfo capability names, it doesn't know about a
specific terminal's private extensions beyond the common ones, and it
doesn't validate that a sequence is something a real terminal would
accept versus something malformed. It reads what's there and tells you
what the spec says it means.

## license

MIT, see LICENSE.
