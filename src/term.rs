use std::io::{self, Write};
use std::panic::{self, PanicHookInfo};
use std::sync::Arc;
use std::sync::atomic::{AtomicU8, Ordering};
use std::thread::{self, ThreadId};

use crossterm::cursor::Show;
use crossterm::event::{
    DisableBracketedPaste, DisableFocusChange, DisableMouseCapture, EnableBracketedPaste,
    EnableFocusChange, EnableMouseCapture,
};
use crossterm::terminal::{
    self, BeginSynchronizedUpdate, Clear, ClearType, EndSynchronizedUpdate, EnterAlternateScreen,
    LeaveAlternateScreen,
};
use crossterm::{execute, queue};
use ratatui::backend::CrosstermBackend;
use ratatui::{DefaultTerminal, Frame, Terminal};

pub(crate) const RAW: u8 = 1;
pub(crate) const ALTERNATE: u8 = 2;
pub(crate) const PASTE: u8 = 4;
pub(crate) const MOUSE: u8 = 8;
pub(crate) const FOCUS: u8 = 16;

static ON: AtomicU8 = AtomicU8::new(0);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Modes {
    pub alternate: bool,
    pub paste: bool,
    pub mouse: bool,
    pub focus: bool,
}

impl Modes {
    pub const fn new() -> Self {
        Modes {
            alternate: true,
            paste: true,
            mouse: false,
            focus: false,
        }
    }

    pub const fn alternate(mut self, on: bool) -> Self {
        self.alternate = on;
        self
    }

    pub const fn paste(mut self, on: bool) -> Self {
        self.paste = on;
        self
    }

    pub const fn mouse(mut self, on: bool) -> Self {
        self.mouse = on;
        self
    }

    pub const fn focus(mut self, on: bool) -> Self {
        self.focus = on;
        self
    }
}

impl Default for Modes {
    fn default() -> Self {
        Modes::new()
    }
}

