use std::time::Duration;

use pito_footer::Styles as FooterStyles;
use pito_header::Styles as HeaderStyles;
use pito_hourglass::Hourglass;
use pito_list::{Bar, Column, Count, List, ListView, Source, Styles as ListStyles};
use ratatui::style::{Color, Modifier, Style};

const GOOD: Color = Color::Rgb(0x8c, 0xe6, 0xb4);
const BAD: Color = Color::Rgb(0xff, 0x6e, 0xc7);
const WARN: Color = Color::Rgb(0xff, 0xc8, 0x57);

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Tone {
    #[default]
    Ink,
    Strong,
    Muted,
    Accent,
    Good,
    Warn,
    Bad,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Palette {
    pub base: Style,
    pub ink: Style,
    pub strong: Style,
    pub muted: Style,
    pub accent: Style,
    pub selected: Style,
    pub good: Style,
    pub warn: Style,
    pub bad: Style,
    pub rule: Style,
    pub line: Style,
    pub glass: Style,
    pub shimmer: Style,
    pub title: Style,
    pub inactive: Style,
    pub focus: Style,
    pub small: Style,
    pub bar: Option<Bar<'static>>,
    pub count: Option<Count<'static>>,
    pub mono: bool,
}

const fn bare(style: Style) -> Style {
    let mut style = style;
    style.fg = None;
    style.bg = None;
    style
}

impl Palette {
    pub const fn new(accent: Color) -> Self {
        let lit = Style::new().fg(accent);
        let muted = Style::new().add_modifier(Modifier::DIM);
        Palette {
            base: Style::new(),
            ink: Style::new(),
            strong: Style::new().add_modifier(Modifier::BOLD),
            muted,
            accent: lit,
            selected: lit.add_modifier(Modifier::BOLD),
            good: Style::new().fg(GOOD),
            warn: Style::new().fg(WARN),
            bad: Style::new().fg(BAD),
            rule: lit.add_modifier(Modifier::BOLD),
            line: muted,
            glass: lit,
            shimmer: lit,
            title: lit.add_modifier(Modifier::BOLD),
            inactive: muted,
            focus: lit
                .add_modifier(Modifier::BOLD)
                .add_modifier(Modifier::REVERSED),
            small: Style::new(),
            bar: None,
            count: None,
            mono: false,
        }
    }

    pub const fn mono(self) -> Self {
        Palette {
            base: bare(self.base),
            ink: bare(self.ink),
            strong: bare(self.strong).add_modifier(Modifier::BOLD),
            muted: bare(self.muted).add_modifier(Modifier::DIM),
            accent: bare(self.accent).add_modifier(Modifier::BOLD),
            selected: bare(self.selected).add_modifier(Modifier::REVERSED),
            good: bare(self.good).add_modifier(Modifier::BOLD),
            warn: bare(self.warn)
                .add_modifier(Modifier::BOLD)
                .add_modifier(Modifier::UNDERLINED),
            bad: bare(self.bad).add_modifier(Modifier::REVERSED),
            rule: bare(self.rule).add_modifier(Modifier::DIM),
            line: bare(self.line).add_modifier(Modifier::DIM),
            glass: bare(self.glass).add_modifier(Modifier::BOLD),
            shimmer: bare(self.shimmer).add_modifier(Modifier::BOLD),
            title: bare(self.title).add_modifier(Modifier::BOLD),
            inactive: bare(self.inactive).add_modifier(Modifier::DIM),
            focus: bare(self.focus).add_modifier(Modifier::REVERSED),
            small: bare(self.small),
            bar: self.bar,
            count: self.count,
            mono: true,
        }
    }

    pub const fn mono_style(style: Style) -> Style {
        bare(style)
    }

    pub const fn base(mut self, style: Style) -> Self {
        self.base = style;
        self
    }

    pub const fn ink(mut self, style: Style) -> Self {
        self.ink = style;
        self
    }

    pub const fn strong(mut self, style: Style) -> Self {
        self.strong = style;
        self
    }

    pub const fn muted(mut self, style: Style) -> Self {
        self.muted = style;
        self
    }

    pub const fn accent(mut self, style: Style) -> Self {
        self.accent = style;
        self
    }

    pub const fn selected(mut self, style: Style) -> Self {
        self.selected = style;
        self
    }

    pub const fn good(mut self, style: Style) -> Self {
        self.good = style;
        self
    }

    pub const fn warn(mut self, style: Style) -> Self {
        self.warn = style;
        self
    }

    pub const fn bad(mut self, style: Style) -> Self {
        self.bad = style;
        self
    }

    pub const fn rule(mut self, style: Style) -> Self {
        self.rule = style;
        self
    }

    pub const fn line(mut self, style: Style) -> Self {
        self.line = style;
        self
    }

    pub const fn glass(mut self, style: Style) -> Self {
        self.glass = style;
        self
    }

    pub const fn shimmer(mut self, style: Style) -> Self {
        self.shimmer = style;
        self
    }

    pub const fn title(mut self, style: Style) -> Self {
        self.title = style;
        self
    }

    pub const fn inactive(mut self, style: Style) -> Self {
        self.inactive = style;
        self
    }

    pub const fn focus(mut self, style: Style) -> Self {
        self.focus = style;
        self
    }

    pub const fn small(mut self, style: Style) -> Self {
        self.small = style;
        self
    }

    pub const fn bar(mut self, bar: Option<Bar<'static>>) -> Self {
        self.bar = bar;
        self
    }

    pub const fn count(mut self, count: Option<Count<'static>>) -> Self {
        self.count = count;
        self
    }

    pub const fn tone(&self, tone: Tone) -> Style {
        match tone {
            Tone::Ink => self.ink,
            Tone::Strong => self.strong,
            Tone::Muted => self.muted,
            Tone::Accent => self.accent.add_modifier(Modifier::BOLD),
            Tone::Good => self.good,
            Tone::Warn => self.warn,
            Tone::Bad => self.bad,
        }
    }

    pub const fn header(&self) -> HeaderStyles {
        HeaderStyles::new()
            .accent(self.accent)
            .muted(self.muted)
            .rule(self.rule)
    }

    pub const fn footer(&self) -> FooterStyles {
        FooterStyles::new()
            .accent(self.accent)
            .muted(self.muted)
            .alert(self.bad)
            .ink(self.ink)
            .rule(self.line)
            .good(self.good)
    }

    pub const fn list(&self) -> ListStyles {
        ListStyles::new()
            .selected(self.selected)
            .ink(self.ink)
            .faint(self.muted)
            .base(self.base)
    }

    pub fn view<'a, S: Source>(
        &self,
        list: &'a mut List<S>,
        columns: &'a [Column<'a>],
    ) -> ListView<'a, S> {
        ListView::new(list, columns)
            .styles(self.list())
            .scrollbar(self.bar)
            .count(self.count)
    }

    pub fn hourglass<'a>(
        &self,
        elapsed: Duration,
        label: &'a str,
        hint: Option<&'a str>,
    ) -> Hourglass<'a> {
        Hourglass::new(elapsed)
            .label(label)
            .hint(hint.filter(|hint| !hint.is_empty()))
            .glass(self.glass)
            .accent(self.shimmer)
            .muted(self.muted)
    }
}
