use std::borrow::Cow;

use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Modifier, Style},
    widgets::Widget,
};

use crate::header::key::Key;
use crate::header::styles::Styles;
use crate::header::text::{self, ELLIPSIS, Pen};

const GROUP_GAP: u16 = 2;
const SECTION_GAP: u16 = 0;
const JOIN: &str = " / ";
pub(crate) const RULE: &str = "─";

#[derive(Debug, Clone, PartialEq, Eq)]
struct Names {
    name: Cow<'static, str>,
    short: Option<Cow<'static, str>>,
}

impl Names {
    fn new(name: impl Into<Cow<'static, str>>) -> Self {
        Names {
            name: name.into(),
            short: None,
        }
    }

    fn short_name(&self) -> &str {
        self.short.as_deref().unwrap_or(&self.name)
    }
}

type Span = (Cow<'static, str>, Style);

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Section {
    names: Names,
    spans: Vec<Span>,
    short_spans: Vec<Span>,
}

fn owned(spans: &[(&str, Style)]) -> Vec<Span> {
    spans
        .iter()
        .map(|(text, style)| (Cow::Owned(text.to_string()), *style))
        .collect()
}

fn joined(spans: &[(&str, Style)]) -> String {
    spans.iter().map(|(text, _)| *text).collect()
}

impl Section {
    pub fn new(name: impl Into<Cow<'static, str>>) -> Self {
        Section {
            names: Names::new(name),
            spans: Vec::new(),
            short_spans: Vec::new(),
        }
    }

    pub fn spans(spans: &[(&str, Style)]) -> Self {
        Section {
            names: Names::new(joined(spans)),
            spans: owned(spans),
            short_spans: Vec::new(),
        }
    }

    pub fn short(mut self, short: impl Into<Cow<'static, str>>) -> Self {
        self.names.short = Some(short.into());
        self.short_spans.clear();
        self
    }

    pub fn short_spans(mut self, spans: &[(&str, Style)]) -> Self {
        self.names.short = Some(Cow::Owned(joined(spans)));
        self.short_spans = owned(spans);
        self
    }

    pub fn set_name(&mut self, name: impl Into<Cow<'static, str>>) {
        self.names.name = name.into();
        self.spans.clear();
    }

    pub fn set_spans(&mut self, spans: &[(&str, Style)]) {
        self.names.name = Cow::Owned(joined(spans));
        self.spans = owned(spans);
    }

    pub fn set_short(&mut self, short: impl Into<Cow<'static, str>>) {
        self.names.short = Some(short.into());
        self.short_spans.clear();
    }

    pub fn set_short_spans(&mut self, spans: &[(&str, Style)]) {
        self.names.short = Some(Cow::Owned(joined(spans)));
        self.short_spans = owned(spans);
    }

    pub fn name(&self) -> &str {
        &self.names.name
    }

    pub fn short_name(&self) -> &str {
        self.names.short_name()
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Group {
    names: Names,
    sections: Vec<Section>,
}

impl Group {
    pub fn new(name: impl Into<Cow<'static, str>>) -> Self {
        Group {
            names: Names::new(name),
            sections: Vec::new(),
        }
    }

    pub fn short(mut self, short: impl Into<Cow<'static, str>>) -> Self {
        self.names.short = Some(short.into());
        self
    }

    pub fn section(mut self, section: Section) -> Self {
        self.sections.push(section);
        self
    }

    pub fn name(&self) -> &str {
        &self.names.name
    }

    pub fn short_name(&self) -> &str {
        self.names.short_name()
    }

    pub fn sections(&self) -> &[Section] {
        &self.sections
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Place {
    pub group: usize,
    pub section: usize,
    pub number: usize,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Action {
    NextGroup,
    PrevGroup,
    NextSection,
    PrevSection,
    Go(usize),
    Back,
    Swallow,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct NavKeys {
    pub next_group: &'static [Key],
    pub prev_group: &'static [Key],
    pub next_section: &'static [Key],
    pub prev_section: &'static [Key],
    pub back: &'static [Key],
    pub digits: bool,
    pub swallow_digits: bool,
    pub zero_first: bool,
}

impl NavKeys {
    pub const HEY: NavKeys = NavKeys {
        next_group: &[Key::Tab],
        prev_group: &[Key::BackTab],
        next_section: &[Key::Char(']')],
        prev_section: &[Key::Char('[')],
        back: &[Key::Esc, Key::Char('q')],
        digits: true,
        swallow_digits: false,
        zero_first: false,
    };

    pub const NONE: NavKeys = NavKeys {
        next_group: &[],
        prev_group: &[],
        next_section: &[],
        prev_section: &[],
        back: &[],
        digits: false,
        swallow_digits: false,
        zero_first: false,
    };

    pub const fn next_group(mut self, keys: &'static [Key]) -> Self {
        self.next_group = keys;
        self
    }

    pub const fn prev_group(mut self, keys: &'static [Key]) -> Self {
        self.prev_group = keys;
        self
    }

    pub const fn next_section(mut self, keys: &'static [Key]) -> Self {
        self.next_section = keys;
        self
    }

    pub const fn prev_section(mut self, keys: &'static [Key]) -> Self {
        self.prev_section = keys;
        self
    }

    pub const fn back(mut self, keys: &'static [Key]) -> Self {
        self.back = keys;
        self
    }

    pub const fn digits(mut self, on: bool) -> Self {
        self.digits = on;
        self
    }

    pub const fn swallow_digits(mut self, on: bool) -> Self {
        self.swallow_digits = on;
        self
    }

    pub const fn zero_first(mut self, on: bool) -> Self {
        self.zero_first = on;
        self
    }

    const fn shown(&self, number: usize) -> Option<usize> {
        match number {
            1..=10 if self.zero_first => Some(number - 1),
            _ if self.zero_first => None,
            10 => Some(0),
            _ => Some(number),
        }
    }
}

impl Default for NavKeys {
    fn default() -> Self {
        NavKeys::HEY
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Step {
    Moved(Place),
    Back { selected: usize },
    Swallowed,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct Level {
    title: Cow<'static, str>,
    selected: usize,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Nav {
    groups: Vec<Group>,
    group: usize,
    last: Vec<usize>,
    stacks: Vec<Vec<Level>>,
    keys: NavKeys,
}

impl Nav {
    pub fn new(groups: Vec<Group>) -> Self {
        let total = groups.iter().map(|group| group.sections.len()).sum();
        let group = groups
            .iter()
            .position(|group| !group.sections.is_empty())
            .unwrap_or(0);
        Nav {
            last: vec![0; groups.len()],
            stacks: vec![Vec::new(); total],
            groups,
            group,
            keys: NavKeys::HEY,
        }
    }

    pub fn keys(mut self, keys: NavKeys) -> Self {
        self.keys = keys;
        self
    }

    pub fn groups(&self) -> &[Group] {
        &self.groups
    }

    pub fn place(&self) -> Place {
        let section = self.last.get(self.group).copied().unwrap_or(0);
        let filled = self
            .groups
            .get(self.group)
            .is_some_and(|group| !group.sections.is_empty());
        Place {
            group: self.group,
            section,
            number: if filled {
                self.offset(self.group) + section + 1
            } else {
                0
            },
        }
    }

    pub fn selected(&self, group: usize) -> Option<usize> {
        let section = *self.last.get(group)?;
        (section < self.groups[group].sections.len()).then_some(section)
    }

    pub fn section(&self) -> Option<&Section> {
        let place = self.place();
        self.groups.get(place.group)?.sections.get(place.section)
    }

    pub fn section_mut(&mut self, group: usize, section: usize) -> Option<&mut Section> {
        self.groups.get_mut(group)?.sections.get_mut(section)
    }

    pub fn count(&self) -> usize {
        self.stacks.len()
    }

    pub fn go(&mut self, number: usize) -> Option<Place> {
        if number == 0 {
            return None;
        }
        let mut offset = 0;
        for (index, group) in self.groups.iter().enumerate() {
            let len = group.sections.len();
            if number <= offset + len {
                self.group = index;
                self.last[index] = number - offset - 1;
                return Some(self.place());
            }
            offset += len;
        }
        None
    }

    pub fn go_to(&mut self, group: usize, section: usize) -> Option<Place> {
        let len = self.groups.get(group)?.sections.len();
        if section >= len {
            return None;
        }
        self.group = group;
        self.last[group] = section;
        Some(self.place())
    }

    pub fn cycle(&mut self, by: isize) -> Option<Place> {
        if by == 0 {
            return Some(self.place());
        }
        let filled = self
            .groups
            .iter()
            .filter(|group| !group.sections.is_empty())
            .count();
        if filled < 2 {
            return None;
        }
        let step = if by < 0 { -1 } else { 1 };
        let count = self.groups.len() as isize;
        let mut index = self.group as isize;
        for _ in 0..by.unsigned_abs() % filled {
            loop {
                index = (index + step).rem_euclid(count);
                if !self.groups[index as usize].sections.is_empty() {
                    break;
                }
            }
        }
        self.group = index as usize;
        Some(self.place())
    }

    pub fn step(&mut self, by: isize) -> Option<Place> {
        let len = self.groups.get(self.group)?.sections.len() as isize;
        if len == 0 {
            return None;
        }
        let at = self.last[self.group] as isize;
        self.last[self.group] = (at + by.rem_euclid(len)).rem_euclid(len) as usize;
        Some(self.place())
    }

    pub fn open(&mut self, title: impl Into<Cow<'static, str>>, selected: usize) {
        if let Some(stack) = self.stack_mut() {
            stack.push(Level {
                title: title.into(),
                selected,
            });
        }
    }

    pub fn back(&mut self) -> Option<usize> {
        self.stack_mut()?.pop().map(|level| level.selected)
    }

    pub fn close(&mut self) -> Option<usize> {
        let stack = self.stack_mut()?;
        let selected = stack.first().map(|level| level.selected);
        stack.clear();
        selected
    }

    pub fn depth(&self) -> usize {
        self.stack().map_or(0, Vec::len)
    }

    pub fn title(&self) -> Option<&str> {
        self.stack()?.last().map(|level| level.title.as_ref())
    }

    pub fn crumbs(&self) -> impl Iterator<Item = &str> {
        self.section().map(Section::name).into_iter().chain(
            self.stack()
                .into_iter()
                .flatten()
                .map(|level| level.title.as_ref()),
        )
    }

    pub fn breadcrumb(&self) -> String {
        let mut out = String::new();
        for (index, crumb) in self.crumbs().enumerate() {
            if index > 0 {
                out.push_str(JOIN);
            }
            out.push_str(crumb);
        }
        out
    }

    pub fn action(&self, key: Key) -> Option<Action> {
        let keys = &self.keys;
        let filled = self
            .groups
            .iter()
            .filter(|group| !group.sections.is_empty())
            .count();
        let action = if keys.next_group.contains(&key) {
            if filled > 1 {
                Action::NextGroup
            } else {
                Action::NextSection
            }
        } else if keys.prev_group.contains(&key) {
            if filled > 1 {
                Action::PrevGroup
            } else {
                Action::PrevSection
            }
        } else if keys.next_section.contains(&key) {
            Action::NextSection
        } else if keys.prev_section.contains(&key) {
            Action::PrevSection
        } else if keys.back.contains(&key) {
            if self.depth() == 0 {
                return None;
            }
            Action::Back
        } else {
            match key {
                Key::Char(digit @ '0'..='9') if keys.digits => {
                    let digit = digit as usize - '0' as usize;
                    Action::Go(match digit {
                        _ if keys.zero_first => digit + 1,
                        0 => 10,
                        _ => digit,
                    })
                }
                _ => return None,
            }
        };
        match action {
            Action::Go(number) if number > self.count() && keys.swallow_digits => {
                Some(Action::Swallow)
            }
            Action::Go(number) if number > self.count() => None,
            Action::NextSection | Action::PrevSection if self.count() == 0 => None,
            action => Some(action),
        }
    }

    pub fn apply(&mut self, action: Action) -> Option<Step> {
        let moved = match action {
            Action::NextGroup => self.cycle(1),
            Action::PrevGroup => self.cycle(-1),
            Action::NextSection => self.step(1),
            Action::PrevSection => self.step(-1),
            Action::Go(number) => self.go(number),
            Action::Back => return self.back().map(|selected| Step::Back { selected }),
            Action::Swallow => return Some(Step::Swallowed),
        };
        moved.map(Step::Moved)
    }

    pub fn key(&mut self, key: Key) -> Option<Step> {
        let action = self.action(key)?;
        self.apply(action)
    }

    pub fn bar(&self) -> NavBar<'_> {
        NavBar::new(self)
    }

    pub fn breadcrumb_bar(&self) -> Breadcrumb<'_> {
        Breadcrumb::new(self)
    }

    fn offset(&self, group: usize) -> usize {
        self.groups[..group]
            .iter()
            .map(|group| group.sections.len())
            .sum()
    }

    fn flat(&self) -> Option<usize> {
        let place = self.place();
        (place.number > 0).then(|| place.number - 1)
    }

    fn stack(&self) -> Option<&Vec<Level>> {
        self.stacks.get(self.flat()?)
    }

    fn stack_mut(&mut self) -> Option<&mut Vec<Level>> {
        let flat = self.flat()?;
        self.stacks.get_mut(flat)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Form {
    Full,
    Short,
    Lone,
    Bare,
}

const FORMS: [Form; 4] = [Form::Full, Form::Short, Form::Lone, Form::Bare];

#[derive(Debug, Clone, Copy)]
struct Label<'a> {
    number: Option<usize>,
    text: &'a str,
    spans: &'a [Span],
    on: bool,
}

impl Label<'_> {
    fn empty(&self) -> bool {
        self.spans.is_empty() && self.text.is_empty()
    }

    fn body_width(&self) -> u16 {
        if self.spans.is_empty() {
            return text::width(self.text);
        }
        self.spans.iter().fold(0u16, |total, (text, _)| {
            total.saturating_add(text::width(text))
        })
    }

    fn width(&self) -> u16 {
        let number = self
            .number
            .map_or(0, |n| text::number_width(n) + u16::from(!self.empty()));
        number.saturating_add(2).saturating_add(self.body_width())
    }

    fn draw(&self, pen: &mut Pen, style: Style, underline: bool) {
        let mut digits = [0; 20];
        pen.put(" ", style);
        if let Some(number) = self.number {
            let number_style = if underline {
                style.add_modifier(Modifier::UNDERLINED)
            } else {
                style
            };
            pen.put(text::digits(number, &mut digits), number_style);
            if !self.empty() {
                pen.put(" ", style);
            }
        }
        if self.spans.is_empty() {
            pen.put(self.text, style);
        }
        for (text, span) in self.spans {
            pen.put(text, style.patch(*span));
        }
        pen.put(" ", style);
    }
}

fn group_text(group: &Group, on: bool, form: Form) -> &str {
    match form {
        Form::Full => group.name(),
        Form::Short => group.short_name(),
        Form::Lone if on => group.short_name(),
        Form::Lone | Form::Bare => text::first(group.short_name()),
    }
}

fn section_body(section: &Section, on: bool, form: Form) -> (&str, &[Span]) {
    let spans = section.spans.as_slice();
    let short = if !section.short_spans.is_empty() {
        ("", section.short_spans.as_slice())
    } else if section.names.short.is_none() && !spans.is_empty() {
        ("", spans)
    } else {
        (section.short_name(), &[][..])
    };
    match form {
        Form::Full if !spans.is_empty() => ("", spans),
        Form::Full => (section.name(), &[]),
        Form::Short => short,
        Form::Lone if on => short,
        Form::Lone | Form::Bare => ("", &[]),
    }
}

fn row_width<'a>(labels: impl Iterator<Item = Label<'a>>, gap: u16) -> u16 {
    labels.enumerate().fold(0u16, |total, (index, label)| {
        let gap = if index > 0 { gap } else { 0 };
        total.saturating_add(gap).saturating_add(label.width())
    })
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct NavBar<'a> {
    nav: &'a Nav,
    styles: Styles,
    lit: bool,
    groups: bool,
    underline: bool,
}

impl<'a> NavBar<'a> {
    pub fn new(nav: &'a Nav) -> Self {
        NavBar {
            nav,
            styles: Styles::new(),
            lit: true,
            groups: nav.groups.len() > 1,
            underline: false,
        }
    }

    pub fn styles(mut self, styles: Styles) -> Self {
        self.styles = styles;
        self
    }

    pub fn lit(mut self, lit: bool) -> Self {
        self.lit = lit;
        self
    }

    pub fn groups(mut self, groups: bool) -> Self {
        self.groups = groups;
        self
    }

    pub fn underline(mut self, underline: bool) -> Self {
        self.underline = underline;
        self
    }

    pub fn height(&self) -> u16 {
        u16::from(self.groups) + u16::from(self.nav.count() > 0)
    }

    pub fn hit(&self, area: Rect, column: u16, row: u16) -> Option<Place> {
        if column < area.x || column >= area.right() {
            return None;
        }
        let (groups, sections) = self.rows(area);
        if let Some(line) = groups
            && row == line.y
        {
            let (form, start) = self.fit_groups(line);
            let index = hit_row(self.group_labels(form), GROUP_GAP, start, column)?;
            let section = self.nav.last[index];
            return (!self.nav.groups[index].sections.is_empty()).then(|| Place {
                group: index,
                section,
                number: self.nav.offset(index) + section + 1,
            });
        }
        let line = sections?;
        if row != line.y {
            return None;
        }
        let (form, start) = self.fit_sections(line);
        let index = hit_row(self.section_labels(form), SECTION_GAP, start, column)?;
        let group = self.nav.group;
        Some(Place {
            group,
            section: index,
            number: self.nav.offset(group) + index + 1,
        })
    }

    pub(crate) fn rows(&self, area: Rect) -> (Option<Rect>, Option<Rect>) {
        let line = |y: u16| Rect::new(area.x, y, area.width, 1);
        let sections = self.nav.count() > 0;
        match area.height {
            0 => (None, None),
            1 if sections => (None, Some(line(area.y))),
            1 if self.groups => (Some(line(area.y)), None),
            _ if self.groups && sections => (Some(line(area.y)), Some(line(area.y + 1))),
            _ if self.groups => (Some(line(area.y)), None),
            _ if sections => (None, Some(line(area.y))),
            _ => (None, None),
        }
    }

    fn group_labels(&self, form: Form) -> impl Iterator<Item = Label<'a>> + Clone {
        let current = self.nav.group;
        self.nav
            .groups
            .iter()
            .enumerate()
            .map(move |(index, group)| {
                let on = index == current;
                Label {
                    number: None,
                    text: group_text(group, on, form),
                    spans: &[],
                    on,
                }
            })
    }

    fn section_labels(&self, form: Form) -> impl Iterator<Item = Label<'a>> + Clone {
        let place = self.nav.place();
        let offset = self.nav.offset(place.group);
        let keys = self.nav.keys;
        self.nav
            .groups
            .get(place.group)
            .map(|group| group.sections.as_slice())
            .unwrap_or(&[])
            .iter()
            .enumerate()
            .map(move |(index, section)| {
                let on = index == place.section;
                let (text, spans) = section_body(section, on, form);
                Label {
                    number: keys.shown(offset + index + 1),
                    text,
                    spans,
                    on,
                }
            })
    }

    fn fit_groups(&self, line: Rect) -> (Form, u16) {
        fit(|form| self.group_labels(form), GROUP_GAP, line)
    }

    fn fit_sections(&self, line: Rect) -> (Form, u16) {
        fit(|form| self.section_labels(form), SECTION_GAP, line)
    }
}

fn fit<'a, I>(labels: impl Fn(Form) -> I, gap: u16, line: Rect) -> (Form, u16)
where
    I: Iterator<Item = Label<'a>>,
{
    for form in FORMS {
        let total = row_width(labels(form), gap);
        if total <= line.width {
            return (form, line.x.saturating_add((line.width - total) / 2));
        }
    }
    (Form::Bare, line.x)
}

fn hit_row<'a>(
    labels: impl Iterator<Item = Label<'a>>,
    gap: u16,
    start: u16,
    column: u16,
) -> Option<usize> {
    let mut x = start;
    for (index, label) in labels.enumerate() {
        if index > 0 {
            x = x.saturating_add(gap);
        }
        let end = x.saturating_add(label.width());
        if (x..end).contains(&column) {
            return Some(index);
        }
        x = end;
    }
    None
}

struct Row {
    gap: u16,
    start: u16,
    on: Style,
    off: Style,
    underline: bool,
}

fn draw_row<'a>(buf: &mut Buffer, line: Rect, labels: impl Iterator<Item = Label<'a>>, row: Row) {
    let Row {
        gap,
        start,
        on,
        off,
        underline,
    } = row;
    let Some(mut pen) = Pen::new(buf, line, start, line.y) else {
        return;
    };
    for (index, label) in labels.enumerate() {
        if index > 0 {
            pen.skip(gap);
        }
        label.draw(&mut pen, if label.on { on } else { off }, underline);
    }
}

impl Widget for NavBar<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        (&self).render(area, buf);
    }
}

impl Widget for &NavBar<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        let (groups, sections) = self.rows(area);
        if let Some(line) = groups {
            self.draw_groups(buf, line);
        }
        if let Some(line) = sections {
            self.draw_sections(buf, line);
        }
    }
}

