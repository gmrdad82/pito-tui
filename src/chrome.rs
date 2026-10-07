use std::time::Duration;

use pito_footer::{ConfirmBar, Footer, Hint, Notice, Words as Choice};
use pito_header::{Breadcrumb, Drill, Header};
use ratatui::{
    Frame,
    buffer::Buffer,
    layout::{Alignment, Rect},
    style::{Modifier, Style},
    text::{Line, Text},
    widgets::{Block, BorderType, Clear, Paragraph, Widget, Wrap},
};

use crate::head::{self, Focus, Item, Kind, Labels, Numbers, Row, Rule};
use crate::label::Span;
use crate::layout::{Edge, PAD, Spot};
use crate::screen::{Phase, Screen};
use crate::shell::{Role, Tui};
use crate::text::cells;
use crate::words::Words;

const SIDES: u16 = 6;
const LEAD: &str = " · ";
const TOAST: u16 = 3;

fn width(text: &str) -> u16 {
    u16::try_from(cells(text)).unwrap_or(u16::MAX)
}

fn room(line: u16, title: &str) -> u16 {
    let wide = width(title).saturating_add(2).min(line);
    let side = ((line - wide) / 2).saturating_sub(2);
    if side < SIDES { 0 } else { side - 2 }
}

pub fn message<'a>(frame: &mut Frame, area: Rect, text: impl Into<Text<'a>>) {
    message_at(frame, area, text, Spot::THIRD);
}

