use std::time::Duration;

use pito_footer::{ConfirmBar, Footer, Hint, Notice, Words as Choice};
use pito_header::{Breadcrumb, Fact, Header};
use ratatui::{
    Frame,
    buffer::Buffer,
    layout::{Alignment, Rect},
    style::{Modifier, Style},
    text::{Line, Text},
    widgets::{Block, Paragraph, Widget, Wrap},
};

use crate::screen::{Phase, Screen};
use crate::shell::Tui;
use crate::words::Words;

pub const TOP: u16 = 1;
pub const SIDE: u16 = 1;
pub const PAD: u16 = 2;
pub const GAP: u16 = 1;

pub fn message<'a>(frame: &mut Frame, area: Rect, text: impl Into<Text<'a>>) {
    let top = area.height / 3;
    let inner = Rect::new(
        area.x.saturating_add(PAD.min(area.width)),
        area.y.saturating_add(top),
        area.width.saturating_sub(2 * PAD),
        area.height - top,
    );
    let paragraph = Paragraph::new(text)
        .alignment(Alignment::Center)
        .wrap(Wrap { trim: true });
    frame.render_widget(paragraph, inner);
}

fn choice(words: &Words) -> Choice<'_> {
    let choice = Choice::new(&words.yes, &words.no);
    if words.choose.is_empty() {
        choice
    } else {
        choice.hint(&words.choose)
    }
}

fn light(buffer: &mut Buffer, plain: &Buffer, accent: Style) {
    let lit = accent.add_modifier(Modifier::BOLD);
    let area = plain.area;
    for y in area.top()..area.bottom() {
        let changed = |x: &u16| {
            let symbol = buffer[(*x, y)].symbol();
            symbol != " " && symbol != plain[(*x, y)].symbol()
        };
        let first = (area.left()..area.right()).find(changed);
        let last = (area.left()..area.right()).rev().find(changed);
        let (Some(first), Some(last)) = (first, last) else {
            continue;
        };
        for x in first..=last {
            let cell = &mut buffer[(x, y)];
            cell.modifier = Modifier::empty();
            cell.set_style(lit);
        }
    }
}

impl<E: Send + 'static> Tui<E> {
    fn footer<'a>(
        &'a self,
        hints: &'a [Hint<'a>],
        screen: &'a dyn Screen<E>,
        again: Option<&'a str>,
    ) -> Footer<'a> {
        let notice = again.map(Notice::accent).or_else(|| {
            self.out
                .said
                .as_ref()
                .map(|(text, tone)| Notice::new(text, *tone))
        });
        let mut choice = choice(&self.words);
        if let Some(text) = again {
            choice = choice.hint(text);
        }
        let confirm = self.quit.bar(choice).or_else(|| {
            self.out
                .asking
                .as_ref()
                .map(|asking| ConfirmBar::new(&asking.question, &asking.confirm, choice))
        });
        let input = screen
            .input()
            .map(|bar| again.map_or(bar, |text| bar.hint(text)));
        let mut footer = Footer::new(hints)
            .styles(self.palette.footer())
            .version(&self.name, &self.version)
            .notice(notice)
            .confirm(confirm)
            .input(input);
        if let Some(help) = &self.help {
            footer = footer.help(help);
        }
        match self.footer_look {
            Some(look) => look(footer),
            None => footer,
        }
    }

    pub(crate) fn header<'a>(
        &'a self,
        facts: &'a [(String, Style)],
        status: Option<&'a (String, Style)>,
    ) -> Header<'a> {
        let left = self
            .help
            .filter(|help| !help.open())
            .map(|_| self.words.help.as_ref())
            .filter(|text| !text.is_empty());
        let header = Header::new(&self.nav)
            .styles(self.palette.header())
            .title(Some(self.name.as_ref()))
            .left(left.map(|text| Fact::new(text, self.palette.muted)))
            .right(status.map(|(text, style)| Fact::new(text, *style)))
            .facts_pairs(facts);
        match self.header_look {
            Some(look) => look(header),
            None => header,
        }
    }

    pub fn draw(&mut self, frame: &mut Frame) {
        let area = frame.area();
        frame.render_widget(Block::new().style(self.palette.base), area);
        if area.width < self.min.0 || area.height < self.min.1 {
            let line = Line::styled(self.words.too_small.as_ref(), self.palette.ink);
            message(frame, area, line);
            return;
        }
        self.follow();
        self.word_quit();
        let Some(index) = self.current() else {
            return;
        };
        let loading = self.loading(index);
        let slot = &mut self.screens[index];
        match (loading, slot.since) {
            (true, None) => slot.since = Some(self.moment),
            (false, Some(_)) => slot.since = None,
            _ => {}
        }
        let width = area.width.saturating_sub(2 * SIDE);
        let screen = &self.screens[index].screen;
        let facts = screen.facts();
        let status = screen.status();
        let header = self.header(&facts, status.as_ref());
        let top = Rect::new(
            area.x + SIDE,
            area.y + TOP,
            width,
            header.height().min(area.height.saturating_sub(TOP)),
        );
        frame.render_widget(header, top);
        self.top = top;
        let mut below = top.bottom();
        if self.nav.depth() == 0 && below < area.bottom() {
            let rule = Breadcrumb::new(&self.nav).styles(self.palette.header());
            frame.render_widget(rule, Rect::new(top.x, below, width, 1));
            below += 1;
        }
        let mut hints = screen.hints();
        hints.extend(self.hints.iter().copied());
        let again = self.quit.notice().filter(|text| !text.is_empty());
        let footer = self.footer(&hints, screen.as_ref(), again);
        let height = footer
            .height(width)
            .min(area.bottom().saturating_sub(below));
        let bottom = area.bottom().saturating_sub(height);
        let place = Rect::new(area.x + SIDE, bottom, width, height);
        frame.render_widget(footer, place);
        if again.is_some() && (screen.input().is_some() || self.out.asking.is_some()) {
            let mut plain = Buffer::empty(place);
            self.footer(&hints, screen.as_ref(), Some(""))
                .render(place, &mut plain);
            light(frame.buffer_mut(), &plain, self.palette.accent);
        }
        let content = Rect::new(
            area.x + PAD,
            below,
            area.width.saturating_sub(2 * PAD),
            bottom.saturating_sub(below + GAP),
        );
        self.content = content;
        let elapsed = self.screens[index].since.map_or(Duration::ZERO, |since| {
            self.moment.saturating_duration_since(since)
        });
        match self.screens[index].screen.phase() {
            Phase::Loading(label) => {
                let waiting = Some(self.words.waiting.as_ref());
                let hourglass = self.palette.hourglass(elapsed, &label, waiting);
                frame.render_widget(hourglass, content);
                return;
            }
            Phase::Trouble(lines) => {
                message(frame, content, lines.into_owned());
                return;
            }
            _ => {}
        }
        self.screens[index]
            .screen
            .draw(frame, content, &self.palette);
    }
}
