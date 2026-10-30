use std::borrow::Cow;

use crate::footer::{Edit, Input, InputBar, Key, Tone};
use crate::list::{Column, Keys, List, ListView, Row, Step};
use crossterm::event::KeyEvent;

use crate::palette::Palette;

const START: &[Key] = &[Key::Char('/')];
const CLEAR: &[Key] = &[Key::Esc];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Turn {
    Pass,
    Taken,
    Filtered,
    Open(usize),
}

#[derive(Debug, Clone)]
pub struct Filter {
    list: List,
    input: Input,
    typing: bool,
    shown: Vec<usize>,
    label: Cow<'static, str>,
    placeholder: Cow<'static, str>,
    hint: Cow<'static, str>,
    start: &'static [Key],
    clear: &'static [Key],
}

impl Filter {
    pub fn new(label: impl Into<Cow<'static, str>>) -> Self {
        Filter {
            list: List::new(),
            input: Input::new(),
            typing: false,
            shown: Vec::new(),
            label: label.into(),
            placeholder: Cow::Borrowed(""),
            hint: Cow::Borrowed(""),
            start: START,
            clear: CLEAR,
        }
    }

    pub fn placeholder(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.placeholder = text.into();
        self
    }

    pub fn hint(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.hint = text.into();
        self
    }

    pub fn keys(mut self, keys: Keys) -> Self {
        self.list = self.list.keys(keys);
        self
    }

    pub fn start_keys(mut self, keys: &'static [Key]) -> Self {
        self.start = keys;
        self
    }

    pub fn clear_keys(mut self, keys: &'static [Key]) -> Self {
        self.clear = keys;
        self
    }

    pub fn input(mut self, input: Input) -> Self {
        self.input = input;
        self
    }

    pub fn typing(&self) -> bool {
        self.typing
    }

    pub fn query(&self) -> &str {
        self.input.value()
    }

    pub fn active(&self) -> bool {
        !self.input.value().trim().is_empty()
    }

    pub fn shown(&self) -> &[usize] {
        &self.shown
    }

    pub fn current(&self) -> Option<usize> {
        self.list
            .selected()
            .and_then(|at| self.shown.get(at).copied())
    }

    pub fn list(&self) -> &List {
        &self.list
    }

    pub fn list_mut(&mut self) -> &mut List {
        &mut self.list
    }

    pub fn sift<R>(
        &mut self,
        rows: &[R],
        keeps: impl Fn(&R, &str) -> bool,
        line: impl Fn(&R) -> Row,
    ) {
        let kept = self.current();
        let query = self.input.value();
        self.shown = rows
            .iter()
            .enumerate()
            .filter(|(_, row)| keeps(row, query))
            .map(|(index, _)| index)
            .collect();
        let lines: Vec<Row> = self.shown.iter().map(|&index| line(&rows[index])).collect();
        self.list.set_rows(lines);
        let at = kept
            .and_then(|kept| self.shown.iter().position(|&index| index == kept))
            .unwrap_or(0);
        self.list.select(at);
    }

    pub fn key(&mut self, key: KeyEvent) -> Turn {
        if self.typing {
            return match self.input.key(key.into()) {
                Edit::Changed => Turn::Filtered,
                Edit::Submit(_) => {
                    self.typing = false;
                    Turn::Taken
                }
                Edit::Cancel => {
                    self.typing = false;
                    self.input.clear();
                    Turn::Filtered
                }
                Edit::Pass => match self.list.key(key.into()) {
                    Step::Pass | Step::Open(_) => Turn::Pass,
                    _ => Turn::Taken,
                },
                _ => Turn::Taken,
            };
        }
        let pressed: Key = key.into();
        if self.start.contains(&pressed) {
            self.typing = true;
            return Turn::Taken;
        }
        if self.active() && self.clear.contains(&pressed) {
            self.input.clear();
            return Turn::Filtered;
        }
        match self.list.key(key.into()) {
            Step::Pass => Turn::Pass,
            Step::Open(at) => self
                .shown
                .get(at)
                .map_or(Turn::Taken, |&index| Turn::Open(index)),
            _ => Turn::Taken,
        }
    }

    pub fn paste(&mut self, text: &str) -> Turn {
        if !self.typing {
            return Turn::Pass;
        }
        match self.input.paste(text) {
            Edit::Changed => Turn::Filtered,
            _ => Turn::Taken,
        }
    }

