use std::borrow::Cow;

use crossterm::event::KeyEvent;
use pito_footer::{Edit, Input, InputBar, Key, Tone};
use pito_list::{Column, Keys, List, ListView, Row, Step};

use crate::palette::Palette;

const START: &[Key] = &[Key::Char('/')];
const CLEAR: &[Key] = &[Key::Esc];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Turn {
    Pass,
    Taken,
    Filtered,
    Open(usize),
}

#[derive(Debug, Clone)]
pub struct Filter {
    list: List,
    input: Input,
    typing: bool,
    shown: Vec<usize>,
    label: Cow<'static, str>,
    placeholder: Cow<'static, str>,
    hint: Cow<'static, str>,
    start: &'static [Key],
    clear: &'static [Key],
}

impl Filter {
    pub fn new(label: impl Into<Cow<'static, str>>) -> Self {
        Filter {
            list: List::new(),
            input: Input::new(),
            typing: false,
            shown: Vec::new(),
            label: label.into(),
            placeholder: Cow::Borrowed(""),
            hint: Cow::Borrowed(""),
            start: START,
            clear: CLEAR,
        }
    }

    pub fn placeholder(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.placeholder = text.into();
        self
    }

    pub fn hint(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.hint = text.into();
        self
    }

    pub fn keys(mut self, keys: Keys) -> Self {
        self.list = self.list.keys(keys);
        self
    }

    pub fn start_keys(mut self, keys: &'static [Key]) -> Self {
        self.start = keys;
        self
    }

    pub fn clear_keys(mut self, keys: &'static [Key]) -> Self {
        self.clear = keys;
        self
    }

    pub fn input(mut self, input: Input) -> Self {
        self.input = input;
        self
    }

    pub fn typing(&self) -> bool {
        self.typing
    }

    pub fn query(&self) -> &str {
        self.input.value()
    }

    pub fn active(&self) -> bool {
        !self.input.value().trim().is_empty()
    }

    pub fn shown(&self) -> &[usize] {
        &self.shown
    }

    pub fn current(&self) -> Option<usize> {
        self.list
            .selected()
            .and_then(|at| self.shown.get(at).copied())
    }

    pub fn list(&self) -> &List {
        &self.list
    }

    pub fn list_mut(&mut self) -> &mut List {
        &mut self.list
    }

    pub fn sift<R>(
        &mut self,
        rows: &[R],
        keeps: impl Fn(&R, &str) -> bool,
        line: impl Fn(&R) -> Row,
    ) {
        let kept = self.current();
        let query = self.input.value();
        self.shown = rows
            .iter()
            .enumerate()
            .filter(|(_, row)| keeps(row, query))
            .map(|(index, _)| index)
            .collect();
        let lines: Vec<Row> = self.shown.iter().map(|&index| line(&rows[index])).collect();
        self.list.set_rows(lines);
        let at = kept
            .and_then(|kept| self.shown.iter().position(|&index| index == kept))
            .unwrap_or(0);
        self.list.select(at);
    }

    pub fn key(&mut self, key: KeyEvent) -> Turn {
        if self.typing {
            return match self.input.key(key.into()) {
                Edit::Changed => Turn::Filtered,
                Edit::Submit(_) => {
                    self.typing = false;
                    Turn::Taken
                }
                Edit::Cancel => {
                    self.typing = false;
                    self.input.clear();
                    Turn::Filtered
                }
                Edit::Pass => match self.list.key(key.into()) {
                    Step::Pass | Step::Open(_) => Turn::Pass,
                    _ => Turn::Taken,
                },
                _ => Turn::Taken,
            };
        }
        let pressed: Key = key.into();
        if self.start.contains(&pressed) {
            self.typing = true;
            return Turn::Taken;
        }
        if self.active() && self.clear.contains(&pressed) {
            self.input.clear();
            return Turn::Filtered;
        }
        match self.list.key(key.into()) {
            Step::Pass => Turn::Pass,
            Step::Open(at) => self
                .shown
                .get(at)
                .map_or(Turn::Taken, |&index| Turn::Open(index)),
            _ => Turn::Taken,
        }
    }

    pub fn paste(&mut self, text: &str) -> Turn {
        if !self.typing {
            return Turn::Pass;
        }
        match self.input.paste(text) {
            Edit::Changed => Turn::Filtered,
            _ => Turn::Taken,
        }
    }

    pub fn bar(&self) -> Option<InputBar<'_>> {
        self.typing.then(|| {
            let bar = InputBar::new(&self.label, &self.input)
                .placeholder(&self.placeholder)
                .tone(Tone::Accent);
            if self.hint.is_empty() {
                bar
            } else {
                bar.hint(&self.hint)
            }
        })
    }

    pub fn view<'a>(&'a mut self, columns: &'a [Column<'a>], palette: &Palette) -> ListView<'a> {
        ListView::new(&mut self.list, columns).styles(palette.list())
    }
}

pub fn matches(text: &str, query: &str) -> bool {
    let text = text.to_lowercase();
    query
        .split_whitespace()
        .all(|word| text.contains(&word.to_lowercase()))
}
