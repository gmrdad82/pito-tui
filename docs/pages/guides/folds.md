---
title: "Fold sections: Folds"
description: A column of titled sections the user expands and collapses, each fold eased like a curtain, with its state as plain data the app saves.
sidebar:
  order: 120
---

`Folds` lays out a column of titled sections. An expanded section draws its
title rule, then its body; a collapsed one is exactly one row, its title rule
carrying a summary the app gives (`▸ Jobs ───── 3 running ─`). Between the two
the body slides open or shut like a curtain, eased in and out over 180 ms; it
never reflows mid-fold. The shell has no words of its own here: every title,
summary, style, mark and key comes from the app.

```rust
const ONE: &[footer::Key] = &[footer::Key::Char(' ')];
const ALL: &[footer::Key] = &[footer::Key::Char('z')];

struct Overview {
    folds: Folds,
    now: Instant,
    rows: usize,
}

impl Overview {
    fn new(saved: Option<&FoldState>, reduced: bool) -> Self {
        let mut folds = Folds::new(["jobs", "logs"]).keys(ONE, ALL).still(reduced);
        if let Some(saved) = saved {
            folds.restore(saved);
        }
        Overview { folds, now: Instant::now(), rows: 12 }
    }
}

impl Screen<Msg> for Overview {
    fn moment(&mut self, now: Instant) {
        self.now = now;
        self.folds.moment(now);
    }

    fn animating(&self) -> bool {
        self.folds.animating()
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) {
        if self.folds.key(key, self.now) {
            save(&self.folds.state());
        }
    }

    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let jobs = [("Jobs".into(), Style::new())];
        let running = [("3 running".into(), palette.good)];
        let logs = [("Logs".into(), Style::new())];
        let sections = [
            Fold { title: &jobs, summary: &running, rows: 3 },
            Fold { title: &logs, summary: &[], rows: self.rows as u16 },
        ];
        self.folds.draw(frame, area, palette, &sections, |buf, index, rect| {
            draw_body(buf, index, rect);
        });
    }
}
```

- `moment(now)` hands `Folds` the loop's moment; drawing reads it, never the
  clock. `animating()` keeps the loop drawing while any fold runs and lets it
  sleep once every section has settled.
- `key` toggles the cursor's section for a key in the first list and every
  section for one in the second (any open one shuts them all; when all are
  shut it opens them all), and says whether it took the key. Move the cursor
  with `point(index)`; `title_at(column, row)` turns a click on a title row
  into its index. `toggle`, `toggle_all`, `set` and `set_all` do the same from
  the app's own code.
- `sections` is a stack array of `Fold`, built each frame from what the app
  already holds; `Fold::rows` is the body's full height. `scroll(top)` moves
  the column, clamped to `total()` at the next draw, and `top_of(index)` gives
  a title's row in the column, so the app can keep the cursor's section in
  view. The clamp isn't read back, so an app that keeps its own scroll
  counter clamps it too after each draw, to
  `total().saturating_sub(area.height)`, or "up" past the end does nothing
  for many presses.
- The body closure gets the visible rect of a steady section, at most its
  room on the area: when a body is taller than its room, the app draws its
  own `+N more` row in the last row it gets. While a section folds, or once
  its top is scrolled off, the closure draws the whole body (or the part that
  would fit if it were open) into a scratch buffer `Folds` keeps, and the
  rows that show are copied from there, over what the frame already held
  under them, so the app's background stays.
- `placed()` lists, for every section drawn, its title row (an empty rect when
  it is off the area), its visible body rows, its full rows and whether it is
  folding.

The look is the app's: `marks(open, shut)` (default `▾` and `▸`), `gap(rows)`
between sections, `lit(true)` to draw the cursor's title in the palette's
`selected` style, and `lasting(Duration)` for the fold's length. The rule is
the palette's `rule`, titles the palette's `strong` and summaries its
`muted`, each span's own style patched over.

## The state is the app's

`state()` returns a `FoldState`, the ids in order with whether each is
collapsed: plain data the app saves where it saves its settings.
`restore(&state)` puts each known id back at once, with no animation; ids it
doesn't know are skipped, and sections the state doesn't name stay as they
are, so a saved state still fits after the app adds, drops or reorders
sections.

## Reduced motion

`still(true)` (or `set_still` while the app runs) makes every fold instant:
the section opens or shuts in the very next frame. A zero `lasting` does the
same.

## Speed

A steady column allocates nothing per frame: titles and summaries are
borrowed, the title rows are written straight into the frame's buffer, and
the scratch buffer only grows, so even a column scrolled past a section's top
reuses it. A fold costs one extra draw of the folding section's body into the
scratch buffer and a copy of the rows that show.
