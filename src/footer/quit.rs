use std::borrow::Cow;
use std::time::{Duration, Instant};

use crate::footer::confirm::{Answer, Confirm, ConfirmBar, Words};
use crate::footer::key::Key;

pub const WINDOW: Duration = Duration::from_secs(2);

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Wording {
    pub again: Cow<'static, str>,
    pub ask: Cow<'static, str>,
}

impl Wording {
    pub fn new(again: impl Into<Cow<'static, str>>) -> Self {
        Wording {
            again: again.into(),
            ask: Cow::Borrowed(""),
        }
    }

    pub fn ask(mut self, ask: impl Into<Cow<'static, str>>) -> Self {
        self.ask = ask.into();
        self
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Mode {
    #[default]
    Twice,
    Ask,
    Once,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Guard {
    Pass,
    Held,
    Quit,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct QuitGuard {
    mode: Mode,
    window: Duration,
    triggers: &'static [Key],
    armed: Option<Instant>,
    asking: Option<Confirm>,
    confirm: Confirm,
    wording: Wording,
}

impl QuitGuard {
    pub fn new(wording: Wording) -> Self {
        QuitGuard {
            mode: Mode::Twice,
            window: WINDOW,
            triggers: &[Key::Ctrl('c')],
            armed: None,
            asking: None,
            confirm: Confirm::new(),
            wording,
        }
    }

    pub fn mode(mut self, mode: Mode) -> Self {
        self.mode = mode;
        self
    }

    pub fn window(mut self, window: Duration) -> Self {
        self.window = window;
        self
    }

    pub fn triggers(mut self, triggers: &'static [Key]) -> Self {
        self.triggers = triggers;
        self
    }

    pub fn confirm(mut self, confirm: Confirm) -> Self {
        self.confirm = confirm;
        self
    }

    pub fn wording_mut(&mut self) -> &mut Wording {
        &mut self.wording
    }

    pub fn key(&mut self, key: Key, now: Instant, busy: bool) -> Guard {
        let trigger = self.triggers.contains(&key);
        if let Some(confirm) = self.asking.as_mut() {
            let answer = if trigger {
                Some(Answer::No)
            } else {
                confirm.key(key)
            };
            return match answer {
                Some(Answer::Yes) => {
                    self.asking = None;
                    Guard::Quit
                }
                Some(Answer::No) => {
                    self.asking = None;
                    Guard::Held
                }
                None => Guard::Held,
            };
        }
        if !trigger {
            self.armed = None;
            return Guard::Pass;
        }
        if self.mode == Mode::Once {
            return Guard::Quit;
        }
        let again = self
            .armed
            .take()
            .is_some_and(|at| now.saturating_duration_since(at) < self.window);
        if !again {
            self.armed = Some(now);
            return Guard::Held;
        }
        if self.mode == Mode::Ask && busy {
            self.asking = Some(self.confirm);
            return Guard::Held;
        }
        Guard::Quit
    }

    pub fn tick(&mut self, now: Instant) -> bool {
        if self
            .armed
            .is_some_and(|at| now.saturating_duration_since(at) >= self.window)
        {
            self.armed = None;
            return true;
        }
        false
    }

    pub fn deadline(&self) -> Option<Instant> {
        self.armed.and_then(|at| at.checked_add(self.window))
    }

    pub fn armed(&self) -> bool {
        self.armed.is_some()
    }

    pub fn notice(&self) -> Option<&str> {
        self.armed.map(|_| self.wording.again.as_ref())
    }

    pub fn asking(&self) -> Option<&Confirm> {
        self.asking.as_ref()
    }

    pub fn bar<'a>(&'a self, words: Words<'a>) -> Option<ConfirmBar<'a>> {
        self.asking
            .as_ref()
            .map(|confirm| ConfirmBar::new(&self.wording.ask, confirm, words))
    }

    pub fn cancel(&mut self) {
        self.armed = None;
        self.asking = None;
    }
}
