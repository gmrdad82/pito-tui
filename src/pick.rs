use std::borrow::Cow;

use crate::footer::{Hint, Key};
use crate::list::{Cell, Column, Keys, List, Row};
use ratatui::{
    Frame,
    layout::Rect,
    text::Line,
    widgets::{Block, BorderType, Clear, Paragraph},
};

use crate::palette::Palette;
use crate::text::cells;

const CANCEL: &[Key] = &[Key::Esc, Key::Char('q')];
const GAP: usize = 2;
const NARROWEST: usize = 24;
const HEADING: u16 = 2;

#[non_exhaustive]
pub struct Pick {
    pub(crate) title: Cow<'static, str>,
    pub(crate) rows: List,
    pub(crate) hints: Vec<Hint<'static>>,
    pub(crate) cancel: &'static [Key],
    columns: Vec<Column<'static>>,
    full: bool,
}

impl Pick {
    pub fn new(
        title: impl Into<Cow<'static, str>>,
        options: impl IntoIterator<Item = impl Into<String>>,
    ) -> Self {
        let rows = options
            .into_iter()
            .map(|option| Row::new([Cell::new(option.into())]));
        Pick::rows(title, rows)
    }

    pub fn rows(title: impl Into<Cow<'static, str>>, rows: impl IntoIterator<Item = Row>) -> Self {
        Pick {
            title: title.into(),
            rows: List::new().with_rows(rows).keys(Keys::VIM),
            hints: Vec::new(),
            cancel: CANCEL,
            columns: Vec::new(),
            full: false,
        }
    }

    pub fn selected(mut self, index: usize) -> Self {
        self.rows.select(index);
        self
    }

    pub fn hints(mut self, hints: impl IntoIterator<Item = Hint<'static>>) -> Self {
        self.hints = hints.into_iter().collect();
        self
    }

    pub fn keys(mut self, keys: Keys) -> Self {
        self.rows = self.rows.keys(keys);
        self
    }

    pub fn cancel_keys(mut self, keys: &'static [Key]) -> Self {
        self.cancel = keys;
        self
    }

    pub fn columns(mut self, columns: impl IntoIterator<Item = Column<'static>>) -> Self {
        self.columns = columns.into_iter().collect();
        self
    }

    pub fn full(mut self, full: bool) -> Self {
        self.full = full;
        self
    }

    fn laid(&self, count: usize) -> Vec<Column<'static>> {
        if !self.columns.is_empty() {
            return self.columns.clone();
        }
        let mut columns = vec![Column::new("", 4, 0).flex()];
        columns.extend((1..count).map(|_| Column::new("", 0, 0).fit(40).right()));
        columns
    }

    fn fill(&mut self, frame: &mut Frame, area: Rect, palette: &Palette, count: usize) {
        frame.render_widget(Clear, area);
        frame.render_widget(Block::new().style(palette.base), area);
        let heading = Line::styled(self.title.to_string(), palette.selected);
        let top = Rect {
            height: area.height.min(1),
            ..area
        };
        frame.render_widget(Paragraph::new(heading), top);
        let below = area.height.min(HEADING);
        let list = Rect {
            y: area.y + below,
            height: area.height - below,
            ..area
        };
        let columns = self.laid(count);
        let view = palette.view(&mut self.rows, &columns).header(false);
        frame.render_widget(view, list);
    }

    pub(crate) fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let rows = self.rows.rows();
        let count = rows.iter().map(|row| row.cells().len()).max().unwrap_or(1);
        if self.full {
            self.fill(frame, area, palette, count);
            return;
        }
        let widest = rows
            .iter()
            .map(|row| {
                row.cells()
                    .iter()
                    .map(|cell| cells(cell.text()) + GAP)
                    .sum::<usize>()
            })
            .max()
            .unwrap_or(0);
        let wanted = (widest + 6).max(cells(&self.title) + 6).max(NARROWEST);
        let width = u16::try_from(wanted).unwrap_or(u16::MAX).min(area.width);
        let tall = u16::try_from(rows.len() + 2).unwrap_or(u16::MAX);
        let height = tall.min(area.height);
        let place = Rect::new(
            area.x + (area.width - width) / 2,
            area.y + (area.height - height) / 3,
            width,
            height,
        );
        frame.render_widget(Clear, place);
        let title = Line::styled(format!(" {} ", self.title), palette.selected);
        let block = Block::bordered()
            .border_type(BorderType::Rounded)
            .border_style(palette.rule)
            .style(palette.base)
            .title(title);
        let inner = block.inner(place);
        frame.render_widget(block, place);
        let columns = self.laid(count);
        let view = palette.view(&mut self.rows, &columns).header(false);
        frame.render_widget(view, inner);
    }
}

#[cfg(test)]
mod tests {
    use crate::header::Section;
    use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
    use ratatui::{Terminal, backend::TestBackend, style::Color};

    use super::*;
    use crate::dump;
    use crate::screen::{Cx, Screen};
    use crate::shell::Tui;

