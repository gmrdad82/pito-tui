use ratatui::{buffer::Buffer, layout::Rect, style::Style, widgets::Widget};

use crate::header::nav::{Breadcrumb, Nav, NavBar, Place, RULE};
use crate::header::styles::Styles;
use crate::header::text::{self, ELLIPSIS, Pen};

const SEPARATOR: &str = " · ";
const SIDE_MIN: u16 = 6;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Fact<'a> {
    pub text: &'a str,
    pub style: Style,
}

impl<'a> Fact<'a> {
    pub const fn new(text: &'a str, style: Style) -> Self {
        Fact { text, style }
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
enum Slot<'a> {
    #[default]
    Empty,
    One(Fact<'a>),
    Parts(&'a [Fact<'a>]),
    Pairs(&'a [(String, Style)]),
}

impl<'a> Slot<'a> {
    fn parts(&self) -> Parts<'a> {
        match *self {
            Slot::Empty => Parts::Slice(&[]),
            Slot::One(fact) => Parts::One(fact),
            Slot::Parts(parts) => Parts::Slice(parts),
            Slot::Pairs(pairs) => Parts::Pairs(pairs),
        }
    }

    fn width(&self) -> u16 {
        self.parts().width()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Parts<'a> {
    One(Fact<'a>),
    Slice(&'a [Fact<'a>]),
    Pairs(&'a [(String, Style)]),
}

impl<'a> Parts<'a> {
    fn is_empty(&self) -> bool {
        self.len() == 0
    }

    fn len(&self) -> usize {
        match self {
            Parts::One(_) => 1,
            Parts::Slice(parts) => parts.len(),
            Parts::Pairs(pairs) => pairs.len(),
        }
    }

    fn get(&self, index: usize) -> Fact<'a> {
        match *self {
            Parts::One(fact) => fact,
            Parts::Slice(parts) => parts[index],
            Parts::Pairs(pairs) => Fact::new(&pairs[index].0, pairs[index].1),
        }
    }

    fn iter(&self) -> impl Iterator<Item = Fact<'a>> + Clone {
        let parts = *self;
        (0..parts.len()).map(move |index| parts.get(index))
    }

    fn width(&self) -> u16 {
        self.iter().fold(0u16, |total, part| {
            total.saturating_add(text::width(part.text))
        })
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Drill {
    #[default]
    Replace,
    Rows,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Header<'a> {
    nav: &'a Nav,
    styles: Styles,
    title: Option<&'a str>,
    left: Slot<'a>,
    right: Slot<'a>,
    tabs: bool,
    lit: bool,
    underline: bool,
    facts: Slot<'a>,
    facts_rule: bool,
    separator: &'a str,
    crumbs: bool,
    closing: bool,
    notice: Option<Fact<'a>>,
    drill: Drill,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct Rows {
    title: Option<Rect>,
    tabs: Option<Rect>,
    facts: Option<Rect>,
    crumbs: Option<Rect>,
    closing: Option<Rect>,
    notice: Option<Rect>,
}

impl<'a> Header<'a> {
    pub fn new(nav: &'a Nav) -> Self {
        Header {
            nav,
            styles: Styles::new(),
            title: None,
            left: Slot::Empty,
            right: Slot::Empty,
            tabs: true,
            lit: true,
            underline: false,
            facts: Slot::Empty,
            facts_rule: false,
            separator: SEPARATOR,
            crumbs: false,
            closing: false,
            notice: None,
            drill: Drill::Replace,
        }
    }

    pub fn styles(mut self, styles: Styles) -> Self {
        self.styles = styles;
        self
    }

    pub fn title(mut self, title: Option<&'a str>) -> Self {
        self.title = title.filter(|title| !title.is_empty());
        self
    }

    pub fn left(mut self, left: Option<Fact<'a>>) -> Self {
        self.left = slot(left.map_or(Slot::Empty, Slot::One));
        self
    }

    pub fn right(mut self, right: Option<Fact<'a>>) -> Self {
        self.right = slot(right.map_or(Slot::Empty, Slot::One));
        self
    }

    pub fn left_parts(mut self, parts: &'a [Fact<'a>]) -> Self {
        self.left = slot(Slot::Parts(parts));
        self
    }

    pub fn right_parts(mut self, parts: &'a [Fact<'a>]) -> Self {
        self.right = slot(Slot::Parts(parts));
        self
    }

    pub fn left_pairs(mut self, parts: &'a [(String, Style)]) -> Self {
        self.left = slot(Slot::Pairs(parts));
        self
    }

    pub fn right_pairs(mut self, parts: &'a [(String, Style)]) -> Self {
        self.right = slot(Slot::Pairs(parts));
        self
    }

    pub fn tabs(mut self, tabs: bool) -> Self {
        self.tabs = tabs;
        self
    }

    pub fn lit(mut self, lit: bool) -> Self {
        self.lit = lit;
        self
    }

    pub fn underline(mut self, underline: bool) -> Self {
        self.underline = underline;
        self
    }

    pub fn facts(mut self, facts: &'a [Fact<'a>]) -> Self {
        self.facts = Slot::Parts(facts);
        self
    }

    pub fn facts_pairs(mut self, facts: &'a [(String, Style)]) -> Self {
        self.facts = Slot::Pairs(facts);
        self
    }

    pub fn facts_rule(mut self, facts_rule: bool) -> Self {
        self.facts_rule = facts_rule;
        self
    }

    pub fn separator(mut self, separator: &'a str) -> Self {
        self.separator = separator;
        self
    }

    pub fn crumbs(mut self, crumbs: bool) -> Self {
        self.crumbs = crumbs;
        self
    }

    pub fn closing(mut self, closing: bool) -> Self {
        self.closing = closing;
        self
    }

    pub fn notice(mut self, notice: Option<Fact<'a>>) -> Self {
        self.notice = notice.filter(|fact| !fact.text.is_empty());
        self
    }

    pub fn drill(mut self, drill: Drill) -> Self {
        self.drill = drill;
        self
    }

    pub fn height(&self) -> u16 {
        let replaced = self.replaced();
        u16::from(self.title.is_some())
            + if self.tabs { self.bar().height() } else { 0 }
            + u16::from(self.has_facts() && !replaced)
            + u16::from(self.crumbs && !replaced)
            + u16::from(self.closing)
            + u16::from(self.notice.is_some())
    }

    pub fn hit(&self, area: Rect, column: u16, row: u16) -> Option<Place> {
        let tabs = self.rows(area).tabs?;
        let bar = self.bar();
        if self.replaced() && bar.rows(tabs).1.is_some_and(|line| line.y == row) {
            return None;
        }
        bar.hit(tabs, column, row)
    }

    fn has_facts(&self) -> bool {
        self.facts.parts().iter().any(|fact| !fact.text.is_empty())
    }

    fn replaced(&self) -> bool {
        self.drill == Drill::Replace && self.tabs && self.nav.depth() > 0
    }

    fn bar(&self) -> NavBar<'a> {
        NavBar::new(self.nav)
            .styles(self.styles)
            .lit(self.lit)
            .underline(self.underline)
    }

