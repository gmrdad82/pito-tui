use ratatui::style::{Modifier, Style};

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub struct Styles {
    pub accent: Style,
    pub muted: Style,
    pub alert: Style,
    pub ink: Style,
    pub rule: Style,
    pub good: Option<Style>,
}

impl Styles {
    pub const fn new() -> Self {
        Styles {
            accent: Style::new(),
            muted: Style::new(),
            alert: Style::new(),
            ink: Style::new(),
            rule: Style::new(),
            good: None,
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

    pub const fn alert(mut self, style: Style) -> Self {
        self.alert = style;
        self
    }

    pub const fn ink(mut self, style: Style) -> Self {
        self.ink = style;
        self
    }

    pub const fn rule(mut self, style: Style) -> Self {
        self.rule = style;
        self
    }

    pub const fn good(mut self, style: Style) -> Self {
        self.good = Some(style);
        self
    }

    pub(crate) fn lit(&self) -> Style {
        self.accent.add_modifier(Modifier::BOLD)
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Tone {
    Ink,
    #[default]
    Muted,
    Accent,
    Alert,
    Good,
}

impl Tone {
    pub(crate) fn style(self, styles: &Styles) -> Style {
        match self {
            Tone::Ink => styles.ink,
            Tone::Muted => styles.muted,
            Tone::Accent => styles.lit(),
            Tone::Alert => styles.alert,
            Tone::Good => styles.good.unwrap_or(styles.ink),
        }
    }
}