    struct Asker {
        options: Vec<String>,
        start: Option<usize>,
        picked: Vec<Option<usize>>,
        keys: Vec<KeyCode>,
    }

    impl Asker {
        fn new(count: usize) -> Self {
            Asker {
                options: (0..count)
                    .map(|index| format!("choice {index:02}"))
                    .collect(),
                start: None,
                picked: Vec::new(),
                keys: Vec::new(),
            }
        }
    }

    impl Screen<()> for Asker {
        fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

        fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, ()>) {
            self.keys.push(key.code);
            if key.code == KeyCode::Char('p') {
                let mut pick = Pick::new("Pick one", self.options.clone());
                if let Some(index) = self.start {
                    pick = pick.selected(index);
                }
                cx.pick(pick);
            }
        }

        fn picked(&mut self, choice: Option<usize>, _cx: &mut Cx<'_, ()>) {
            self.picked.push(choice);
        }
    }

    fn press(code: KeyCode) -> KeyEvent {
        KeyEvent::new(code, KeyModifiers::NONE)
    }

    fn asking(asker: Asker) -> Tui<()> {
        let mut tui = Tui::new("probe", "1.2.3", Color::Blue)
            .screen(Section::new("One"), asker)
            .screen(Section::new("Two"), Asker::new(0));
        tui.start();
        tui.key(press(KeyCode::Char('p')));
        tui
    }

    fn picked(tui: &Tui<()>) -> &[Option<usize>] {
        &tui.find::<Asker>().unwrap().picked
    }

    fn keys(tui: &mut Tui<()>, codes: &[KeyCode]) {
        for code in codes {
            tui.key(press(*code));
        }
    }

    #[test]
    fn arrows_move_the_choice_and_enter_picks_it() {
        let mut tui = asking(Asker::new(3));
        assert!(dump::text(&tui.frame(60, 16)).contains("Pick one"));
        keys(
            &mut tui,
            &[KeyCode::Down, KeyCode::Down, KeyCode::Up, KeyCode::Enter],
        );
        assert_eq!(picked(&tui), [Some(1)]);
        assert!(!dump::text(&tui.frame(60, 16)).contains("Pick one"));
        assert_eq!(tui.find::<Asker>().unwrap().keys, [KeyCode::Char('p')]);
    }

    #[test]
    fn vim_keys_and_the_ends_move_the_choice() {
        let mut tui = asking(Asker::new(5));
        keys(
            &mut tui,
            &[
                KeyCode::Char('j'),
                KeyCode::Char('j'),
                KeyCode::Char('k'),
                KeyCode::Enter,
            ],
        );
        tui.key(press(KeyCode::Char('p')));
        keys(&mut tui, &[KeyCode::Char('G'), KeyCode::Enter]);
        tui.key(press(KeyCode::Char('p')));
        keys(&mut tui, &[KeyCode::End, KeyCode::Home, KeyCode::Enter]);
        assert_eq!(picked(&tui), [Some(1), Some(4), Some(0)]);
    }

    #[test]
    fn the_pick_stops_at_its_first_and_last_choice() {
        let mut tui = asking(Asker::new(3));
        keys(&mut tui, &[KeyCode::Up, KeyCode::Enter]);
        tui.key(press(KeyCode::Char('p')));
        keys(&mut tui, &[KeyCode::Down; 9]);
        tui.key(press(KeyCode::Enter));
        assert_eq!(picked(&tui), [Some(0), Some(2)]);
    }

    #[test]
    fn a_digit_neither_picks_nor_leaves_for_another_screen() {
        let mut tui = asking(Asker::new(3));
        keys(&mut tui, &[KeyCode::Char('2'), KeyCode::Char('3')]);
        assert!(picked(&tui).is_empty());
        assert_eq!(tui.current(), Some(0));
        assert!(dump::text(&tui.frame(60, 16)).contains("Pick one"));
        tui.key(press(KeyCode::Enter));
        assert_eq!(picked(&tui), [Some(0)]);
    }

    #[test]
    fn escape_or_q_cancels_with_none() {
        let mut tui = asking(Asker::new(3));
        keys(&mut tui, &[KeyCode::Down, KeyCode::Esc]);
        tui.key(press(KeyCode::Char('p')));
        tui.key(press(KeyCode::Char('q')));
        assert_eq!(picked(&tui), [None, None]);
        assert!(!dump::text(&tui.frame(60, 16)).contains("Pick one"));
    }

    #[test]
    fn the_app_s_cancel_keys_replace_the_default_ones() {
        struct Custom(Vec<Option<usize>>);
        impl Screen<()> for Custom {
            fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}
            fn key(&mut self, _key: KeyEvent, cx: &mut Cx<'_, ()>) {
                cx.pick(Pick::new("Pick", ["a", "b"]).cancel_keys(&[Key::Char('x')]));
            }
            fn picked(&mut self, choice: Option<usize>, _cx: &mut Cx<'_, ()>) {
                self.0.push(choice);
            }
        }
        let mut tui =
            Tui::new("probe", "1.2.3", Color::Blue).screen(Section::new("One"), Custom(Vec::new()));
        tui.start();
        tui.key(press(KeyCode::Char('p')));
        keys(&mut tui, &[KeyCode::Char('q'), KeyCode::Esc]);
        assert!(tui.find::<Custom>().unwrap().0.is_empty());
        tui.key(press(KeyCode::Char('x')));
        assert_eq!(tui.find::<Custom>().unwrap().0, [None]);
    }

    #[test]
    fn a_single_choice_is_picked_whatever_the_arrows_do() {
        let mut tui = asking(Asker::new(1));
        keys(
            &mut tui,
            &[KeyCode::Down, KeyCode::Up, KeyCode::Down, KeyCode::Enter],
        );
        assert_eq!(picked(&tui), [Some(0)]);
    }

    #[test]
    fn a_pick_can_start_on_a_chosen_row() {
        let mut asker = Asker::new(4);
        asker.start = Some(2);
        let mut tui = asking(asker);
        keys(&mut tui, &[KeyCode::Down, KeyCode::Enter]);
        assert_eq!(picked(&tui), [Some(3)]);
    }

    #[test]
    fn choices_past_the_screen_height_scroll_to_keep_the_choice_in_view() {
        let mut tui = asking(Asker::new(40));
        let text = dump::text(&tui.frame(60, 16));
        assert!(
            text.contains("choice 00") && !text.contains("choice 39"),
            "{text}"
        );
        tui.key(press(KeyCode::End));
        let text = dump::text(&tui.frame(60, 16));
        assert!(
            text.contains("choice 39") && !text.contains("choice 00"),
            "{text}"
        );
        keys(&mut tui, &[KeyCode::Up; 3]);
        let text = dump::text(&tui.frame(60, 16));
        assert!(text.contains("choice 36"), "{text}");
        tui.key(press(KeyCode::Enter));
        assert_eq!(picked(&tui), [Some(36)]);
    }

    fn drawn(pick: &mut Pick, width: u16, height: u16) -> Vec<String> {
        let mut terminal = Terminal::new(TestBackend::new(width, height)).unwrap();
        terminal
            .draw(|frame| pick.draw(frame, frame.area(), &Palette::new(Color::Blue)))
            .unwrap();
        let text = dump::text(terminal.backend().buffer());
        let mut rows: Vec<String> = text.lines().map(str::to_string).collect();
        rows.resize(usize::from(height), String::new());
        rows
    }

    #[test]
    fn a_small_pick_is_a_rounded_box_as_tall_as_its_choices_with_its_title_on_top() {
        let mut pick = Pick::new("Colour", ["red", "green"]);
        let rows = drawn(&mut pick, 60, 20);
        let top = rows.iter().position(|row| row.contains("Colour")).unwrap();
        assert!(
            rows[top].contains('╭') && rows[top].contains('╮'),
            "{rows:?}"
        );
        assert!(rows[top + 1].contains("red"), "{rows:?}");
        assert!(rows[top + 2].contains("green"), "{rows:?}");
        assert!(rows[top + 3].contains('╰'), "{rows:?}");
        let left = rows[top].find('╭').unwrap();
        let width = rows[top].chars().count() - rows[top][..left].chars().count();
        assert_eq!(width, NARROWEST);
        assert_eq!(top, (20 - 4) / 3);
    }

    #[test]
    fn a_pick_never_draws_wider_or_taller_than_its_area() {
        let mut pick = Pick::new(
            "A title far wider than the screen it sits on",
            (0..50).map(|i| i.to_string()),
        );
        let rows = drawn(&mut pick, 20, 6);
        assert_eq!(rows.len(), 6);
        assert!(rows[0].starts_with('╭'), "{rows:?}");
        assert!(rows[5].starts_with('╰'), "{rows:?}");
        assert!(rows.iter().all(|row| row.chars().count() <= 20));
    }

    #[test]
    fn a_full_pick_fills_its_area_with_the_title_on_the_first_row_and_choices_after_a_gap() {
        let mut pick = Pick::new("Everything", ["one", "two"]).full(true);
        let rows = drawn(&mut pick, 40, 8);
        assert_eq!(rows[0], "Everything");
        assert_eq!(rows[1], "");
        assert!(
            rows[2].contains("one") && rows[3].contains("two"),
            "{rows:?}"
        );
        assert!(rows.iter().all(|row| !row.contains('╭')));
    }

    #[test]
    fn a_pick_with_no_choices_draws_its_box_and_title_alone() {
        let mut pick = Pick::new("Nothing", Vec::<String>::new());
        let rows = drawn(&mut pick, 40, 10);
        let top = rows.iter().position(|row| row.contains("Nothing")).unwrap();
        assert!(rows[top + 1].contains('╰'), "{rows:?}");
    }
}