type Hook = dyn Fn(&PanicHookInfo<'_>) + Sync + Send + 'static;

pub(crate) fn restores(ui: ThreadId, here: ThreadId, on: u8) -> bool {
    ui == here && on & RAW != 0
}

pub(crate) fn undo(on: &AtomicU8, out: &mut impl Write, raw: impl FnOnce()) -> u8 {
    let was = on.swap(0, Ordering::SeqCst);
    if was & FOCUS != 0 {
        let _ = queue!(out, DisableFocusChange);
    }
    if was & MOUSE != 0 {
        let _ = queue!(out, DisableMouseCapture);
    }
    if was & PASTE != 0 {
        let _ = queue!(out, DisableBracketedPaste);
    }
    if was & ALTERNATE != 0 {
        let _ = queue!(out, LeaveAlternateScreen);
    }
    if was != 0 {
        let _ = queue!(out, Show);
        let _ = out.flush();
    }
    if was & RAW != 0 {
        raw();
    }
    was
}

pub fn restore() -> bool {
    let mut out = io::stdout().lock();
    undo(&ON, &mut out, || {
        let _ = terminal::disable_raw_mode();
    }) != 0
}

fn switch(on: &AtomicU8, flag: u8, out: &mut impl Write, enable: bool) -> io::Result<()> {
    if enable == (on.load(Ordering::SeqCst) & flag != 0) {
        return Ok(());
    }
    match (flag, enable) {
        (MOUSE, true) => execute!(out, EnableMouseCapture)?,
        (MOUSE, false) => execute!(out, DisableMouseCapture)?,
        (PASTE, true) => execute!(out, EnableBracketedPaste)?,
        (PASTE, false) => execute!(out, DisableBracketedPaste)?,
        (FOCUS, true) => execute!(out, EnableFocusChange)?,
        (FOCUS, false) => execute!(out, DisableFocusChange)?,
        (ALTERNATE, true) => execute!(out, EnterAlternateScreen)?,
        (ALTERNATE, false) => execute!(out, LeaveAlternateScreen)?,
        _ => {}
    }
    if enable {
        on.fetch_or(flag, Ordering::SeqCst);
    } else {
        on.fetch_and(!flag, Ordering::SeqCst);
    }
    Ok(())
}

fn turn_on(modes: Modes) -> io::Result<()> {
    let mut out = io::stdout().lock();
    terminal::enable_raw_mode()?;
    ON.fetch_or(RAW, Ordering::SeqCst);
    switch(&ON, ALTERNATE, &mut out, modes.alternate)?;
    switch(&ON, PASTE, &mut out, modes.paste)?;
    switch(&ON, MOUSE, &mut out, modes.mouse)?;
    switch(&ON, FOCUS, &mut out, modes.focus)
}

fn live() -> bool {
    ON.load(Ordering::SeqCst) & RAW != 0
}

struct Restore {
    previous: Arc<Hook>,
}

impl Restore {
    fn install() -> Self {
        let previous: Arc<Hook> = panic::take_hook().into();
        let chained = Arc::clone(&previous);
        let ui = thread::current().id();
        panic::set_hook(Box::new(move |info| {
            if restores(ui, thread::current().id(), ON.load(Ordering::SeqCst)) {
                restore();
            }
            chained(info);
        }));
        Restore { previous }
    }
}

impl Drop for Restore {
    fn drop(&mut self) {
        restore();
        if !thread::panicking() {
            let _ = panic::take_hook();
            let previous = Arc::clone(&self.previous);
            panic::set_hook(Box::new(move |info| previous(info)));
        }
    }
}

pub struct Term {
    terminal: DefaultTerminal,
    modes: Modes,
    _restore: Restore,
}

impl Term {
    pub fn enter(modes: Modes) -> io::Result<Term> {
        let restore = Restore::install();
        turn_on(modes)?;
        execute!(io::stdout(), Clear(ClearType::All))?;
        let terminal = Terminal::new(CrosstermBackend::new(io::stdout()))?;
        Ok(Term {
            terminal,
            modes,
            _restore: restore,
        })
    }

    pub fn terminal(&mut self) -> &mut DefaultTerminal {
        &mut self.terminal
    }

    pub fn modes(&self) -> Modes {
        self.modes
    }

    pub fn draw(&mut self, render: impl FnOnce(&mut Frame)) -> io::Result<()> {
        let mut out = io::stdout().lock();
        if !live() {
            return Ok(());
        }
        execute!(out, BeginSynchronizedUpdate)?;
        let drawn = self.terminal.draw(render).map(|_| ());
        execute!(out, EndSynchronizedUpdate)?;
        drawn
    }

    pub fn mouse(&mut self, on: bool) -> io::Result<()> {
        self.modes.mouse = on;
        let mut out = io::stdout().lock();
        if !live() {
            return Ok(());
        }
        switch(&ON, MOUSE, &mut out, on)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_terminal_is_restored_on_the_ui_thread_only_while_raw_mode_is_on() {
        let ui = thread::current().id();
        let worker = thread::spawn(|| thread::current().id()).join().unwrap();
        assert!(restores(ui, ui, RAW | ALTERNATE));
        assert!(!restores(ui, worker, RAW | ALTERNATE));
        assert!(!restores(ui, ui, 0));
        assert!(!restores(ui, ui, ALTERNATE));
    }

    #[test]
    fn every_mode_that_was_turned_on_is_turned_off_once() {
        let on = AtomicU8::new(RAW | ALTERNATE | PASTE | MOUSE);
        let mut out = Vec::new();
        let mut raw = 0;
        assert_eq!(
            undo(&on, &mut out, || raw += 1),
            RAW | ALTERNATE | PASTE | MOUSE
        );
        let written = String::from_utf8(out).unwrap();
        for off in ["\x1b[?2004l", "\x1b[?1049l", "\x1b[?1000l", "\x1b[?25h"] {
            assert!(written.contains(off), "{off:?} in {written:?}");
        }
        assert!(!written.contains("\x1b[?1004l"), "focus was never on");
        assert_eq!(raw, 1);
        let mut again = Vec::new();
        assert_eq!(undo(&on, &mut again, || raw += 1), 0);
        assert!(again.is_empty());
        assert_eq!(raw, 1);
    }

    #[test]
    fn any_thread_gives_the_terminal_back_and_only_the_first_one_does() {
        ON.store(RAW | PASTE, Ordering::SeqCst);
        let threads: Vec<_> = (0..4).map(|_| thread::spawn(restore)).collect();
        let gave = threads
            .into_iter()
            .filter_map(|thread| thread.join().ok())
            .filter(|gave| *gave)
            .count();
        assert_eq!(gave, 1);
        assert_eq!(ON.load(Ordering::SeqCst), 0);
        assert!(!restore());
    }
}