pub fn message_at<'a>(frame: &mut Frame, area: Rect, text: impl Into<Text<'a>>, spot: Spot) {
    let text = text.into();
    let inner = area.width.saturating_sub(2 * PAD);
    let x = area.x.saturating_add(PAD.min(area.width));
    let lines: usize = text
        .lines
        .iter()
        .map(|line| line.width().div_ceil(usize::from(inner.max(1))).max(1))
        .sum();
    let lines = u16::try_from(lines).unwrap_or(u16::MAX).min(area.height);
    let top = match spot.y {
        Edge::Start => 0,
        Edge::Third => area.height / 3,
        Edge::Middle => (area.height - lines) / 2,
        Edge::End => area.height - lines,
    };
    let alignment = match spot.x {
        Edge::Start => Alignment::Left,
        Edge::End => Alignment::Right,
        Edge::Third | Edge::Middle => Alignment::Center,
    };
    let place = Rect::new(x, area.y + top, inner, area.height - top);
    let paragraph = Paragraph::new(text)
        .alignment(alignment)
        .wrap(Wrap { trim: true });
    frame.render_widget(paragraph, place);
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

fn spans(parts: &[(String, Style)]) -> Vec<Span> {
    parts
        .iter()
        .map(|(text, style)| (text.clone().into(), *style))
        .collect()
}

impl<E: Send + 'static> Tui<E> {
    pub(crate) fn title_text(&self) -> String {
        self.title.iter().map(|(text, _)| text.as_ref()).collect()
    }

    fn footer<'a>(
        &'a self,
        hints: &'a [Hint<'a>],
        screen: &'a dyn Screen<E>,
        again: Option<&'a str>,
        width: u16,
    ) -> Footer<'a> {
        let hints_open = self.help.is_none_or(|help| help.open());
        let notice = again
            .map(|text| Notice::new(text, self.quit_tone))
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
            .styles(self.drawn.footer())
            .version(&self.name, &self.version)
            .notice(notice)
            .confirm(confirm)
            .input(input);
        if let Some(help) = &self.help
            && !self.gated
        {
            footer = footer.help(help);
        }
        if self.version_fit {
            let bare = footer.version("", "");
            if again.is_some() || bare.height(width) < footer.height(width) {
                footer = bare;
            }
        }
        match self.footer_look {
            Some(look) => look(footer, width),
            None => footer,
        }
    }

    fn sides(&self, index: usize) -> (&dyn Screen<E>, &dyn Screen<E>) {
        let screen = self.screens[index].screen.as_ref();
        let Some(app) = self.app_slot().map(|app| self.screens[app].screen.as_ref()) else {
            return (screen, screen);
        };
        (app, screen)
    }

    fn left(&self, index: usize, room: u16) -> Vec<(String, Style)> {
        let help = self
            .help
            .filter(|help| !help.open() && !self.gated)
            .map(|_| self.words.help.as_ref())
            .filter(|text| !text.is_empty());
        let (app, screen) = self.sides(index);
        for source in [app, screen] {
            if let Some(left) = source.left(room, help) {
                return left;
            }
        }
        let taken = help.map_or(0, |text| width(text).saturating_add(width(LEAD)));
        let mut lead = app.lead(room.saturating_sub(taken));
        if lead.iter().all(|(text, _)| text.is_empty()) {
            lead = screen.lead(room.saturating_sub(taken));
        }
        let mut left = Vec::new();
        if let Some(text) = help {
            left.push((text.to_string(), self.drawn.muted));
            if lead.iter().any(|(text, _)| !text.is_empty()) {
                left.push((LEAD.to_string(), self.drawn.muted));
            }
        }
        left.extend(lead);
        left
    }

    fn right(&self, index: usize, room: u16) -> Vec<(String, Style)> {
        let (app, screen) = self.sides(index);
        let parts = app.status_parts(room);
        if parts.iter().any(|(text, _)| !text.is_empty()) {
            return parts;
        }
        screen.status_parts(room)
    }

    pub(crate) fn header<'a>(
        &'a self,
        title: &'a str,
        facts: &'a [(String, Style)],
        left: &'a [(String, Style)],
        status: &'a [(String, Style)],
    ) -> Header<'a> {
        let header = Header::new(&self.model.nav)
            .styles(self.drawn.header())
            .title(Some(title))
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
        self.header("", facts, &[], &[])
            .title(None)
            .tabs(false)
            .crumbs(false)
            .closing(false)
            .notice(None)
    }

    pub(crate) fn tail(&self) -> Vec<(String, Style)> {
        self.current()
            .and_then(|index| self.screens.get(index))
            .map(|slot| slot.screen.after_tabs())
            .unwrap_or_default()
    }

    pub(crate) fn group_labels(&self) -> Labels<'_> {
        let place = self.model.nav.place();
        let palette = &self.drawn;
        let items = self
            .model
            .groups
            .iter()
            .enumerate()
            .map(|(group, heading)| Item {
                label: &self.model.headings[*heading].label,
                number: None,
                on: group == place.group,
            })
            .collect();
        let on = if self.focus == Focus::Groups {
            palette.focus
        } else {
            palette.accent.add_modifier(Modifier::BOLD)
        };
        Labels {
            items,
            kind: Kind::Groups,
            on,
            off: palette.inactive,
            tail: &[],
            tail_style: Style::new(),
        }
    }

    pub(crate) fn section_labels<'a>(&'a self, tail: &'a [(String, Style)]) -> Labels<'a> {
        let palette = &self.drawn;
        let place = self.model.nav.place();
        let tabs = self.model.group_tabs();
        let numbers = self
            .head
            .as_ref()
            .map_or(Numbers::Across, |head| head.numbers);
        let lone = self.head.as_ref().is_some_and(|head| head.lone) && tabs.len() == 1;
        let offset = place.number.saturating_sub(place.section + 1);
        let items = tabs
            .iter()
            .enumerate()
            .map(|(at, tab)| Item {
                label: &self.model.tabs[*tab].label,
                number: match numbers {
                    _ if lone => None,
                    Numbers::Across => Some(offset + at + 1),
                    Numbers::InGroup => Some(at + 1),
                    Numbers::Off => None,
                },
                on: at == place.section,
            })
            .collect();
        let lit = self.open.is_none();
        let on = match (lit, self.focus) {
            (false, _) => palette.inactive,
            (true, Focus::Sections) => palette.focus,
            (true, _) => palette.accent.add_modifier(Modifier::BOLD),
        };
        let tail_style = if self.focus == Focus::Slot {
            palette.focus
        } else {
            Style::new()
        };
        Labels {
            items,
            kind: Kind::Sections,
            on,
            off: palette.inactive,
            tail,
            tail_style,
        }
    }

    fn title_rule(
        &self,
        buf: &mut Buffer,
        line: Rect,
        title: &[Span],
        left: &[(String, Style)],
        right: &[(String, Style)],
    ) {
        Rule {
            rule: self.drawn.rule,
            base: self.drawn.title,
            middle: title,
            left,
            right,
        }
        .draw(buf, line);
    }

    #[allow(clippy::too_many_arguments)]
    fn stack(
        &self,
        buf: &mut Buffer,
        line: Rect,
        bottom: u16,
        index: usize,
        facts: &[(String, Style)],
        left: &[(String, Style)],
        status: &[(String, Style)],
    ) -> (u16, Vec<(Row, Rect)>) {
        let Some(head) = self.head.as_ref() else {
            return (line.y, Vec::new());
        };
        let palette = &self.drawn;
        let overlay = self
            .open
            .and_then(|open| match &self.screens.get(open)?.role {
                Role::Overlay(nav, _) => Some(nav),
                _ => None,
            });
        let nav = &self.model.nav;
        let mut y = line.y;
        let mut rows = Vec::new();
        for row in &head.rows {
            if y >= bottom {
                break;
            }
            let at = Rect { y, ..line };
            match row {
                Row::Title => {
                    self.title_rule(buf, at, &self.title, left, status);
                }
                Row::Groups => {
                    if self.model.groups.len() < 2 {
                        continue;
                    }
                    self.group_labels().draw(buf, at);
                    rows.push((Row::Groups, at));
                }
                Row::Caption => {
                    let caption = self.screens[index]
                        .screen
                        .caption()
                        .map(|parts| spans(&parts));
                    let group = self.model.groups.get(nav.place().group).map(|heading| {
                        let name = self.model.headings[*heading].label.text().to_string();
                        vec![(name.into(), Style::new())]
                    });
                    let middle = caption.or(group).unwrap_or_default();
                    Rule {
                        rule: palette.rule,
                        base: palette.accent.add_modifier(Modifier::BOLD),
                        middle: &middle,
                        left: &[],
                        right: &[],
                    }
                    .draw(buf, at);
                }
                Row::Sections => {
                    if nav.count() == 0 {
                        continue;
                    }
                    if overlay.is_none() && nav.depth() > 0 {
                        Breadcrumb::new(nav)
                            .styles(palette.header())
                            .centred(true)
                            .render(at, buf);
                    } else {
                        let tail = self.screens[index].screen.after_tabs();
                        self.section_labels(&tail).draw(buf, at);
                        rows.push((Row::Sections, at));
                    }
                }
                Row::Facts => {
                    if facts.iter().all(|(text, _)| text.is_empty()) {
                        continue;
                    }
                    self.facts_row(facts).render(at, buf);
                }
                Row::Crumbs => {
                    if overlay.is_none() && nav.depth() > 0 {
                        continue;
                    }
                    Breadcrumb::new(overlay.unwrap_or(nav))
                        .styles(palette.header())
                        .render(at, buf);
                }
                Row::Blank => head::blank(buf, at, palette.base),
            }
            y += 1;
        }
        (y, rows)
    }

    fn time_glass(&mut self, index: usize) {
        let moment = self.moment;
        let (delay, least) = (self.delay, self.least);
        let slot = &mut self.screens[index];
        let label = match slot.screen.phase() {
            Phase::Loading(label) => Some(label.into_owned()),
            _ => None,
        };
        match label {
            Some(label) => {
                let since = *slot.since.get_or_insert(moment);
                if slot.glass.is_none() && moment.saturating_duration_since(since) >= delay {
                    slot.glass = Some(moment);
                }
                slot.label = label;
            }
            None => {
                slot.since = None;
                if slot
                    .glass
                    .is_some_and(|shown| moment.saturating_duration_since(shown) >= least)
                {
                    slot.glass = None;
                }
            }
        }
    }

    fn draw_takeover(&self, frame: &mut Frame, area: Rect) {
        let layout = self.layout;
        let palette = self.drawn;
        let width = area.width.saturating_sub(2 * layout.side);
        let x = area.x + layout.side;
        let bottom = area.bottom().saturating_sub(layout.bottom);
        let top = (area.y + layout.top).min(bottom);
        let Some((_, takeover)) = self.out.takeover.as_ref() else {
            return;
        };
        if top < bottom {
            Rule {
                rule: palette.rule,
                base: palette.title,
                middle: &takeover.title,
                left: &[],
                right: &[],
            }
            .draw(frame.buffer_mut(), Rect::new(x, top, width, 1));
        }
        let again = self.quit.notice().filter(|text| !text.is_empty());
        let mut floor = bottom;
        let words = &self.words;
        let leave = if words.leave.is_empty() {
            &words.choose
        } else {
            &words.leave
        };
        let confirm = self.quit.bar(choice(words, leave));
        if again.is_some() || confirm.is_some() {
            let footer = Footer::new(&[])
                .styles(palette.footer())
                .notice(again.map(|text| Notice::new(text, self.quit_tone)))
                .confirm(confirm)
                .rule(false);
            let height = footer.height(width).min(bottom.saturating_sub(top + 1));
            floor = bottom - height;
            frame.render_widget(footer, Rect::new(x, floor, width, height));
        }
        let elapsed = self.taken.map_or(Duration::ZERO, |since| {
            self.moment.saturating_duration_since(since)
        });
        let below = top.saturating_add(1).min(floor);
        let inner = area.width.saturating_sub(2 * layout.pad);
        let place = Rect::new(area.x + layout.pad, below, inner, floor - below);
        let hourglass = palette.hourglass(elapsed, &takeover.label, None);
        frame.render_widget(hourglass, place);
    }

    fn draw_toasts(&self, frame: &mut Frame, area: Rect) {
        let mut stacks: Vec<(Spot, u16)> = Vec::new();
        for (until, toast) in &self.toasts {
            if *until <= self.moment || area.is_empty() {
                continue;
            }
            let wide = width(&toast.text).saturating_add(4).min(area.width);
            let shift = match stacks.iter_mut().find(|(spot, _)| *spot == toast.spot) {
                Some((_, count)) => {
                    *count += 1;
                    *count - 1
                }
                None => {
                    stacks.push((toast.spot, 1));
                    0
                }
            };
            let mut place = toast.spot.place(area, wide, TOAST);
            let by = shift.saturating_mul(TOAST);
            place.y = match toast.spot.y {
                Edge::End => place.y.saturating_sub(by).max(area.y),
                _ => place.y.saturating_add(by),
            };
            if place.bottom() > area.bottom() || place.y < area.y {
                continue;
            }
            let style = self.drawn.tone(toast.tone);
            frame.render_widget(Clear, place);
            let block = Block::bordered()
                .border_type(BorderType::Rounded)
                .border_style(style)
                .style(self.drawn.base);
            let inner = block.inner(place);
            frame.render_widget(block, place);
            let text = Line::styled(toast.text.to_string(), style).alignment(Alignment::Center);
            frame.render_widget(Paragraph::new(text), inner);
        }
    }

    pub fn draw(&mut self, frame: &mut Frame) {
        let area = frame.area();
        let palette = self.drawn;
        frame.render_widget(Block::new().style(palette.base), area);
        if area.width < self.min.0 || area.height < self.min.1 {
            let line = Line::styled(self.words.too_small.as_ref(), palette.small);
            message_at(frame, area, line, self.layout.small);
            return;
        }
        self.follow();
        self.word_quit();
        self.rows.clear();
        if self.out.takeover.is_some() {
            self.draw_takeover(frame, area);
            return;
        }
        let Some(index) = self.current() else {
            return;
        };
        let moment = self.moment;
        self.screens[index].screen.moment(moment);
        self.time_glass(index);
        let layout = self.layout;
        let width = area.width.saturating_sub(2 * layout.side);
        let bottom = area.bottom().saturating_sub(layout.bottom);
        let title = self.title_text();
        let screen = &self.screens[index].screen;
        let facts = screen.facts();
        let room = room(width, &title);
        let left = self.left(index, room);
        let status = self.right(index, room);
        let line = Rect::new(area.x + layout.side, area.y + layout.top, width, 1);
        let mut below;
        if self.gated || self.head.is_some() {
            let top = line.y.min(bottom);
            if self.gated {
                if top < bottom {
                    self.title_rule(frame.buffer_mut(), line, &self.title, &left, &status);
                }
                below = (top + 1).min(bottom);
            } else {
                let (y, rows) = self.stack(
                    frame.buffer_mut(),
                    line,
                    bottom,
                    index,
                    &facts,
                    &left,
                    &status,
                );
                below = y;
                self.rows = rows;
            }
            self.top = Rect::default();
        } else {
            let header = self.header(&title, &facts, &left, &status);
            let top = Rect::new(
                line.x,
                line.y,
                width,
                header.height().min(bottom.saturating_sub(line.y)),
            );
            let dropped = header.height() == header.facts_pairs(&[]).height();
            frame.render_widget(header, top);
            self.top = top;
            below = top.bottom();
            let overlay = self
                .open
                .and_then(|open| match &self.screens.get(open)?.role {
                    Role::Overlay(nav, _) => Some(nav),
                    _ => None,
                });
            if below < bottom && (overlay.is_some() || self.model.nav.depth() == 0) {
                let nav = overlay.unwrap_or(&self.model.nav);
                let rule = Breadcrumb::new(nav).styles(palette.header());
                frame.render_widget(rule, Rect::new(top.x, below, width, 1));
                below += 1;
            } else if self.keep_facts && dropped && below < bottom {
                let row = self.facts_row(&facts);
                let height = row.height().min(bottom - below);
                frame.render_widget(row, Rect::new(top.x, below, width, height));
                below += height;
            }
        }
        below = below.saturating_add(layout.head_gap).min(bottom);
        let screen = &self.screens[index].screen;
        let again = self.quit.notice().filter(|text| !text.is_empty());
        let (mut hints, prompted) = match (self.out.modals.last(), &self.out.picking) {
            (Some(top), _) => (top.modal.hints.clone(), true),
            (None, Some(picking)) => (picking.pick.hints.clone(), true),
            (None, None) => (screen.hints(), false),
        };
        if !self.gated {
            let shown = self.hints.iter().filter(|hint| !prompted || hint.pinned);
            hints.extend(shown.copied());
        }
        if again.is_some() && !self.quit_hints {
            hints.clear();
        }
        let footer = self.footer(&hints, screen.as_ref(), again, width);
        let height = footer.height(width).min(bottom.saturating_sub(below));
        let floor_line = bottom.saturating_sub(height);
        let place = Rect::new(line.x, floor_line, width, height);
        frame.render_widget(footer, place);
        if again.is_some()
            && self.lit_again
            && (screen.input().is_some() || self.out.asking.is_some())
        {
            let mut plain = Buffer::empty(place);
            self.footer(&hints, screen.as_ref(), Some(""), width)
                .render(place, &mut plain);
            light(frame.buffer_mut(), &plain, palette.accent);
        }
        let inner = area.width.saturating_sub(2 * layout.pad);
        let x = area.x + layout.pad;
        let mut floor = floor_line;
        let room = floor_line.saturating_sub(below);
        let band = if self.gated { None } else { self.band.as_mut() };
        if let Some(band) = band {
            let rows = band
                .height(room, moment)
                .min(room.saturating_sub(layout.gap));
            if rows > 0 {
                floor = floor_line - rows;
                band.draw(frame, Rect::new(x, floor, inner, rows), &palette, moment);
            }
        } else if let Some(board) = self.board().filter(|_| self.open.is_none() && !self.gated) {
            let shown = board.banded(moment).len();
            let rows = u16::try_from(shown).unwrap_or(u16::MAX).min(room / 2);
            if rows > 0 {
                floor = floor_line - rows;
                board.band(frame, Rect::new(x, floor, inner, rows), &palette, moment);
            }
        }
        if !self.gated
            && let Some(band) = self.top_band.as_mut()
        {
            let room = floor.saturating_sub(below);
            let rows = band
                .height(room, moment)
                .min(room.saturating_sub(layout.gap + 1));
            if rows > 0 {
                band.draw(frame, Rect::new(x, below, inner, rows), &palette, moment);
                below += rows;
            }
        }
        let content = Rect::new(x, below, inner, floor.saturating_sub(below + layout.gap));
        self.content = content;
        let slot = &self.screens[index];
        let shown = slot
            .glass
            .map(|since| moment.saturating_duration_since(since));
        let phase = slot.screen.phase();
        match (shown, phase) {
            (Some(elapsed), _) => {
                let waiting = Some(self.words.waiting.as_ref());
                let hourglass = palette.hourglass(elapsed, &slot.label, waiting);
                frame.render_widget(hourglass, content);
            }
            (None, Phase::Loading(_)) => {}
            (None, Phase::Trouble(lines)) => message(frame, content, lines.into_owned()),
            (None, _) => self.screens[index].screen.draw(frame, content, &palette),
        }
        if let Some(picking) = self.out.picking.as_mut() {
            picking.pick.draw(frame, content, &palette);
        }
        for opened in &mut self.out.modals {
            opened.modal.draw(frame, content, &palette);
        }
        self.draw_toasts(frame, content);
    }
}
