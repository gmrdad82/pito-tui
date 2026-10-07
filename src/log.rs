use std::borrow::Cow;
use std::sync::Arc;

use crossterm::event::KeyEvent;
use pito_footer::{Input, InputBar};
use pito_list::{Key, Keys};
use ratatui::{Frame, layout::Rect, text::Line, widgets::Paragraph};

use crate::filter::matches;
use crate::palette::Palette;

pub type Lines = Arc<Vec<Line<'static>>>;

const SEARCH: &[Key] = &[Key::Char('/')];
const NEXT: &[Key] = &[Key::Char('n')];
const PREVIOUS: &[Key] = &[Key::Char('N')];
const COPY: &[Key] = &[Key::Char('y')];
const ALL: &[Key] = &[Key::Char('Y')];

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Turn {
    Pass,
    Taken,
    Copy(String),
}

#[derive(Debug, Clone)]
pub struct Log {
    lines: Lines,
    top: usize,
    page: usize,
    input: Input,
    typing: bool,
    hits: Vec<usize>,
    hit: Option<usize>,
    label: Cow<'static, str>,
    placeholder: Cow<'static, str>,
    hint: Cow<'static, str>,
    keys: Keys,
    search: &'static [Key],
    next: &'static [Key],
    previous: &'static [Key],
    copy: &'static [Key],
    all: &'static [Key],
}

fn plain(line: &Line) -> String {
    line.spans
        .iter()
        .map(|span| span.content.as_ref())
        .collect()
}

impl Log {
    pub fn new(label: impl Into<Cow<'static, str>>) -> Self {
        Log {
            lines: Arc::new(Vec::new()),
            top: 0,
            page: 1,
            input: Input::new(),
            typing: false,
            hits: Vec::new(),
            hit: None,
            label: label.into(),
            placeholder: Cow::Borrowed(""),
            hint: Cow::Borrowed(""),
            keys: Keys::VIM,
            search: SEARCH,
            next: NEXT,
            previous: PREVIOUS,
            copy: COPY,
            all: ALL,
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
        self.keys = keys;
        self
    }

    pub fn search_keys(mut self, keys: &'static [Key]) -> Self {
        self.search = keys;
        self
    }

    pub fn hit_keys(mut self, next: &'static [Key], previous: &'static [Key]) -> Self {
        self.next = next;
        self.previous = previous;
        self
    }

    pub fn copy_keys(mut self, page: &'static [Key], all: &'static [Key]) -> Self {
        self.copy = page;
        self.all = all;
        self
    }

    pub fn lines(&self) -> &Lines {
        &self.lines
    }

    pub fn set_lines(&mut self, lines: Lines) {
        if Arc::ptr_eq(&self.lines, &lines) {
            return;
        }
        let following = self.top >= self.last();
        self.lines = lines;
        if following && self.top > 0 {
            self.top = self.last();
        }
        self.top = self.top.min(self.last());
        self.find(false);
    }

    pub fn len(&self) -> usize {
        self.lines.len()
    }

    pub fn is_empty(&self) -> bool {
        self.lines.is_empty()
    }

    pub fn top(&self) -> usize {
        self.top
    }

    pub fn hits(&self) -> usize {
        self.hits.len()
    }

    pub fn hit(&self) -> Option<usize> {
        self.hit
    }

    pub fn query(&self) -> &str {
        self.input.value()
    }

    pub fn typing(&self) -> bool {
        self.typing
    }

    pub fn bar(&self) -> Option<InputBar<'_>> {
        self.typing.then(|| {
            InputBar::new(&self.label, &self.input)
                .placeholder(&self.placeholder)
                .hint(&self.hint)
        })
    }

    fn last(&self) -> usize {
        self.lines.len().saturating_sub(self.page)
    }

    fn show(&mut self, line: usize) {
        if line < self.top || line >= self.top + self.page {
            self.top = line.saturating_sub(self.page / 3).min(self.last());
        }
    }

    fn find(&mut self, jump: bool) {
        let query = self.input.value().trim().to_string();
        self.hits = if query.is_empty() {
            Vec::new()
        } else {
            self.lines
                .iter()
                .enumerate()
                .filter(|(_, line)| matches(&plain(line), &query))
                .map(|(index, _)| index)
                .collect()
        };
        self.hit = None;
        if jump && let Some(at) = self.hits.iter().position(|line| *line >= self.top) {
            self.hit = Some(at);
            self.show(self.hits[at]);
        }
    }

    fn step(&mut self, by: isize) {
        if self.hits.is_empty() {
            return;
        }
        let count = self.hits.len() as isize;
        let at = match self.hit {
            Some(at) => (at as isize + by).rem_euclid(count),
            None if by > 0 => 0,
            None => count - 1,
        } as usize;
        self.hit = Some(at);
        self.show(self.hits[at]);
    }

    fn text(&self, lines: &[Line]) -> String {
        let mut text: Vec<String> = lines.iter().map(|line| plain(line)).collect();
        for line in &mut text {
            line.truncate(line.trim_end().len());
        }
        text.join("\n")
    }

    pub fn key(&mut self, event: KeyEvent) -> Turn {
        let key: Key = event.into();
        if self.typing {
            match key {
                Key::Enter => self.typing = false,
                Key::Esc => {
                    self.typing = false;
                    self.input.clear();
                    self.find(false);
                }
                _ => {
                    let before = self.input.value().to_string();
                    self.input.key(event.into());
                    if self.input.value() != before {
                        self.find(true);
                    }
                }
            }
            return Turn::Taken;
        }
        let page = self.page.max(1);
        if self.keys.up.contains(&key) {
            self.top = self.top.saturating_sub(1);
        } else if self.keys.down.contains(&key) {
            self.top = (self.top + 1).min(self.last());
        } else if self.keys.page_up.contains(&key) {
            self.top = self.top.saturating_sub(page);
        } else if self.keys.page_down.contains(&key) {
            self.top = (self.top + page).min(self.last());
        } else if self.keys.first.contains(&key) {
            self.top = 0;
        } else if self.keys.last.contains(&key) {
            self.top = self.last();
        } else if self.search.contains(&key) {
            self.typing = true;
        } else if self.next.contains(&key) {
            self.step(1);
        } else if self.previous.contains(&key) {
            self.step(-1);
        } else if self.copy.contains(&key) {
            let end = (self.top + page).min(self.lines.len());
            return Turn::Copy(self.text(&self.lines[self.top.min(end)..end]));
        } else if self.all.contains(&key) {
            return Turn::Copy(self.text(&self.lines));
        } else {
            return Turn::Pass;
        }
        Turn::Taken
    }

    pub fn paste(&mut self, text: &str) -> Turn {
        if !self.typing {
            return Turn::Pass;
        }
        self.input.paste(text);
        self.find(true);
        Turn::Taken
    }

    pub fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        self.page = usize::from(area.height).max(1);
        self.top = self.top.min(self.last());
        let current = self.hit.map(|at| self.hits[at]);
        let end = (self.top + self.page).min(self.lines.len());
        let shown: Vec<Line> = (self.top..end)
            .map(|index| {
                let line = self.lines[index].clone();
                if Some(index) == current {
                    line.patch_style(palette.selected)
                } else {
                    line
                }
            })
            .collect();
        frame.render_widget(Paragraph::new(shown), area);
    }
}