    fn rows(&self, area: Rect) -> Rows {
        let mut y = area.y;
        let mut take = |on: bool, height: u16| {
            if !on || height == 0 || y >= area.bottom() {
                return None;
            }
            let height = height.min(area.bottom() - y);
            let row = Rect::new(area.x, y, area.width, height);
            y += height;
            Some(row)
        };
        Rows {
            title: take(self.title.is_some(), 1),
            tabs: take(self.tabs, self.bar().height()),
            facts: take(self.has_facts() && !self.replaced(), 1),
            crumbs: take(self.crumbs && !self.replaced(), 1),
            closing: take(self.closing, 1),
            notice: take(self.notice.is_some(), 1),
        }
    }

    fn draw_title(&self, buf: &mut Buffer, line: Rect, title: &str) {
        let Some(mut pen) = Pen::new(buf, line, line.x, line.y) else {
            return;
        };
        let rule = self.styles.rule;
        let lit = self.styles.lit();
        let wide = text::width(title).saturating_add(2).min(line.width);
        let start = line.x.saturating_add((line.width - wide) / 2);
        let side = start.saturating_sub(line.x).saturating_sub(2);
        let sides = side >= SIDE_MIN;
        if sides && self.left != Slot::Empty {
            pen.fill_to(line.x.saturating_add(1), RULE, rule);
            label(&mut pen, self.left.parts(), side - 2);
        }
        pen.fill_to(start, RULE, rule);
        label(
            &mut pen,
            Parts::One(Fact::new(title, lit)),
            wide.saturating_sub(2),
        );
        if sides && self.right != Slot::Empty {
            let wide = self.right.width().min(side - 2).saturating_add(2);
            pen.fill_to(line.right().saturating_sub(1 + wide), RULE, rule);
            label(&mut pen, self.right.parts(), wide - 2);
        }
        pen.fill(RULE, rule);
    }

