use ratatui::{
    buffer::Buffer,
    layout::{Position, Rect},
    style::Style,
    widgets::Widget,
};

use crate::footer::confirm::ConfirmBar;
use crate::footer::input::InputBar;
use crate::footer::key::Key;
use crate::footer::styles::{Styles, Tone};
use crate::footer::text::{self, Pen};

const SEPARATOR: &str = " · ";
const MOST: usize = 64;
const BESIDE: u16 = 2;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Hint<'a> {
    pub key: &'a str,
    pub label: &'a str,
    pub lead: &'a str,
    pub rank: u8,
    pub pinned: bool,
}

impl<'a> Hint<'a> {
    pub const fn new(key: &'a str, label: &'a str) -> Self {
        Hint {
            key,
            label,
            lead: "",
            rank: 0,
            pinned: false,
        }
    }

    pub const fn rank(mut self, rank: u8) -> Self {
        self.rank = rank;
        self
    }

    pub const fn lead(mut self, lead: &'a str) -> Self {
        self.lead = lead;
        self
    }

    pub const fn pinned(mut self) -> Self {
        self.pinned = true;
        self
    }

    fn width(&self) -> u16 {
        let mut total = 0u16;
        for part in [self.lead, self.key, self.label] {
            if part.is_empty() {
                continue;
            }
            if total > 0 {
                total = total.saturating_add(1);
            }
            total = total.saturating_add(text::width(part));
        }
        total
    }

    fn draw(&self, pen: &mut Pen, key: Style, muted: Style, room: u16) {
        let stop = pen.x.saturating_add(room);
        let mut first = true;
        for (part, style) in [(self.lead, muted), (self.key, key), (self.label, muted)] {
            if part.is_empty() {
                continue;
            }
            if !first {
                pen.clip(" ", muted, stop.saturating_sub(pen.x));
            }
            first = false;
            let start = pen.x;
            pen.clip(part, style, stop.saturating_sub(pen.x));
            if pen.x - start < text::width(part) {
                break;
            }
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Notice<'a> {
    pub text: &'a str,
    pub tone: Tone,
    pub over: bool,
}

impl<'a> Notice<'a> {
    pub const fn new(text: &'a str, tone: Tone) -> Self {
        Notice {
            text,
            tone,
            over: false,
        }
    }

    pub const fn legend(text: &'a str) -> Self {
        Notice::new(text, Tone::Muted)
    }

    pub const fn accent(text: &'a str) -> Self {
        Notice::new(text, Tone::Accent)
    }

    pub const fn alert(text: &'a str) -> Self {
        Notice::new(text, Tone::Alert)
    }

    pub const fn good(text: &'a str) -> Self {
        Notice::new(text, Tone::Good)
    }

    pub const fn over(mut self) -> Self {
        self.over = true;
        self
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Help {
    open: bool,
    on: Key,
}

impl Default for Help {
    fn default() -> Self {
        Help::new(true)
    }
}

impl Help {
    pub const fn new(open: bool) -> Self {
        Help {
            open,
            on: Key::Char('?'),
        }
    }

    pub const fn toggle_on(mut self, key: Key) -> Self {
        self.on = key;
        self
    }

    pub fn open(&self) -> bool {
        self.open
    }

    pub fn toggle(&mut self) {
        self.open = !self.open;
    }

    pub fn wants(&self, key: Key) -> bool {
        key == self.on
    }

    pub fn key(&mut self, key: Key) -> bool {
        if self.wants(key) {
            self.toggle();
            return true;
        }
        false
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Part<'a> {
    Text(&'a str, Style),
    Spans(&'a [(&'a str, Style)]),
    Hints(&'a [Hint<'a>]),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Segment<'a> {
    pub part: Part<'a>,
    pub rank: u8,
    pub right: bool,
    pub shrink: bool,
}

impl<'a> Segment<'a> {
    pub const fn text(text: &'a str, style: Style) -> Self {
        Segment {
            part: Part::Text(text, style),
            rank: 0,
            right: false,
            shrink: false,
        }
    }

    pub const fn spans(spans: &'a [(&'a str, Style)]) -> Self {
        Segment {
            part: Part::Spans(spans),
            rank: 0,
            right: false,
            shrink: false,
        }
    }

    pub const fn hints(hints: &'a [Hint<'a>]) -> Self {
        Segment {
            part: Part::Hints(hints),
            rank: 0,
            right: false,
            shrink: true,
        }
    }

