---
title: Show a 20,000-line log
description: Embed the Log pane for a long log or trace, with find, copy and a scrollbar, drawing only the rows on screen.
sidebar:
  order: 80
---

![A find in a 20,000-line log, stepping through the hits and to both ends, with the scrollbar and the range count](../images/log.gif)

`Log` is a pane a screen embeds. It holds the lines once (`Arc`), builds only
the rows on screen, clips rather than wraps, and scrolls, pages and jumps to
either end. Drawing the demo's Log screen at 200x60 took 0.314 ms on average.

```rust
struct Journal {
    log: Log,
}

impl Journal {
    fn new(lines: Vec<Line<'static>>) -> Self {
        let mut log = Log::new("Find")
            .placeholder("type to find in the log")
            .hint("enter keeps it · esc clears it · n and N step through");
        log.set_lines(Arc::new(lines));
        Journal { log }
    }
}

impl Screen<Msg> for Journal {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        self.log.draw(frame, area, palette);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) {
        if let log::Turn::Copy(text) = self.log.key(key) {
            if cx.copy(text) {
                cx.say("Copied.", footer::Tone::Good);
            } else {
                cx.say("Too long to copy; y copies the page.", footer::Tone::Alert);
            }
        }
    }

    fn paste(&mut self, text: &str, _cx: &mut Cx<'_, Msg>) {
        self.log.paste(text);
    }

    fn typing(&self) -> bool {
        self.log.typing()
    }

    fn input(&self) -> Option<InputBar<'_>> {
        self.log.bar()
    }
}
```

The keys, each the app's to change (`keys`, `search_keys`, `hit_keys`,
`copy_keys`):

| Key | Does |
| --- | --- |
| arrows or `j`, `k` | scroll a line |
| `pgup`, `pgdn` or `ctrl+u`, `ctrl+d` | scroll a page |
| `g`, `G` or `home`, `end` | jump to the first or last line |
| `/` | find every line holding each word of the query |
| `n`, `N` | step to the next or previous hit |
| `y`, `Y` | copy the page, or every line (OSC 52, up to 48 KiB) |

At the bottom it follows new lines as `set_lines` replaces them, unless
`follow(false)` keeps its place. `top()`, `len()`, `hits()` and `hit()` feed
the screen's facts. The scrollbar and the range count come from the palette's
`bar` and `count` ([Colours and words](../palette-and-words/)).

```text
$ demo --dump 90x14 --keys "3 / 1999 enter n"
 ───────────────────────────────────────── demo ───────────────────────── workspace one ─
                             1 Home  2 What to do next  3 Log
                           line 11998 of 20000 · 2 of 12 found
 ─ Log ──────────────────────────────────────────────────────────────────────────────────
  11998  test      tested part 11998 of 20000                                          │
  11999  package   packed part 11999 of 20000                                          │
  12000  fetch     fetched part 12000 of 20000                                         ╽
  12001  compile   compiled part 12001 of 20000                                        ╿
  12002  link      linked part 12002 of 20000                                          │
                                                                 11,998–12,002 of 20,000

 ────────────────────────────────────────────────────────────────────────────────────────
 ↑↓ scroll · g/G ends · / find · n/N next · o jobs · ctrl+k go to · ctrl+c twice quit
```

A list that big, rather than a log, wants pito-list's `Shared`, which builds
only the rows on screen from the app's own items. The activity screen's
detail is a `Log` too.

Next: [Dumps, benches and frame captures](../headless/).
