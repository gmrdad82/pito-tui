use std::time::Duration;

use pito_footer::Styles as FooterStyles;
use pito_header::Styles as HeaderStyles;
use pito_hourglass::Hourglass;
use pito_list::Styles as ListStyles;
use ratatui::style::{Color, Modifier, Style};

const GOOD: Color = Color::Rgb(0x8c, 0xe6, 0xb4);
const BAD: Color = Color::Rgb(0xff, 0x6e, 0xc7);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Palette {
    pub base: Style,
    pub ink: Style,
    pub muted: Style,
    pub accent: Style,
    pub selected: Style,
    pub good: Style,
    pub bad: Style,
    pub rule: Style,
    pub line: Style,
    pub glass: Style,
    pub shimmer: Style,
}

impl Palette {
    pub const fn new(accent: Color) -> Self {
        let lit = Style::new().fg(accent);
        let muted = Style::new().add_modifier(Modifier::DIM);
        Palette {
            base: Style::new(),
            ink: Style::new(),
            muted,
            accent: lit,
            selected: lit.add_modifier(Modifier::BOLD),
            good: Style::new().fg(GOOD),
            bad: Style::new().fg(BAD),
            rule: lit.add_modifier(Modifier::BOLD),
            line: muted,
            glass: lit,
            shimmer: lit,
        }
    }

    pub const fn base(mut self, style: Style) -> Self {
        self.base = style;
        self
    }

    pub const fn ink(mut self, style: Style) -> Self {
        self.ink = style;
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