    pub const fn rank(mut self, rank: u8) -> Self {
        self.rank = rank;
        self
    }

    pub const fn right(mut self) -> Self {
        self.right = true;
        self
    }

    pub const fn shrink(mut self) -> Self {
        self.shrink = true;
        self
    }
}

fn mask(hints: &[Hint], keep: impl Fn(&Hint) -> bool) -> u64 {
    hints
        .iter()
        .take(MOST)
        .enumerate()
        .filter(|(_, hint)| keep(hint))
        .fold(0u64, |kept, (index, _)| kept | 1 << index)
}

#[derive(Debug, Clone, Copy)]
struct Fit {
    kept: u64,
    widths: [u16; MOST],
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Footer<'a> {
    hints: &'a [Hint<'a>],
    segments: &'a [Segment<'a>],
    notice: Option<Notice<'a>>,
    confirm: Option<ConfirmBar<'a>>,
    input: Option<InputBar<'a>>,
    name: &'a str,
    version: &'a str,
    separator: &'a str,
    gap: u16,
    indent: u16,
    open: bool,
    rule: bool,
    wrap: bool,
    styles: Styles,
}

impl<'a> Footer<'a> {
    pub fn new(hints: &'a [Hint<'a>]) -> Self {
        Footer {
            hints,
            segments: &[],
            notice: None,
            confirm: None,
            input: None,
            name: "",
            version: "",
            separator: SEPARATOR,
            gap: 1,
            indent: 0,
            open: true,
            rule: true,
            wrap: false,
            styles: Styles::new(),
        }
    }

    pub fn status(segments: &'a [Segment<'a>]) -> Self {
        Footer {
            segments,
            rule: false,
            ..Footer::new(&[])
        }
    }

    pub fn notice(mut self, notice: Option<Notice<'a>>) -> Self {
        self.notice = notice.filter(|notice| !notice.text.is_empty());
        self
    }

    pub fn confirm(mut self, confirm: Option<ConfirmBar<'a>>) -> Self {
        self.confirm = confirm;
        self
    }

    pub fn input(mut self, input: Option<InputBar<'a>>) -> Self {
        self.input = input;
        self
    }

    pub fn version(mut self, name: &'a str, version: &'a str) -> Self {
        self.name = name;
        self.version = version;
        self
    }

    pub fn separator(mut self, separator: &'a str) -> Self {
        self.separator = separator;
        self
    }

    pub fn gap(mut self, gap: u16) -> Self {
        self.gap = gap;
        self
    }

    pub fn indent(mut self, indent: u16) -> Self {
        self.indent = indent;
        self
    }

    pub fn open(mut self, open: bool) -> Self {
        self.open = open;
        self
    }

    pub fn help(self, help: &Help) -> Self {
        self.open(help.open())
    }

    pub fn rule(mut self, rule: bool) -> Self {
        self.rule = rule;
        self
    }

    pub fn wrap(mut self, wrap: bool) -> Self {
        self.wrap = wrap;
        self
    }

    pub fn styles(mut self, styles: Styles) -> Self {
        self.styles = styles;
        self
    }

    pub fn height(&self, width: u16) -> u16 {
        if let Some(confirm) = self.confirm {
            return confirm.rule(false).height() + u16::from(self.rule);
        }
        if let Some(input) = self.input {
            return input.rule(false).height() + u16::from(self.rule);
        }
        let inner = self.inner(width);
        let lines =
            self.line_count(inner) + u16::from(self.notice.is_some()) + self.pinned_lines(inner);
        if lines == 0 {
            0
        } else {
            lines + u16::from(self.rule)
        }
    }

    pub fn shown(&self, key: &str, width: u16) -> bool {
        let width = self.inner(width);
        if self.confirm.is_some() || self.input.is_some() {
            return false;
        }
        if self.takes_over(width) {
            return self.pinned_lines(width) > 0 && self.pinned().shown(key, width);
        }
        if self.line_count(width) == 0 {
            return false;
        }
        if self.wrapping() {
            return self
                .hints
                .iter()
                .any(|hint| hint.key == key && self.visible(hint));
        }
        let fit = self.fit(width);
        (0..self.count()).any(|index| {
            if fit.kept & (1 << index) == 0 {
                return false;
            }
            let Part::Hints(hints) = self.segment(index).part else {
                return false;
            };
            let kept = self.kept_hints(hints, fit.widths[index]);
            hints
                .iter()
                .take(MOST)
                .enumerate()
                .any(|(at, hint)| kept & (1 << at) != 0 && hint.key == key)
        })
    }

    pub fn cursor(&self, area: Rect) -> Option<Position> {
        if self.confirm.is_some() {
            return None;
        }
        let input = self.input?;
        input.rule(false).cursor(self.body(area))
    }

    fn body(&self, area: Rect) -> Rect {
        let rule = u16::from(self.rule).min(area.height);
        Rect {
            x: area.x.saturating_add(self.indent.min(area.width)),
            y: area.y.saturating_add(rule),
            width: self.inner(area.width),
            height: area.height - rule,
        }
    }

    fn inner(&self, width: u16) -> u16 {
        width.saturating_sub(self.indent)
    }

    fn count(&self) -> usize {
        if self.segments.is_empty() {
            1
        } else {
            self.segments.len().min(MOST)
        }
    }

    fn segment(&self, index: usize) -> Segment<'a> {
        if self.segments.is_empty() {
            Segment::hints(self.hints)
        } else {
            self.segments[index]
        }
    }

    fn wrapping(&self) -> bool {
        self.wrap && self.segments.is_empty()
    }

    fn visible(&self, hint: &Hint) -> bool {
        self.open || hint.pinned
    }

    fn any_visible(&self) -> bool {
        (0..self.count()).any(|index| match self.segment(index).part {
            Part::Text(text, _) => !text.is_empty(),
            Part::Spans(spans) => spans.iter().any(|(text, _)| !text.is_empty()),
            Part::Hints(hints) => hints.iter().any(|hint| self.visible(hint)),
        })
    }

    fn line_count(&self, width: u16) -> u16 {
        if !self.any_visible() {
            return u16::from(self.tag(width) > 0);
        }
        if !self.wrapping() {
            return 1;
        }
        self.flow(width, |_, _, _| {}).0
    }

    fn tag(&self, width: u16) -> u16 {
        if self.version.is_empty() {
            return 0;
        }
        let mut total = text::width(self.version);
        if !self.version.starts_with('v') {
            total = total.saturating_add(1);
        }
        if !self.name.is_empty() {
            total = total
                .saturating_add(text::width(self.name))
                .saturating_add(1);
        }
        if total > width { 0 } else { total }
    }

    fn beside(&self) -> u16 {
        self.gap.max(BESIDE)
    }

    fn natural_line(&self) -> u16 {
        let (total, shown) = (0..self.count())
            .map(|index| self.natural(&self.segment(index)))
            .filter(|width| *width > 0)
            .fold((0u16, 0u16), |(total, shown), width| {
                (total.saturating_add(width), shown + 1)
            });
        total.saturating_add(self.gap.saturating_mul(shown.saturating_sub(1)))
    }

    fn pinned(&self) -> Footer<'a> {
        Footer {
            open: false,
            notice: None,
            rule: false,
            indent: 0,
            ..*self
        }
    }

    fn has_pinned(&self, width: u16) -> bool {
        self.tag(width) > 0
            || (0..self.count()).any(|index| match self.segment(index).part {
                Part::Hints(hints) => hints.iter().any(|hint| hint.pinned),
                Part::Text(..) | Part::Spans(..) => false,
            })
    }

    fn pinned_lines(&self, width: u16) -> u16 {
        if !self.takes_over(width) || !self.has_pinned(width) {
            return 0;
        }
        self.pinned().line_count(width)
    }

    fn takes_over(&self, width: u16) -> bool {
        self.notice
            .is_some_and(|notice| notice.over && text::width(notice.text) > width)
    }

    fn flow(&self, width: u16, mut each: impl FnMut(usize, u16, bool)) -> (u16, Option<u16>) {
        let separator = text::width(self.separator);
        let tag = self.tag(width);
        let reserve = tag.saturating_add(self.beside());
        let last = self.hints.iter().rposition(|hint| self.visible(hint));
        let mut shared = false;
        let mut line = 0;
        let mut used = 0u16;
        for (index, hint) in self.hints.iter().enumerate() {
            if !self.visible(hint) {
                continue;
            }
            let mut wide = hint.width();
            if tag > 0 && Some(index) == last && wide.saturating_add(reserve) <= width {
                wide += reserve;
                shared = true;
            }
            if used > 0 && used.saturating_add(separator).saturating_add(wide) > width {
                line += 1;
                used = 0;
            }
            let first = used == 0;
            if !first {
                used = used.saturating_add(separator);
            }
            used = used.saturating_add(wide);
            each(index, line, first);
        }
        match (last, tag > 0) {
            (None, false) => (0, None),
            (None, true) => (1, Some(0)),
            (Some(_), false) => (line + 1, None),
            (Some(_), true) if shared => (line + 1, Some(line)),
            (Some(_), true) => (line + 2, Some(line + 1)),
        }
    }

    fn hints_width(&self, hints: &[Hint], kept: u64) -> u16 {
        let separator = text::width(self.separator);
        let mut total = 0u16;
        let mut first = true;
        for (index, hint) in hints.iter().take(MOST).enumerate() {
            if kept & (1 << index) == 0 {
                continue;
            }
            if !first {
                total = total.saturating_add(separator);
            }
            first = false;
            total = total.saturating_add(hint.width());
        }
        total
    }

    fn kept_hints(&self, hints: &[Hint], width: u16) -> u64 {
        let mut kept = mask(hints, |hint| self.visible(hint));
        while kept.count_ones() > 1 && self.hints_width(hints, kept) > width {
            let drop = (0..hints.len().min(MOST))
                .filter(|index| kept & (1 << index) != 0)
                .max_by_key(|index| (!hints[*index].pinned, hints[*index].rank, *index));
            match drop {
                Some(index) => kept &= !(1 << index),
                None => break,
            }
        }
        kept
    }

    fn natural(&self, segment: &Segment) -> u16 {
        match segment.part {
            Part::Text(text, _) => text::width(text),
            Part::Spans(spans) => text::spans_width(spans),
            Part::Hints(hints) => self.hints_width(hints, self.kept_hints(hints, u16::MAX)),
        }
    }

    fn least(&self, segment: &Segment) -> u16 {
        if !segment.shrink {
            return self.natural(segment);
        }
        match segment.part {
            Part::Text(..) | Part::Spans(..) => 0,
            Part::Hints(hints) => {
                let pinned = mask(hints, |hint| hint.pinned && self.visible(hint));
                self.hints_width(hints, pinned)
            }
        }
    }

    fn fit(&self, width: u16) -> Fit {
        let count = self.count();
        let mut kept = 0u64;
        for index in 0..count {
            if self.natural(&self.segment(index)) > 0 {
                kept |= 1 << index;
            }
        }
        loop {
            let shown = kept.count_ones() as u16;
            let gaps = self.gap.saturating_mul(shown.saturating_sub(1));
            let fixed = (0..count)
                .filter(|index| kept & (1 << index) != 0)
                .fold(gaps, |total, index| {
                    total.saturating_add(self.least(&self.segment(index)))
                });
            if fixed > width
                && shown > 1
                && let Some(index) = (0..count)
                    .filter(|index| kept & (1 << index) != 0)
                    .max_by_key(|index| (self.segment(*index).rank, *index))
            {
                kept &= !(1 << index);
                continue;
            }
            let mut spare = width.saturating_sub(fixed);
            let mut order = [0usize; MOST];
            let mut len = 0;
            for index in 0..count {
                if kept & (1 << index) != 0 {
                    order[len] = index;
                    len += 1;
                }
            }
            order[..len].sort_unstable_by_key(|index| (self.segment(*index).rank, *index));
            let mut widths = [0u16; MOST];
            for index in order[..len].iter().copied() {
                let segment = self.segment(index);
                let least = self.least(&segment);
                let grow = self.natural(&segment).saturating_sub(least).min(spare);
                spare -= grow;
                widths[index] = least.min(width) + grow;
            }
            let starved = (0..count)
                .find(|index| kept & (1 << index) != 0 && widths[*index] == 0 && shown > 1);
            match starved {
                Some(index) => kept &= !(1 << index),
                None => return Fit { kept, widths },
            }
        }
    }

    fn draw_segment(&self, buf: &mut Buffer, line: Rect, x: u16, segment: &Segment, width: u16) {
        let Some(mut pen) = Pen::new(buf, line, x, line.y) else {
            return;
        };
        match segment.part {
            Part::Text(text, style) => pen.clip(text, style, width),
            Part::Spans(spans) => pen.spans(spans, width),
            Part::Hints(hints) => {
                let kept = self.kept_hints(hints, width);
                let stop = x.saturating_add(width);
                let mut first = true;
                for (index, hint) in hints.iter().take(MOST).enumerate() {
                    if kept & (1 << index) == 0 {
                        continue;
                    }
                    if !first {
                        let room = stop.saturating_sub(pen.x);
                        pen.clip(self.separator, self.styles.muted, room);
                    }
                    first = false;
                    let room = stop.saturating_sub(pen.x);
                    hint.draw(&mut pen, self.styles.lit(), self.styles.muted, room);
                }
            }
        }
    }

    fn draw_side(&self, buf: &mut Buffer, line: Rect, fit: &Fit, right: bool) {
        let count = self.count();
        let side =
            |index: &usize| fit.kept & (1 << index) != 0 && self.segment(*index).right == right;
        let total = (0..count)
            .filter(side)
            .enumerate()
            .fold(0u16, |total, (at, index)| {
                let gap = if at > 0 { self.gap } else { 0 };
                total.saturating_add(gap).saturating_add(fit.widths[index])
            });
        let mut x = if right {
            line.right().saturating_sub(total).max(line.x)
        } else {
            line.x
        };
        for (at, index) in (0..count).filter(side).enumerate() {
            if at > 0 {
                x = x.saturating_add(self.gap);
            }
            self.draw_segment(buf, line, x, &self.segment(index), fit.widths[index]);
            x = x.saturating_add(fit.widths[index]);
        }
    }

    fn draw_tag(&self, buf: &mut Buffer, line: Rect) {
        let tag = self.tag(line.width);
        if tag == 0 {
            return;
        }
        let Some(mut pen) = Pen::new(buf, line, line.right() - tag, line.y) else {
            return;
        };
        let style = self.styles.muted;
        if !self.name.is_empty() {
            pen.put(self.name, style);
            pen.put(" ", style);
        }
        if !self.version.starts_with('v') {
            pen.put("v", style);
        }
        pen.put(self.version, style);
    }

    fn draw_tag_after(&self, buf: &mut Buffer, line: Rect, used: u16) -> Rect {
        let tag = self.tag(line.width);
        let beside = if used == 0 { 0 } else { self.beside() };
        if tag == 0 || used.saturating_add(beside).saturating_add(tag) > line.width {
            return line;
        }
        self.draw_tag(buf, line);
        Rect {
            width: line.width - tag - beside,
            ..line
        }
    }

    fn draw_tag_at(&self, buf: &mut Buffer, body: Rect, end: Option<Position>) {
        if let Some(end) = end {
            let line = Rect::new(body.x, end.y, body.width, 1);
            self.draw_tag_after(buf, line, end.x.saturating_sub(body.x));
        }
    }

    fn draw_line(&self, buf: &mut Buffer, line: Rect) {
        let line = self.draw_tag_after(buf, line, self.natural_line());
        let fit = self.fit(line.width);
        self.draw_side(buf, line, &fit, false);
        self.draw_side(buf, line, &fit, true);
    }

    fn draw_wrapped(&self, buf: &mut Buffer, area: Rect) -> u16 {
        let mut x = area.x;
        let mut last = 0;
        let (_, tag) = self.flow(area.width, |index, line, first| {
            if line >= area.height {
                return;
            }
            last = line + 1;
            let y = area.y.saturating_add(line);
            let start = if first { area.x } else { x };
            let Some(mut pen) = Pen::new(buf, area, start, y) else {
                return;
            };
            if !first {
                pen.put(self.separator, self.styles.muted);
            }
            let room = pen.room();
            self.hints[index].draw(&mut pen, self.styles.lit(), self.styles.muted, room);
            x = pen.x;
        });
        if let Some(row) = tag
            && row < area.height
        {
            self.draw_tag(
                buf,
                Rect::new(area.x, area.y.saturating_add(row), area.width, 1),
            );
            last = last.max(row + 1);
        }
        last
    }
}

