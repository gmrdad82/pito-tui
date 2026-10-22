---
title: Formats
description: The progress lines pito-tui reads from commands, the key scripts and sizes it parses, its text dumps, and the capture files and differences it writes.
sidebar:
  order: 210
---

## Progress lines

`Cx::run` and `Cx::run_from` follow the `--progress json` lines the PITO MD
command-line tools print, version 1: one JSON object a line.

```text
{"v":1,"op":"check","app":"demo","ref":null,"stage":"resolve","state":"start","fraction":null,"msg":"resolve starting","at":1792004625071}
{"v":1,"op":"check","app":"demo","ref":null,"stage":"resolve","state":"progress","fraction":0.062,"msg":"resolve 1 of 4","at":1792004625272}
{"v":1,"op":"check","app":"demo","ref":null,"state":"end","ok":true,"msg":"every check passed","at":1792004628472}
```

The fields pito-tui reads; any other field is ignored:

| Field | Type | Meaning |
| --- | --- | --- |
| `v` | number | the version; a line with a `v` other than `1` is not a progress line |
| `state` | string, required | `start`, `progress`, `done`, `fail` or `end` |
| `stage` | string | the step the command is on |
| `fraction` | number | how far along, 0 to 1 |
| `msg` | string | a short status for the band |
| `ok` | boolean | on the `end` line, whether the command succeeded |

A line that isn't JSON, or has no `state`, is not a progress line; it still
goes into the activity's detail and reaches `Screen::heard` as text.
`command::read(line)` parses one line into a `command::Progress`. The
activity ends on the `end` line, or with the command's exit status.

## Key scripts

`dump::keys("tab / foo enter ctrl+c")` reads keys separated by spaces:

- the names `enter`, `esc`, `up`, `down`, `left`, `right`, `pgup`, `pgdn`,
  `home`, `end`, `backspace`, `delete`, `space`, `tab` and `backtab`, in any
  case;
- `ctrl+` or `alt+`, in lowercase, followed by one character, such as
  `ctrl+k`;
- any other word types its characters one by one, so `foo` is `f`, `o`,
  `o`.

A `ctrl+` or `alt+` word with more than one character after it is an error,
returned as that word.

## Sizes

`dump::size("120x34")` reads `WIDTHxHEIGHT` (an `X` works too), both plain
digits above zero; a sign, a space or anything else is refused.

## Text dumps

- `dump::text(&buffer)` prints each row's symbols, a wide character once,
  with the trailing spaces of each row trimmed, rows joined by newlines.
- `dump::ansi(&buffer)` hides the cursor, clears the screen, then writes each
  row at its position with the SGR codes for its colours and modifiers,
  resetting at the end of each row.

## Captures

`capture::capture` and `capture::Recorder` write one folder per scenario,
and one file per size in it:

```text
tmp/before/
  00-home/
    40x12
    80x24
    120x34
    200x60
  01-what-to-do-next/
  drill-in/
  quit-guard/
```

A screen's folder is its index among the app's screens, two digits, then its
name; a script's is its name. Names are lowercased, with each run of other
characters turned into one `-`. A name with no letters or digits is refused
(`InvalidInput`), and so is a walk whose scenarios land in the same folder
(`AlreadyExists`), before anything is written.

Each file is one frame, as text:

```text
pito-tui frame 1
80 24
<row 0 symbols, tab-separated>
<row 0 styles>
<row 1 symbols, tab-separated>
<row 1 styles>
...
```

- The first line names the format and its version; the second is the
  width and the height.
- Then two lines a row. The first holds every cell's symbol, separated by
  tabs, with `\\`, `\t`, `\n` and `\r` escaped.
- The second holds the row's styles as runs, separated by tabs: a count of
  cells, the foreground, the background and the modifier bits, such as
  `1 #5B8CFF Reset 1` (one cell, the accent, on the terminal's background,
  bold) or `78 Reset Reset 2` (78 dim cells). Colours are ratatui's names
  (`Reset`, `Red`, ...), `#RRGGBB` or an index.

A frame whose head or size line doesn't parse, that ends early, whose row
holds more or fewer symbols than its width, whose runs don't add up to
exactly its width, or whose size line disagrees with its file name is
refused (`InvalidData`).

## Differences

`capture::compare` and `Recorder::finish` return `capture::Difference`s,
which print one a line:

```text
00-home 80x24 0,0: "X" Reset Reset NONE → " " Reset Reset NONE
about 40x12: +
gone 80x24: -
```

- A cell: the scenario, the size, `x,y`, then the cell before and after
  (its symbol, foreground, background and modifiers).
- `+`: a frame only the new walk draws (`Added`).
- `-`: a frame only the capture holds (`Removed`).

The added and changed frames come in the walk's order (for `compare_dirs`,
by scenario, then by size), and the removed ones after them, by scenario,
then by size.

The demo's `--compare` exits 1 when any frame differs.