    pub fn bar(&self) -> Option<InputBar<'_>> {
        self.typing.then(|| {
            let bar = InputBar::new(&self.label, &self.input)
                .placeholder(&self.placeholder)
                .tone(Tone::Accent);
            if self.hint.is_empty() {
                bar
            } else {
                bar.hint(&self.hint)
            }
        })
    }

    pub fn view<'a>(&'a mut self, columns: &'a [Column<'a>], palette: &Palette) -> ListView<'a> {
        palette.view(&mut self.list, columns)
    }
}

pub fn matches(text: &str, query: &str) -> bool {
    let text = text.to_lowercase();
    query
        .split_whitespace()
        .all(|word| text.contains(&word.to_lowercase()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crossterm::event::{KeyCode, KeyEventKind, KeyModifiers};

    const NAMES: [&str; 5] = ["alpha", "Beta", "gamma", "alphabet", "Ștefan Ăla"];

    fn press(code: KeyCode) -> KeyEvent {
        KeyEvent::new(code, KeyModifiers::NONE)
    }

    fn char_key(c: char) -> KeyEvent {
        press(KeyCode::Char(c))
    }

    fn sift(filter: &mut Filter) {
        filter.sift(
            &NAMES,
            |name, query| matches(name, query),
            |name| Row::new([*name]),
        );
    }

    fn typed(filter: &mut Filter, text: &str) {
        for c in text.chars() {
            assert_eq!(filter.key(char_key(c)), Turn::Filtered, "typing {c:?}");
        }
        sift(filter);
    }

    fn searching(text: &str) -> Filter {
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        assert_eq!(filter.key(char_key('/')), Turn::Taken);
        typed(&mut filter, text);
        filter
    }

    #[test]
    fn matches_needs_every_word_anywhere_in_any_case() {
        assert!(matches("Hello World", ""));
        assert!(matches("Hello World", "   "));
        assert!(matches("", ""));
        assert!(matches("Hello World", "hello"));
        assert!(matches("Hello World", "WORLD"));
        assert!(matches("Hello World", "wor hel"));
        assert!(matches("Hello World", "lo wo"));
        assert!(matches("Hello World", "  hello\tworld  "));
        assert!(!matches("Hello World", "xyz"));
        assert!(!matches("Hello World", "hello xyz"));
        assert!(!matches("", "a"));
        assert!(!matches("Hello World", "helloworld"));
    }

    #[test]
    fn matches_folds_case_beyond_ascii() {
        assert!(matches("Ștefan Ăla", "ștefan"));
        assert!(matches("Ștefan Ăla", "ĂLA"));
        assert!(matches("ΣΟΦΙΑ", "σοφια"));
        assert!(matches("日本語のテキスト", "テキスト"));
        assert!(matches("café", "CAFÉ"));
        assert!(!matches("日本語", "中"));
    }

    #[test]
    fn a_new_filter_is_idle_empty_and_shows_no_bar() {
        let filter = Filter::new("Find");
        assert!(!filter.typing());
        assert_eq!(filter.query(), "");
        assert!(!filter.active());
        assert!(filter.shown().is_empty());
        assert_eq!(filter.current(), None);
        assert!(filter.bar().is_none());
    }

    #[test]
    fn an_empty_query_shows_every_row_with_the_first_selected() {
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        assert_eq!(filter.shown(), [0, 1, 2, 3, 4]);
        assert_eq!(filter.current(), Some(0));
        assert_eq!(filter.list().len(), 5);
    }

    #[test]
    fn the_start_key_opens_the_bar_and_typing_narrows_the_rows() {
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        assert_eq!(filter.key(char_key('/')), Turn::Taken);
        assert!(filter.typing());
        assert!(filter.bar().is_some());
        typed(&mut filter, "al");
        assert_eq!(filter.query(), "al");
        assert!(filter.active());
        assert_eq!(filter.shown(), [0, 3]);
        assert_eq!(filter.current(), Some(0));
    }

    #[test]
    fn typed_queries_ignore_case_and_unicode_case() {
        assert_eq!(searching("ALP").shown(), [0, 3]);
        assert_eq!(searching("beta").shown(), [1]);
        assert_eq!(searching("ăla").shown(), [4]);
        assert_eq!(searching("ș").shown(), [4]);
    }

    #[test]
    fn a_query_that_matches_nothing_shows_nothing_and_opens_nothing() {
        let mut filter = searching("zzz");
        assert!(filter.shown().is_empty());
        assert_eq!(filter.current(), None);
        assert_eq!(filter.key(press(KeyCode::Enter)), Turn::Taken);
        assert!(!filter.typing());
        assert_eq!(filter.key(press(KeyCode::Enter)), Turn::Taken);
        assert_eq!(filter.key(press(KeyCode::Down)), Turn::Taken);
    }

    #[test]
    fn arrows_move_the_list_while_typing_and_enter_keeps_the_query() {
        let mut filter = searching("al");
        assert_eq!(filter.key(press(KeyCode::Down)), Turn::Taken);
        assert_eq!(filter.current(), Some(3));
        assert_eq!(filter.key(press(KeyCode::Enter)), Turn::Taken);
        assert!(!filter.typing());
        assert!(filter.bar().is_none());
        assert_eq!(filter.query(), "al");
        assert_eq!(filter.key(press(KeyCode::Enter)), Turn::Open(3));
    }

    #[test]
    fn open_reports_the_index_in_the_apps_rows_not_the_shown_row() {
        let mut filter = searching("gamma");
        filter.key(press(KeyCode::Enter));
        assert_eq!(filter.shown(), [2]);
        assert_eq!(filter.key(press(KeyCode::Enter)), Turn::Open(2));
    }

    #[test]
    fn escape_while_typing_drops_the_query_and_closes_the_bar() {
        let mut filter = searching("bet");
        assert_eq!(filter.key(press(KeyCode::Esc)), Turn::Filtered);
        assert!(!filter.typing());
        assert_eq!(filter.query(), "");
        assert!(!filter.active());
        sift(&mut filter);
        assert_eq!(filter.shown().len(), NAMES.len());
    }

    #[test]
    fn the_clear_key_drops_a_kept_query_and_keeps_the_selected_row() {
        let mut filter = searching("al");
        filter.key(press(KeyCode::Down));
        filter.key(press(KeyCode::Enter));
        assert_eq!(filter.current(), Some(3));
        assert_eq!(filter.key(press(KeyCode::Esc)), Turn::Filtered);
        assert_eq!(filter.query(), "");
        sift(&mut filter);
        assert_eq!(filter.shown(), [0, 1, 2, 3, 4]);
        assert_eq!(filter.current(), Some(3));
    }

    #[test]
    fn the_clear_key_passes_when_there_is_no_query() {
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        assert_eq!(filter.key(press(KeyCode::Esc)), Turn::Pass);
        let mut spaces = searching("  ");
        spaces.key(press(KeyCode::Enter));
        assert!(!spaces.active());
        assert_eq!(spaces.key(press(KeyCode::Esc)), Turn::Pass);
        assert_eq!(spaces.query(), "  ");
    }

    #[test]
    fn a_selection_that_is_filtered_out_falls_back_to_the_first_row() {
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        filter.key(press(KeyCode::Down));
        assert_eq!(filter.current(), Some(1));
        filter.key(char_key('/'));
        typed(&mut filter, "gam");
        assert_eq!(filter.current(), Some(2));
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        filter.key(press(KeyCode::Down));
        filter.key(char_key('/'));
        typed(&mut filter, "al");
        assert_eq!(filter.current(), Some(0));
    }

    #[test]
    fn keys_the_filter_and_list_do_not_use_pass_to_the_app() {
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        assert_eq!(filter.key(char_key('q')), Turn::Pass);
        assert_eq!(filter.key(press(KeyCode::Tab)), Turn::Pass);
        let mut typing = searching("");
        assert_eq!(typing.key(press(KeyCode::Tab)), Turn::Pass);
        assert_eq!(typing.key(press(KeyCode::F(5))), Turn::Pass);
        let release = KeyEvent::new_with_kind(
            KeyCode::Char('x'),
            KeyModifiers::NONE,
            KeyEventKind::Release,
        );
        assert_eq!(typing.key(release), Turn::Pass);
        assert_eq!(typing.query(), "");
    }

    #[test]
    fn editing_keys_that_change_nothing_are_still_taken_while_typing() {
        let mut filter = searching("");
        assert_eq!(filter.key(press(KeyCode::Backspace)), Turn::Taken);
        assert_eq!(filter.key(press(KeyCode::Left)), Turn::Taken);
        typed(&mut filter, "ab");
        assert_eq!(filter.key(press(KeyCode::Backspace)), Turn::Filtered);
        assert_eq!(filter.query(), "a");
        assert_eq!(filter.key(char_key('/')), Turn::Filtered);
        assert_eq!(filter.query(), "a/");
    }

    #[test]
    fn paste_only_reaches_an_open_bar() {
        let mut filter = Filter::new("Find");
        assert_eq!(filter.paste("alpha"), Turn::Pass);
        assert_eq!(filter.query(), "");
        filter.key(char_key('/'));
        assert_eq!(filter.paste("alp"), Turn::Filtered);
        assert_eq!(filter.query(), "alp");
        assert_eq!(filter.paste(""), Turn::Taken);
        sift(&mut filter);
        assert_eq!(filter.shown(), [0, 3]);
    }

    #[test]
    fn apps_can_rebind_the_start_and_clear_keys() {
        const FIND: &[Key] = &[Key::Ctrl('f')];
        const WIPE: &[Key] = &[Key::Char('x')];
        let mut filter = Filter::new("Find").start_keys(FIND).clear_keys(WIPE);
        sift(&mut filter);
        assert_eq!(filter.key(char_key('/')), Turn::Pass);
        assert!(!filter.typing());
        let ctrl_f = KeyEvent::new(KeyCode::Char('f'), KeyModifiers::CONTROL);
        assert_eq!(filter.key(ctrl_f), Turn::Taken);
        assert!(filter.typing());
        typed(&mut filter, "be");
        filter.key(press(KeyCode::Enter));
        assert_eq!(filter.key(press(KeyCode::Esc)), Turn::Pass);
        assert_eq!(filter.query(), "be");
        assert_eq!(filter.key(char_key('x')), Turn::Filtered);
        assert_eq!(filter.query(), "");
    }

    #[test]
    fn apps_can_rebind_the_list_keys() {
        let mut filter = Filter::new("Find").keys(Keys::VIM);
        sift(&mut filter);
        assert_eq!(filter.key(char_key('j')), Turn::Taken);
        assert_eq!(filter.current(), Some(1));
        assert_eq!(filter.key(char_key('G')), Turn::Taken);
        assert_eq!(filter.current(), Some(4));
    }

    #[test]
    fn a_seeded_input_starts_the_filter_active() {
        let mut filter = Filter::new("Find").input(Input::new().with("gam"));
        assert!(filter.active());
        assert!(!filter.typing());
        sift(&mut filter);
        assert_eq!(filter.shown(), [2]);
        assert_eq!(filter.current(), Some(2));
    }

    #[test]
    fn placeholder_and_hint_leave_the_filters_state_alone() {
        let mut filter = Filter::new("Find")
            .placeholder("type to filter")
            .hint("esc clears");
        assert!(filter.bar().is_none());
        filter.key(char_key('/'));
        assert!(filter.bar().is_some());
        assert_eq!(filter.query(), "");
    }

    #[test]
    fn list_mut_reaches_the_same_list_the_filter_reads() {
        let mut filter = Filter::new("Find");
        sift(&mut filter);
        filter.list_mut().select(4);
        assert_eq!(filter.current(), Some(4));
        assert_eq!(filter.list().selected(), Some(4));
    }

    #[test]
    fn the_view_draws_only_the_rows_the_query_kept() {
        use ratatui::buffer::Buffer;
        use ratatui::layout::Rect;
        use ratatui::widgets::Widget;
        let mut filter = searching("alpha");
        let columns = [Column::new("Name", 4, 10)];
        let area = Rect::new(0, 0, 12, 4);
        let mut buffer = Buffer::empty(area);
        filter
            .view(&columns, &Palette::new(ratatui::style::Color::Blue))
            .header(false)
            .render(area, &mut buffer);
        let lines: Vec<String> = (0..area.height)
            .map(|y| {
                (0..area.width)
                    .map(|x| buffer.cell((x, y)).map_or(" ", |cell| cell.symbol()))
                    .collect::<String>()
                    .trim_end()
                    .to_string()
            })
            .collect();
        assert!(lines[0].contains("alpha"), "{lines:?}");
        assert!(lines[1].contains("alphabet"), "{lines:?}");
        assert!(
            lines
                .iter()
                .all(|line| !line.contains("Beta") && !line.contains("gamma")),
            "{lines:?}"
        );
        assert!(lines[2].is_empty() && lines[3].is_empty(), "{lines:?}");
    }
}