impl Widget for Footer<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        (&self).render(area, buf);
    }
}

impl Widget for &Footer<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        if area.is_empty() || self.height(area.width) == 0 {
            return;
        }
        if self.rule {
            text::rule(buf, area, self.styles.rule);
        }
        let body = self.body(area);
        let mut y = body.y;
        let area = Rect {
            y: area.y,
            height: area.height,
            ..body
        };
        let rest = |y: u16| Rect::new(area.x, y, area.width, area.bottom().saturating_sub(y));
        if let Some(confirm) = self.confirm {
            let end = confirm.rule(false).styled(self.styles).paint(body, buf);
            self.draw_tag_at(buf, body, end);
            return;
        }
        if let Some(input) = self.input {
            let end = input.rule(false).styled(self.styles).paint(body, buf);
            self.draw_tag_at(buf, body, end);
            return;
        }
        if self.takes_over(area.width)
            && let Some(notice) = self.notice
        {
            let all = rest(y);
            let pinned = self.pinned_lines(area.width);
            let kept = if pinned > 0 && all.height > pinned {
                pinned
            } else {
                0
            };
            let flowing = Rect {
                height: all.height - kept,
                ..all
            };
            text::flow(buf, flowing, notice.text, notice.tone.style(&self.styles));
            if kept > 0 {
                let low = Rect {
                    y: all.bottom() - kept,
                    height: kept,
                    ..all
                };
                self.pinned().styles(self.styles).render(low, buf);
            }
            return;
        }
        if self.line_count(area.width) > 0 && y < area.bottom() {
            if self.wrapping() {
                let room = rest(y);
                let notice = u16::from(self.notice.is_some());
                let room = Rect {
                    height: room.height.saturating_sub(notice).max(1),
                    ..room
                };
                y += self.draw_wrapped(buf, room);
            } else {
                self.draw_line(buf, Rect::new(area.x, y, area.width, 1));
                y += 1;
            }
        }
        if let Some(notice) = self.notice
            && y < area.bottom()
            && let Some(mut pen) = Pen::new(buf, area, area.x, y)
        {
            pen.rest(notice.text, notice.tone.style(&self.styles));
        }
    }
}