impl NavBar<'_> {
    pub(crate) fn draw_groups(&self, buf: &mut Buffer, line: Rect) {
        let (form, start) = self.fit_groups(line);
        let row = Row {
            gap: GROUP_GAP,
            start,
            on: self.styles.lit(),
            off: self.styles.muted,
            underline: false,
        };
        draw_row(buf, line, self.group_labels(form), row);
    }

    fn draw_sections(&self, buf: &mut Buffer, line: Rect) {
        let (form, start) = self.fit_sections(line);
        let muted = self.styles.muted;
        let row = Row {
            gap: SECTION_GAP,
            start,
            on: if self.lit { self.styles.lit() } else { muted },
            off: muted,
            underline: self.underline,
        };
        draw_row(buf, line, self.section_labels(form), row);
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Trail {
    Whole,
    Ends,
    Last,
}

impl Trail {
    fn shown(self, index: usize, count: usize, crumb: &str) -> Option<&str> {
        let last = index + 1 == count;
        match self {
            Trail::Whole => Some(crumb),
            Trail::Ends if index == 0 || last => Some(crumb),
            Trail::Ends if index == 1 => Some(ELLIPSIS),
            Trail::Last if last => Some(crumb),
            Trail::Last if index == 0 => Some(ELLIPSIS),
            Trail::Ends | Trail::Last => None,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Breadcrumb<'a> {
    nav: &'a Nav,
    styles: Styles,
    centred: bool,
}

impl<'a> Breadcrumb<'a> {
    pub fn new(nav: &'a Nav) -> Self {
        Breadcrumb {
            nav,
            styles: Styles::new(),
            centred: false,
        }
    }

    pub fn styles(mut self, styles: Styles) -> Self {
        self.styles = styles;
        self
    }

    pub fn centred(mut self, centred: bool) -> Self {
        self.centred = centred;
        self
    }

    fn trail_width(&self, trail: Trail) -> u16 {
        let count = self.nav.crumbs().count();
        let join = text::width(JOIN);
        let mut first = true;
        let mut total = 0u16;
        for (index, crumb) in self.nav.crumbs().enumerate() {
            if let Some(shown) = trail.shown(index, count, crumb) {
                if !first {
                    total = total.saturating_add(join);
                }
                first = false;
                total = total.saturating_add(text::width(shown));
            }
        }
        total
    }

    fn draw_trail(&self, pen: &mut Pen, trail: Trail, room: u16, style: Style) {
        let count = self.nav.crumbs().count();
        let join = text::width(JOIN);
        let mut first = true;
        let mut elided = false;
        let stop = pen.x.saturating_add(room);
        for (index, crumb) in self.nav.crumbs().enumerate() {
            let Some(shown) = trail.shown(index, count, crumb) else {
                continue;
            };
            let left = stop.saturating_sub(pen.x);
            let wide = text::width(shown);
            if first {
                pen.clip(shown, style, left);
                if wide > left {
                    return;
                }
            } else if join.saturating_add(wide) <= left {
                pen.put(JOIN, style);
                pen.put(shown, style);
            } else {
                if left > join {
                    pen.put(JOIN, style);
                    pen.clip(shown, style, left - join);
                } else if !elided {
                    pen.clip(ELLIPSIS, style, left);
                }
                return;
            }
            first = false;
            elided = shown == ELLIPSIS;
        }
    }
}

impl Widget for Breadcrumb<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        (&self).render(area, buf);
    }
}

impl Widget for &Breadcrumb<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        let style = self.styles.lit();
        let rule = self.styles.rule;
        let Some(mut pen) = Pen::new(buf, area, area.x, area.y) else {
            return;
        };
        let room = area.width.saturating_sub(4);
        if self.nav.crumbs().next().is_none() || room == 0 {
            pen.fill(RULE, rule);
            return;
        }
        let count = self.nav.crumbs().count();
        let trail = [Trail::Whole, Trail::Ends, Trail::Last]
            .into_iter()
            .filter(|trail| *trail != Trail::Ends || count > 2)
            .find(|trail| self.trail_width(*trail) <= room)
            .unwrap_or(Trail::Last);
        if self.centred {
            let block = self.trail_width(trail).min(room) + 2;
            pen.fill_to(
                area.x.saturating_add(area.width.saturating_sub(block) / 2),
                RULE,
                rule,
            );
        } else {
            pen.put(RULE, rule);
        }
        pen.put(" ", rule);
        self.draw_trail(&mut pen, trail, room, style);
        pen.put(" ", rule);
        pen.fill(RULE, rule);
    }
}
