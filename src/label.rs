use std::borrow::Cow;

use pito_header::{Group, Section};
use ratatui::style::Style;

pub type Span = (Cow<'static, str>, Style);

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Label {
    name: String,
    spans: Vec<Span>,
    short: Option<Vec<Span>>,
    section: Option<Section>,
}

fn owned<T: Into<Cow<'static, str>>>(spans: impl IntoIterator<Item = (T, Style)>) -> Vec<Span> {
    spans
        .into_iter()
        .map(|(text, style)| (text.into(), style))
        .collect()
}

fn joined(spans: &[Span]) -> String {
    spans.iter().map(|(text, _)| text.as_ref()).collect()
}

fn plain(spans: &[Span]) -> bool {
    spans.iter().all(|(_, style)| *style == Style::new())
}

fn borrowed(spans: &[Span]) -> Vec<(&str, Style)> {
    spans
        .iter()
        .map(|(text, style)| (text.as_ref(), *style))
        .collect()
}

impl Label {
    pub fn new(text: impl Into<Cow<'static, str>>) -> Self {
        let text = text.into();
        Label {
            name: text.to_string(),
            spans: vec![(text, Style::new())],
            short: None,
            section: None,
        }
    }

    pub fn spans<T: Into<Cow<'static, str>>>(spans: impl IntoIterator<Item = (T, Style)>) -> Self {
        let spans = owned(spans);
        Label {
            name: joined(&spans),
            spans,
            short: None,
            section: None,
        }
    }

    pub fn short(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.short = Some(vec![(text.into(), Style::new())]);
        self.section = None;
        self
    }

    pub fn short_spans<T: Into<Cow<'static, str>>>(
        mut self,
        spans: impl IntoIterator<Item = (T, Style)>,
    ) -> Self {
        self.short = Some(owned(spans));
        self.section = None;
        self
    }

    pub fn text(&self) -> &str {
        &self.name
    }

    pub fn short_text(&self) -> String {
        joined(self.short_spans_or_full())
    }

    pub(crate) fn full(&self) -> &[Span] {
        &self.spans
    }

    pub(crate) fn short_spans_or_full(&self) -> &[Span] {
        self.short.as_deref().unwrap_or(&self.spans)
    }

    pub(crate) fn section(&self) -> Section {
        if let Some(section) = &self.section {
            return section.clone();
        }
        let section = if plain(&self.spans) {
            Section::new(self.name.clone())
        } else {
            Section::spans(&borrowed(&self.spans))
        };
        match &self.short {
            Some(short) if plain(short) => section.short(joined(short)),
            Some(short) => section.short_spans(&borrowed(short)),
            None => section,
        }
    }

    pub(crate) fn group(&self) -> Group {
        let group = Group::new(self.name.clone());
        match &self.short {
            Some(short) => group.short(joined(short)),
            None => group,
        }
    }
}

impl From<&'static str> for Label {
    fn from(text: &'static str) -> Self {
        Label::new(text)
    }
}

impl From<String> for Label {
    fn from(text: String) -> Self {
        Label::new(text)
    }
}

impl From<Cow<'static, str>> for Label {
    fn from(text: Cow<'static, str>) -> Self {
        Label::new(text)
    }
}

impl From<Section> for Label {
    fn from(section: Section) -> Self {
        let name = section.name().to_string();
        let short = section.short_name().to_string();
        let mut label = Label::new(name.clone());
        if short != name {
            label = label.short(short);
        }
        label.section = Some(section);
        label
    }
}

impl From<Group> for Label {
    fn from(group: Group) -> Self {
        let name = group.name().to_string();
        let short = group.short_name().to_string();
        let label = Label::new(name.clone());
        if short != name {
            label.short(short)
        } else {
            label
        }
    }
}
