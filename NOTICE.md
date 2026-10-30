# Notices

The look this shell draws with is in the style of HEY's terminal UI by 37signals
([basecamp/hey-cli](https://github.com/basecamp/hey-cli)): the centred rules,
the nested numbered tabs, the accent-lit current tab and the breadcrumb rule
of the header (`pito_tui::header`), the rules, the "key label · key label"
hints and the "Yes ▸ No" confirm bar of the footer (`pito_tui::footer`), the
bold marker on the selected row, the padded columns and the quiet end line of
the list (`pito_tui::list`), and the hourglass (`pito_tui::hourglass`).

The hourglass's 13 frames (`pito_tui::hourglass::FRAMES`: 7 drain, then 6
flip) are copied verbatim from hey-cli's `internal/tui/loading.go`, and its
drain and flip timing follows that file's `hourglassFrameIndex`. They are used
under hey-cli's MIT licence, reproduced here in full:

```text
MIT License

Copyright (c) 2026 37signals, LLC

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

No other code or text of hey-cli is copied into this crate.

## Images

Every image in `docs/images/` but the PITO MD wordmark (below) is made from
this crate's own code and sample data, no real data: © 2026 Cătălin Ilinca,
under the MIT licence like the code (see LICENSE); the names and logos in them
are as TRADEMARKS.md says. The images of the shell are named in
`docs/images/NOTICE`; the widgets' images are these:

- `docs/images/header/`, each with a `.license` file beside it:
  `demo.gif` and `fit.gif` are `examples/header_demo.rs` and
  `examples/header_fit.rs`, recorded with prec from
  `render/header/render/terminal.toml` (shots `header_demo` and `header_fit`)
  and made into GIFs with pcod
  ([pito-codecs](https://github.com/gmrdad82/pito-codecs)); `rows.png`,
  `spans.png` and `drill.png` are `examples/header_gallery.rs` with the
  argument `rows`, `spans` or `drill`, stills by prec (shots `header_rows`,
  `header_spans` and `header_drill`); `draw-time-bars.png` is drawn by
  pito-compare from `draw-time.toml`, the output of
  `cargo run --release --example header_bench 200000` on 2026-10-14.
  `bin/header-shots` makes them all again.
- `docs/images/footer/`: `demo.gif`, `input.gif` and `confirm.gif` are
  recorded with prec from `examples/footer_demo.rs` and
  `examples/footer_gallery.rs` (the shots in
  `render/footer/render/terminal.toml`) and made into GIFs with pcod
  (pito-codecs); `fit.png`, `notices.png` and `status.png` are prec stills of
  `examples/footer_gallery.rs`; `speed.png` is drawn by pito-compare from
  `render/footer/render/bench.toml`, the numbers of `examples/footer_bench.rs`.
- `docs/images/hourglass/`: `demo.gif`, `gallery.gif` and `fit.gif` are
  `examples/hourglass_demo.rs`, `examples/hourglass_gallery.rs` and
  `examples/hourglass_fit.rs`, recorded with prec from
  `render/hourglass/render/terminal.toml` (shots `hourglass_demo`,
  `hourglass_gallery` and `hourglass_fit`) and made into GIFs with pcod
  ([pito-codecs](https://github.com/gmrdad82/pito-codecs)); `frames.png` is
  `examples/hourglass_frames.rs`, a prec still (shot `hourglass_frames`);
  `draw-time.png` is drawn by pito-compare's `pcmp` from `draw-time.toml`,
  which `examples/hourglass_bench.rs` printed on 2026-10-14.
- `docs/images/list/`: `demo.gif` (with a `.license` file beside it),
  `range.gif`, `scroll.gif`, `live.gif` and `columns.gif` are clips of
  `examples/list_demo.rs`, recorded with prec from the shots in
  `render/list/render/terminal.toml` and made into GIFs with pcod
  (pito-codecs); `columns-96.png`, `columns-46.png` and `columns-38.png` are
  stills of `examples/list_demo.rs` at 96, 46 and 38 columns, rendered with
  prec from the `list_narrow` shot; `bench-bars.png` and
  `frame-time-paired.png` are charts drawn with pito-compare from
  `docs/images/list/charts/bench.toml` (the output of `examples/list_bench.rs`)
  and `docs/images/list/charts/frame-time.toml` (the output of
  `examples/list_versus.rs`), measured on 2026-10-14.

## The PITO MD wordmark

The PITO MD wordmark in docs/images/pitomd-wordmark.svg, shown at the README's
Licence section, is copied by bin/marks from the PITO MD brand masters
at aa3aee8b232a1b848995bf3d9a012a01d4aff8ec
(`exports/pitomd/readme/wordmark.svg`). The PITO MD name and
its logos are © Cătălin Ilinca, all rights reserved and not covered by the MIT
licence (TRADEMARKS.md).
