use std::borrow::Cow;

use crate::header::{Group, Section};
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

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::style::{Color, Modifier};

    fn bold() -> Style {
        Style::new().add_modifier(Modifier::BOLD)
    }

    fn red() -> Style {
        Style::new().fg(Color::Red)
    }

    #[test]
    fn a_plain_label_is_one_unstyled_span_and_its_own_short_form() {
        let label = Label::new("Files");
        assert_eq!(label.text(), "Files");
        assert_eq!(label.short_text(), "Files");
        assert_eq!(label.full(), [(Cow::Borrowed("Files"), Style::new())]);
        assert_eq!(label.short_spans_or_full(), label.full());
        assert_eq!(label.section(), Section::new("Files"));
        assert_eq!(label.group(), Group::new("Files"));
    }

    #[test]
    fn an_empty_label_stays_empty_everywhere() {
        let label = Label::new("");
        assert_eq!(label.text(), "");
        assert_eq!(label.short_text(), "");
        let none: [(&str, Style); 0] = [];
        let spans = Label::spans(none);
        assert_eq!(spans.text(), "");
        assert!(spans.full().is_empty());
        assert_eq!(spans.short_text(), "");
    }

    #[test]
    fn styled_spans_join_into_the_name_and_keep_their_styles() {
        let label = Label::spans([("ab", bold()), ("cd", Style::new())]);
        assert_eq!(label.text(), "abcd");
        assert_eq!(label.short_text(), "abcd");
        assert_eq!(
            label.full(),
            [
                (Cow::Borrowed("ab"), bold()),
                (Cow::Borrowed("cd"), Style::new())
            ]
        );
        assert_eq!(
            label.section(),
            Section::spans(&[("ab", bold()), ("cd", Style::new())])
        );
        assert_eq!(label.group(), Group::new("abcd"));
    }

    #[test]
    fn spans_take_owned_and_borrowed_text_alike() {
        let owned = Label::spans([(String::from("日本"), red())]);
        let borrowed = Label::spans([("日本", red())]);
        assert_eq!(owned, borrowed);
        assert_eq!(owned.text(), "日本");
    }

    #[test]
    fn an_unstyled_span_list_makes_a_plain_section() {
        let label = Label::spans([("a", Style::new()), ("b", Style::new())]);
        assert_eq!(label.section(), Section::new("ab"));
    }

    #[test]
    fn a_short_form_replaces_only_the_short_text() {
        let label = Label::new("Library").short("Lib");
        assert_eq!(label.text(), "Library");
        assert_eq!(label.short_text(), "Lib");
        assert_eq!(label.full(), [(Cow::Borrowed("Library"), Style::new())]);
        assert_eq!(label.section(), Section::new("Library").short("Lib"));
        assert_eq!(label.group(), Group::new("Library").short("Lib"));
    }

    #[test]
    fn styled_short_spans_reach_the_section_and_join_for_the_group() {
        let label = Label::new("Library").short_spans([("L", red()), ("ib", Style::new())]);
        assert_eq!(label.short_text(), "Lib");
        assert_eq!(
            label.section(),
            Section::new("Library").short_spans(&[("L", red()), ("ib", Style::new())])
        );
        assert_eq!(label.group(), Group::new("Library").short("Lib"));
        let plain = Label::new("Library").short_spans([("Li", Style::new()), ("b", Style::new())]);
        assert_eq!(plain.section(), Section::new("Library").short("Lib"));
    }

    #[test]
    fn the_last_short_form_wins() {
        let label = Label::new("Settings")
            .short("Set")
            .short_spans([("S", bold())])
            .short("Cfg");
        assert_eq!(label.short_text(), "Cfg");
        assert_eq!(label.section(), Section::new("Settings").short("Cfg"));
    }

    #[test]
    fn strings_and_cows_convert_into_the_same_label() {
        let from_str: Label = "Inbox".into();
        let from_string: Label = String::from("Inbox").into();
        let from_cow: Label = Cow::<'static, str>::Owned("Inbox".into()).into();
        assert_eq!(from_str, Label::new("Inbox"));
        assert_eq!(from_string, Label::new("Inbox"));
        assert_eq!(from_cow, Label::new("Inbox"));
    }

    #[test]
    fn a_section_converts_whole_and_is_handed_back_unchanged() {
        let section =
            Section::spans(&[("Lib", red()), ("rary", bold())]).short_spans(&[("L", red())]);
        let label = Label::from(section.clone());
        assert_eq!(label.text(), "Library");
        assert_eq!(label.short_text(), "L");
        assert_eq!(label.section(), section);
    }

    #[test]
    fn a_section_without_a_short_form_gives_a_label_without_one() {
        let label = Label::from(Section::new("Home"));
        assert_eq!(label.text(), "Home");
        assert_eq!(label.short_text(), "Home");
        assert_eq!(label.section(), Section::new("Home"));
    }

    #[test]
    fn a_new_short_form_after_a_section_rebuilds_the_section() {
        let label = Label::from(Section::new("Library").short("Lib")).short("L");
        assert_eq!(label.short_text(), "L");
        assert_eq!(label.section(), Section::new("Library").short("L"));
        let spans = Label::from(Section::new("Library")).short_spans([("Li", red())]);
        assert_eq!(
            spans.section(),
            Section::new("Library").short_spans(&[("Li", red())])
        );
    }

    #[test]
    fn a_group_converts_with_its_name_and_short_form() {
        let label = Label::from(Group::new("Tools").short("T"));
        assert_eq!(label.text(), "Tools");
        assert_eq!(label.short_text(), "T");
        assert_eq!(label.group(), Group::new("Tools").short("T"));
        let bare = Label::from(Group::new("Tools"));
        assert_eq!(bare, Label::new("Tools"));
    }
}
