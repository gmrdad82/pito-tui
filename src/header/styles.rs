use ratatui::style::{Modifier, Style};

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Styles {
    pub accent: Style,
    pub muted: Style,
    pub rule: Style,
}

impl Styles {
    pub const fn new() -> Self {
        Styles {
            accent: Style::new(),
            muted: Style::new(),
            rule: Style::new(),
        }
    }

    pub const fn accent(mut self, style: Style) -> Self {
        self.accent = style;
        self
    }

    pub const fn muted(mut self, style: Style) -> Self {
        self.muted = style;
        self
    }

    pub const fn rule(mut self, style: Style) -> Self {
        self.rule = style;
        self
    }

    pub(crate) fn lit(&self) -> Style {
        self.accent.add_modifier(Modifier::BOLD)
    }
}
