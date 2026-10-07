use std::time::Duration;

use pito_footer::{ConfirmBar, Footer, Hint, Notice, Words as Choice};
use pito_header::{Breadcrumb, Drill, Header};
use ratatui::{
    Frame,
    buffer::Buffer,
    layout::{Alignment, Rect},
    style::{Modifier, Style},
    text::{Line, Text},
    widgets::{Block, Paragraph, Widget, Wrap},
};

use crate::screen::{Phase, Screen};
use crate::shell::{Role, Tui};
use crate::text::cells;
use crate::words::Words;

pub const TOP: u16 = 1;
pub const SIDE: u16 = 1;
pub const PAD: u16 = 2;
pub const GAP: u16 = 1;

const SIDES: u16 = 6;
const LEAD: &str = " · ";

fn width(text: &str) -> u16 {
    u16::try_from(cells(text)).unwrap_or(u16::MAX)
}

fn room(line: u16, title: &str) -> u16 {
    let wide = width(title).saturating_add(2).min(line);
    let side = ((line - wide) / 2).saturating_sub(2);
    if side < SIDES { 0 } else { side - 2 }
}

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

fn choice<'a>(words: &'a Words, hint: &'a str) -> Choice<'a> {
    let choice = Choice::new(&words.yes, &words.no);
    if hint.is_empty() {
        choice
    } else {
        choice.hint(hint)
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
        let hints_open = self.help.is_none_or(|help| help.open());
        let notice = again
            .map(Notice::accent)
            .or_else(|| {
                self.out
                    .said
                    .as_ref()
                    .map(|(text, tone)| Notice::new(text, *tone))
            })
            .or_else(|| screen.notice())
            .or_else(|| screen.legend().filter(|_| hints_open).map(Notice::legend));
        let words = &self.words;
        let leave = if words.leave.is_empty() {
            &words.choose
        } else {
            &words.leave
        };
        let mut asked = choice(words, &words.choose);
        let mut leaving = choice(words, leave);
        if let Some(text) = again.filter(|_| self.lit_again) {
            asked = asked.hint(text);
            leaving = leaving.hint(text);
        }
        let confirm = self.quit.bar(leaving).or_else(|| {
            self.out
                .asking
                .as_ref()
                .map(|asking| ConfirmBar::new(&asking.question, &asking.confirm, asked))
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

    fn left(&self, screen: &dyn Screen<E>, room: u16) -> Vec<(String, Style)> {
        let help = self
            .help
            .filter(|help| !help.open())
            .map(|_| self.words.help.as_ref())
            .filter(|text| !text.is_empty());
        if let Some(left) = screen.left(room, help) {
            return left;
        }
        let taken = help.map_or(0, |text| width(text).saturating_add(width(LEAD)));
        let lead = screen.lead(room.saturating_sub(taken));
        let mut left = Vec::new();
        if let Some(text) = help {
            left.push((text.to_string(), self.palette.muted));
            if lead.iter().any(|(text, _)| !text.is_empty()) {
                left.push((LEAD.to_string(), self.palette.muted));
            }
        }
        left.extend(lead);
        left
    }

    pub(crate) fn header<'a>(
        &'a self,
        facts: &'a [(String, Style)],
        left: &'a [(String, Style)],
        status: &'a [(String, Style)],
    ) -> Header<'a> {
        let header = Header::new(&self.nav)
            .styles(self.palette.header())
            .title(Some(self.name.as_ref()))
            .left_pairs(left)
            .right_pairs(status)
            .facts_pairs(facts);
        let header = match self.header_look {
            Some(look) => look(header),
            None => header,
        };
        if self.open.is_some() {
            header.lit(false).drill(Drill::Rows).crumbs(false)
        } else {
            header
        }
    }

    fn facts_row<'a>(&'a self, facts: &'a [(String, Style)]) -> Header<'a> {
        self.header(facts, &[], &[])
            .title(None)
            .tabs(false)
            .crumbs(false)
            .closing(false)
            .notice(None)
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
        let moment = self.moment;
        self.screens[index].screen.moment(moment);
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
        let room = room(width, &self.name);
        let left = self.left(screen.as_ref(), room);
        let status = screen.status_parts(room);
        let header = self.header(&facts, &left, &status);
        let top = Rect::new(
            area.x + SIDE,
            area.y + TOP,
            width,
            header.height().min(area.height.saturating_sub(TOP)),
        );
        let dropped = header.height() == header.facts_pairs(&[]).height();
        frame.render_widget(header, top);
        self.top = top;
        let mut below = top.bottom();
        let overlay = self
            .open
            .and_then(|open| match &self.screens.get(open)?.role {
                Role::Overlay(nav, _) => Some(nav),
                Role::Tab => None,
            });
        if below < area.bottom() && (overlay.is_some() || self.nav.depth() == 0) {
            let nav = overlay.unwrap_or(&self.nav);
            let rule = Breadcrumb::new(nav).styles(self.palette.header());
            frame.render_widget(rule, Rect::new(top.x, below, width, 1));
            below += 1;
        } else if self.keep_facts && dropped && below < area.bottom() {
            let row = self.facts_row(&facts);
            let height = row.height().min(area.bottom() - below);
            frame.render_widget(row, Rect::new(top.x, below, width, height));
            below += height;
        }
        let mut hints = match &self.out.picking {
            Some(picking) => picking.pick.hints.clone(),
            None => screen.hints(),
        };
        hints.extend(self.hints.iter().copied());
        let again = self.quit.notice().filter(|text| !text.is_empty());
        let footer = self.footer(&hints, screen.as_ref(), again);
        let height = footer
            .height(width)
            .min(area.bottom().saturating_sub(below));
        let bottom = area.bottom().saturating_sub(height);
        let place = Rect::new(area.x + SIDE, bottom, width, height);
        frame.render_widget(footer, place);
        if again.is_some()
            && self.lit_again
            && (screen.input().is_some() || self.out.asking.is_some())
        {
            let mut plain = Buffer::empty(place);
            self.footer(&hints, screen.as_ref(), Some(""))
                .render(place, &mut plain);
            light(frame.buffer_mut(), &plain, self.palette.accent);
        }
        let inner = area.width.saturating_sub(2 * PAD);
        let mut floor = bottom;
        let room = bottom.saturating_sub(below);
        if let Some(band) = self.band.as_mut() {
            let rows = band.height(room, moment).min(room.saturating_sub(GAP));
            if rows > 0 {
                floor = bottom - rows;
                let place = Rect::new(area.x + PAD, floor, inner, rows);
                band.draw(frame, place, &self.palette, moment);
            }
        } else if let Some(board) = self.board().filter(|_| self.open.is_none()) {
            let shown = board.banded(moment).len();
            let rows = u16::try_from(shown).unwrap_or(u16::MAX).min(room / 2);
            if rows > 0 {
                floor = bottom - rows;
                let place = Rect::new(area.x + PAD, floor, inner, rows);
                board.band(frame, place, &self.palette, moment);
            }
        }
        let content = Rect::new(
            area.x + PAD,
            below,
            inner,
            floor.saturating_sub(below + GAP),
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
            }
            Phase::Trouble(lines) => message(frame, content, lines.into_owned()),
            _ => self.screens[index]
                .screen
                .draw(frame, content, &self.palette),
        }
        if let Some(picking) = self.out.picking.as_mut() {
            picking.pick.draw(frame, content, &self.palette);
        }
    }
}
