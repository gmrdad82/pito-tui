use ratatui::{
    buffer::Buffer,
    layout::{Position, Rect},
    style::{Modifier, Style},
    widgets::Widget,
};

use crate::footer::key::Key;
use crate::footer::styles::{Styles, Tone};
use crate::footer::text::{self, Pen};

const MARK: &str = "▸ ";
const BEFORE: u16 = 2;
const BETWEEN: u16 = 3;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Answer {
    Yes,
    No,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct ConfirmKeys {
    pub yes: &'static [Key],
    pub no: &'static [Key],
    pub choose: bool,
    pub toggle: &'static [Key],
    pub accept: &'static [Key],
}

impl ConfirmKeys {
    pub const HEY: ConfirmKeys = ConfirmKeys {
        yes: &[Key::Char('y'), Key::Char('Y')],
        no: &[Key::Char('n'), Key::Char('N'), Key::Esc],
        choose: true,
        toggle: &[
            Key::Left,
            Key::Right,
            Key::Tab,
            Key::BackTab,
            Key::Char('h'),
            Key::Char('l'),
        ],
        accept: &[Key::Enter],
    };

    pub const ENTER: ConfirmKeys = ConfirmKeys {
        yes: &[Key::Enter],
        no: &[Key::Esc],
        choose: false,
        toggle: &[],
        accept: &[],
    };

    pub const fn yes(mut self, keys: &'static [Key]) -> Self {
        self.yes = keys;
        self
    }

    pub const fn no(mut self, keys: &'static [Key]) -> Self {
        self.no = keys;
        self
    }

    pub const fn choose(mut self, choose: bool) -> Self {
        self.choose = choose;
        self
    }

    pub const fn toggle(mut self, keys: &'static [Key]) -> Self {
        self.toggle = keys;
        self
    }

    pub const fn accept(mut self, keys: &'static [Key]) -> Self {
        self.accept = keys;
        self
    }
}

impl Default for ConfirmKeys {
    fn default() -> Self {
        ConfirmKeys::HEY
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Confirm {
    yes: bool,
    keys: ConfirmKeys,
}

impl Default for Confirm {
    fn default() -> Self {
        Confirm::new()
    }
}

impl Confirm {
    pub const fn new() -> Self {
        Confirm {
            yes: false,
            keys: ConfirmKeys::HEY,
        }
    }

    pub const fn keys(mut self, keys: ConfirmKeys) -> Self {
        self.keys = keys;
        self
    }

    pub const fn start(mut self, answer: Answer) -> Self {
        self.yes = matches!(answer, Answer::Yes);
        self
    }

    pub fn yes(&self) -> bool {
        self.yes
    }

    pub fn choosing(&self) -> bool {
        self.keys.choose
    }

    pub fn key(&mut self, key: Key) -> Option<Answer> {
        if self.keys.yes.contains(&key) {
            return Some(Answer::Yes);
        }
        if self.keys.no.contains(&key) {
            return Some(Answer::No);
        }
        if !self.keys.choose {
            return None;
        }
        if self.keys.accept.contains(&key) {
            return Some(if self.yes { Answer::Yes } else { Answer::No });
        }
        if self.keys.toggle.contains(&key) {
            self.yes = !self.yes;
        }
        None
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Words<'a> {
    pub yes: &'a str,
    pub no: &'a str,
    pub hint: Option<&'a str>,
}

impl<'a> Words<'a> {
    pub const fn new(yes: &'a str, no: &'a str) -> Self {
        Words {
            yes,
            no,
            hint: None,
        }
    }

    pub const fn hint(mut self, hint: &'a str) -> Self {
        self.hint = Some(hint);
        self
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ConfirmBar<'a> {
    question: &'a str,
    confirm: Confirm,
    words: Words<'a>,
    rule: bool,
    inline: bool,
    tone: Tone,
    styles: Option<Styles>,
}

impl<'a> ConfirmBar<'a> {
    pub fn new(question: &'a str, confirm: &Confirm, words: Words<'a>) -> Self {
        ConfirmBar {
            question,
            confirm: *confirm,
            words,
            rule: true,
            inline: false,
            tone: Tone::Ink,
            styles: None,
        }
    }

    pub fn rule(mut self, rule: bool) -> Self {
        self.rule = rule;
        self
    }

    pub fn inline(mut self, inline: bool) -> Self {
        self.inline = inline;
        self
    }

    pub fn tone(mut self, tone: Tone) -> Self {
        self.tone = tone;
        self
    }

    pub fn styles(mut self, styles: Styles) -> Self {
        self.styles = Some(styles);
        self
    }

    pub(crate) fn styled(mut self, fallback: Styles) -> Self {
        self.styles = Some(self.styles.unwrap_or(fallback));
        self
    }

    fn look(&self) -> Styles {
        self.styles.unwrap_or_default()
    }

    pub fn height(&self) -> u16 {
        let hint = !self.inline && self.words.hint.is_some();
        1 + u16::from(self.rule) + u16::from(hint)
    }

    fn choices_width(&self) -> u16 {
        if !self.confirm.keys.choose {
            return 0;
        }
        BEFORE
            .saturating_add(text::width(MARK))
            .saturating_add(text::width(self.words.yes))
            .saturating_add(BETWEEN)
            .saturating_add(text::width(self.words.no))
    }

    fn question_style(&self) -> Style {
        self.tone.style(&self.look()).add_modifier(Modifier::BOLD)
    }

    fn draw_choices(&self, pen: &mut Pen) {
        if !self.confirm.keys.choose {
            return;
        }
        pen.skip(BEFORE);
        let styles = self.look();
        let lit = styles.lit();
        let muted = styles.muted;
        if self.confirm.yes {
            pen.put(MARK, lit);
            pen.put(self.words.yes, lit);
            pen.skip(BETWEEN);
            pen.put(self.words.no, muted);
        } else {
            pen.put(self.words.yes, muted);
            pen.skip(BETWEEN);
            pen.put(MARK, lit);
            pen.put(self.words.no, lit);
        }
    }
}

impl Widget for ConfirmBar<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        (&self).render(area, buf);
    }
}

impl Widget for &ConfirmBar<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        self.paint(area, buf);
    }
}

impl ConfirmBar<'_> {
    pub(crate) fn paint(&self, area: Rect, buf: &mut Buffer) -> Option<Position> {
        if area.is_empty() {
            return None;
        }
        let mut end = None;
        let mut y = area.y;
        let styles = self.look();
        if self.rule {
            text::rule(buf, area, styles.rule);
            y += 1;
        }
        let muted = styles.muted;
        if let Some(mut pen) = Pen::new(buf, area, area.x, y) {
            let choices = self.choices_width();
            let room = area.width.saturating_sub(choices);
            let least = text::width(self.question).min(area.width / 2).max(1);
            if room >= least {
                let stop = pen.x.saturating_add(room);
                pen.clip(self.question, self.question_style(), room);
                if text::width(self.question) > room {
                    pen.skip(stop.saturating_sub(pen.x));
                }
                self.draw_choices(&mut pen);
            } else {
                pen.clip(self.question, self.question_style(), area.width);
            }
            if self.inline
                && let Some(hint) = self.words.hint
            {
                pen.indented(hint, muted);
                return Some(Position::new(pen.x, y));
            }
            end = Some(Position::new(pen.x, y));
            y += 1;
        }
        if let Some(hint) = self.words.hint
            && let Some(mut pen) = Pen::new(buf, area, area.x, y)
        {
            pen.indented(hint, muted);
            end = Some(Position::new(pen.x, y));
        }
        end
    }
}