    fn draw_tabs(&self, buf: &mut Buffer, tabs: Rect) {
        let bar = self.bar();
        if !self.replaced() {
            return bar.render(tabs, buf);
        }
        let (groups, sections) = bar.rows(tabs);
        if let Some(line) = groups {
            bar.draw_groups(buf, line);
        }
        if let Some(line) = sections {
            Breadcrumb::new(self.nav)
                .styles(self.styles)
                .centred(true)
                .render(line, buf);
        }
    }

    fn draw_facts(&self, buf: &mut Buffer, line: Rect) {
        let pad = if self.facts_rule { 1 } else { 0 };
        let separator = text::width(self.separator);
        let facts = self.facts.parts();
        let present = || facts.iter().filter(|fact| !fact.text.is_empty());
        let inner = present().enumerate().fold(0u16, |total, (index, fact)| {
            let gap = if index > 0 { separator } else { 0 };
            total
                .saturating_add(gap)
                .saturating_add(text::width(fact.text))
        });
        let shown = inner.min(line.width.saturating_sub(4 * pad));
        let block = shown.saturating_add(2 * pad);
        let start = line.x.saturating_add(line.width.saturating_sub(block) / 2);
        let rule = self.styles.rule;
        let Some(mut pen) = Pen::new(buf, line, line.x, line.y) else {
            return;
        };
        if self.facts_rule {
            pen.fill_to(start, RULE, rule);
        } else {
            pen.skip(start.saturating_sub(line.x));
        }
        let muted = self.styles.muted;
        if pad > 0 {
            pen.put(" ", muted);
        }
        let end = pen.x.saturating_add(shown);
        for (index, fact) in present().enumerate() {
            let wide = text::width(fact.text);
            if index > 0 {
                let room = end.saturating_sub(pen.x);
                if room < separator.saturating_add(wide.min(2)) {
                    pen.clip(ELLIPSIS, muted, room);
                    break;
                }
                pen.put(self.separator, muted);
            }
            let room = end.saturating_sub(pen.x);
            pen.clip(fact.text, fact.style, room);
            if wide > room {
                break;
            }
        }
        if pad > 0 {
            pen.put(" ", muted);
            pen.fill(RULE, rule);
        }
    }

    fn draw_notice(&self, buf: &mut Buffer, line: Rect, notice: Fact) {
        if let Some(mut pen) = Pen::new(buf, line, line.x, line.y) {
            let room = pen.room();
            pen.clip(notice.text, notice.style, room);
        }
    }
}

fn slot(slot: Slot) -> Slot {
    if slot.width() == 0 { Slot::Empty } else { slot }
}

fn label(pen: &mut Pen, parts: Parts, room: u16) {
    if parts.is_empty() {
        return;
    }
    let first = parts.get(0).style;
    let last = parts.get(parts.len() - 1).style;
    pen.put(" ", first);
    if parts.width() <= room {
        for part in parts.iter() {
            pen.put(part.text, part.style);
        }
    } else if room > 0 {
        let stop = pen.x.saturating_add(room - 1);
        let mut style = first;
        for part in parts.iter().filter(|part| !part.text.is_empty()) {
            style = part.style;
            if pen.x >= stop {
                break;
            }
            let start = pen.x;
            pen.putn(part.text, part.style, stop - pen.x);
            if pen.x - start < text::width(part.text) {
                break;
            }
        }
        pen.x = pen.x.min(stop);
        pen.put(ELLIPSIS, style);
    }
    pen.put(" ", last);
}

impl Widget for Header<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        (&self).render(area, buf);
    }
}

impl Widget for &Header<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        let rows = self.rows(area);
        if let (Some(line), Some(title)) = (rows.title, self.title) {
            self.draw_title(buf, line, title);
        }
        if let Some(tabs) = rows.tabs {
            self.draw_tabs(buf, tabs);
        }
        if let Some(line) = rows.facts {
            self.draw_facts(buf, line);
        }
        if let Some(line) = rows.crumbs {
            Breadcrumb::new(self.nav)
                .styles(self.styles)
                .render(line, buf);
        }
        if let Some(line) = rows.closing
            && let Some(mut pen) = Pen::new(buf, line, line.x, line.y)
        {
            pen.fill(RULE, self.styles.rule);
        }
        if let (Some(line), Some(notice)) = (rows.notice, self.notice) {
            self.draw_notice(buf, line, notice);
        }
    }
}
