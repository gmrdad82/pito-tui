---
title: Select and copy text
description: A drag with the left button selects any text on screen and copies it on release, in every app that captures the mouse, on the shell or on a loop of its own.
sidebar:
  order: 130
---

An app that captures the mouse (`Modes::mouse(true)`, for the wheel and
clicks) takes the terminal's own text selection away. The shell gives it
back: drag with the left button over anything on screen (the screen's own
drawing, the header, the footer, lists, logs, modals, toasts, the `?` sheet)
and the cells are highlighted while the button is held and copied to the
clipboard on release.

## In the shell

There is nothing to wire: every app on `Tui` that captures the mouse
selects. An app that doesn't capture it gets no mouse events, so the
terminal keeps its own selection and nothing changes.

```rust
Tui::new("notes", env!("CARGO_PKG_VERSION"), accent)
    .modes(Modes::new().mouse(true))
    .words(Words::new().copied("Copied"))
    .screen("Notes", Notes::default())
```

- **A click is still a click.** The press always reaches the screen; a
  press and release on the same cell reaches it whole. Once the pointer
  moves a cell, the drag and the release belong to the selection.
- **Like a terminal's.** From the pressed cell to the released one, whole
  rows between, in either direction. Wide glyphs are copied once, combining
  marks stay with their letter, each row's trailing blanks are trimmed and
  the rows are joined with new lines.
- **The highlight** is `Palette::selection`, reversed by default so it reads
  over any colour (a cell that is already reversed flips back), painted over
  the whole frame after everything else. It stays until the next press or
  key.
- **A toast in the app's words.** `Words::copied`, empty by default, shows
  after each copy; the shell says nothing of its own.
- **Shift-drag is the terminal's.** Most terminals keep shift-drag for their
  own selection even while an app captures the mouse; if one passes it on,
  pito-tui doesn't select.

`Tui::select(false)` turns it off for an app. A screen that uses drags
itself (a slider, a canvas) keeps them by answering `Screen::drags`:

```rust
impl Screen<Msg> for Canvas {
    fn drags(&self) -> bool {
        true
    }
}
```

A press over that screen's area then reaches it whole (press, drag,
release) and starts no selection; the header and footer around it stay
selectable.

## Where the copy goes

Each copy goes two ways at once, so it lands wherever either works:

- **OSC 52** to the terminal, up to `COPY_MAX` (48 KiB); longer text skips it.
- **The platform's clipboard command** when one is on `PATH`: `pbcopy` on
  macOS, `clip.exe` on Windows, `wl-copy` under Wayland (`WAYLAND_DISPLAY`
  set), else `xclip -selection clipboard` or `xsel --clipboard --input`
  under X11 (`DISPLAY` set). It is looked up on the first copy and runs on a
  thread of its own with no window, so the loop never waits for it.

With OSC 52 off in the terminal and no command (over SSH to a host with no
display, say), nothing is copied. `Tui::clipboard(sink)` hands both to the
app's own `select::Clip` instead, the way tests keep off the real clipboard.

Tested headless only, by the crate's tests on Linux and macOS. Untested in a
real terminal: Ghostty, kitty, Alacritty, foot, WezTerm, GNOME Terminal,
Konsole, iTerm2, Terminal.app and Windows Terminal; the clipboard commands
are untested on macOS and Windows.

## On a loop of its own

`Selection` is the same model the shell uses. Feed it every mouse event and
keep the ones it answers `Pass`, `Press` or `Click` for the app; `Drag` and
`Done` are its own. After drawing each frame, take the text a release left
(once) and paint the highlight:

```rust
let mut selection = Selection::new();
let mut clipboard = select::System::new();

match selection.feed(mouse) {
    Fed::Drag | Fed::Done => {}
    _ => app.mouse(mouse),
}

let mut copied = None;
terminal.draw(|frame| {
    app.draw(frame);
    copied = selection.take(frame.buffer_mut());
    selection.paint(frame.buffer_mut(), palette.selection);
})?;
if let Some(text) = copied {
    select::send(&text, &mut clipboard);
}
```

Clear it on a key with `selection.clear()`, as the shell does. A steady
frame with a selection shown allocates nothing; only the frame after a
release takes the text.

## Try it

```sh
cargo run --example select_demo
cargo run --example select_demo -- --dump 90x24 drag 8,5 40,13
```

The first runs the demo: drag over the sky, the list or the log. The second
prints one frame after a scripted drag, the highlight in reverse video,
then the text it copied.

Next: [the API at a glance](../../reference/api/).
