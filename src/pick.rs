use std::borrow::Cow;

use pito_footer::{Hint, Key};
use pito_list::{Cell, Column, Keys, List, ListView, Row};
use ratatui::{
    Frame,
    layout::Rect,
    text::Line,
    widgets::{Block, BorderType, Clear},
};

use crate::palette::Palette;
use crate::text::cells;

const CANCEL: &[Key] = &[Key::Esc, Key::Char('q')];
const GAP: usize = 2;
const NARROWEST: usize = 24;

#[non_exhaustive]
pub struct Pick {
    pub(crate) title: Cow<'static, str>,
    pub(crate) rows: List,
    pub(crate) hints: Vec<Hint<'static>>,
    pub(crate) cancel: &'static [Key],
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

    pub(crate) fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let rows = self.rows.rows();
        let count = rows.iter().map(|row| row.cells().len()).max().unwrap_or(1);
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
        let mut columns = vec![Column::new("", 4, 0).flex()];
        columns.extend((1..count).map(|_| Column::new("", 0, 0).fit(40).right()));
        let view = ListView::new(&mut self.rows, &columns)
            .styles(palette.list())
            .header(false);
        frame.render_widget(view, inner);
    }
}
