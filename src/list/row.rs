use ratatui::style::Style;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Paint {
    Ink,
    Faint,
    Own(Style),
}

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Part {
    pub(crate) text: String,
    pub(crate) paint: Option<Paint>,
}

impl Part {
    pub fn new(text: impl Into<String>) -> Self {
        Part {
            text: text.into(),
            paint: None,
        }
    }

    pub fn faint(mut self) -> Self {
        self.paint = Some(Paint::Faint);
        self
    }

    pub fn style(mut self, style: Style) -> Self {
        self.paint = Some(Paint::Own(style));
        self
    }

    pub fn text(&self) -> &str {
        &self.text
    }
}

impl From<&str> for Part {
    fn from(text: &str) -> Self {
        Part::new(text)
    }
}

impl From<String> for Part {
    fn from(text: String) -> Self {
        Part::new(text)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct Run {
    end: usize,
    paint: Option<Paint>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Cell {
    pub(crate) text: String,
    pub(crate) paint: Paint,
    pub(crate) runs: Vec<Run>,
    pub(crate) rest: bool,
}

impl Cell {
    pub fn new(text: impl Into<String>) -> Self {
        Cell {
            text: text.into(),
            paint: Paint::Ink,
            runs: Vec::new(),
            rest: false,
        }
    }

    pub fn parts<I, P>(parts: I) -> Self
    where
        I: IntoIterator<Item = P>,
        P: Into<Part>,
    {
        let mut cell = Cell::new(String::new());
        for part in parts {
            let part = part.into();
            cell.text.push_str(&part.text);
            cell.runs.push(Run {
                end: cell.text.len(),
                paint: part.paint,
            });
        }
        cell
    }

    pub fn faint(mut self) -> Self {
        self.paint = Paint::Faint;
        self
    }

    pub fn style(mut self, style: Style) -> Self {
        self.paint = Paint::Own(style);
        self
    }

    pub fn rest(mut self) -> Self {
        self.rest = true;
        self
    }

    pub fn text(&self) -> &str {
        &self.text
    }

    pub fn set_text(&mut self, text: &str) {
        self.text.clear();
        self.text.push_str(text);
        self.runs.clear();
    }

    pub fn set_part(&mut self, index: usize, text: &str) -> bool {
        if self.runs.is_empty() {
            if index == 0 {
                self.text.clear();
                self.text.push_str(text);
            }
            return index == 0;
        }
        let Some(end) = self.runs.get(index).map(|run| run.end) else {
            return false;
        };
        let start = index
            .checked_sub(1)
            .map_or(0, |before| self.runs[before].end);
        self.text.replace_range(start..end, text);
        for run in &mut self.runs[index..] {
            run.end = run.end - (end - start) + text.len();
        }
        true
    }

    pub fn set_style(&mut self, style: Style) {
        self.paint = Paint::Own(style);
    }

    pub fn set_part_style(&mut self, index: usize, style: Style) -> bool {
        if self.runs.is_empty() {
            if index == 0 {
                self.paint = Paint::Own(style);
            }
            return index == 0;
        }
        let Some(run) = self.runs.get_mut(index) else {
            return false;
        };
        run.paint = Some(Paint::Own(style));
        true
    }

    pub(crate) fn pieces(&self) -> Pieces<'_> {
        Pieces {
            cell: self,
            at: 0,
            start: 0,
        }
    }
}

#[derive(Debug, Clone)]
pub(crate) struct Pieces<'c> {
    cell: &'c Cell,
    at: usize,
    start: usize,
}

impl<'c> Iterator for Pieces<'c> {
    type Item = (&'c str, Paint);

    fn next(&mut self) -> Option<Self::Item> {
        let cell = self.cell;
        if cell.runs.is_empty() {
            if self.at > 0 {
                return None;
            }
            self.at = 1;
            return Some((cell.text.as_str(), cell.paint));
        }
        let run = cell.runs.get(self.at)?;
        let text = cell.text.get(self.start..run.end).unwrap_or("");
        self.start = run.end;
        self.at += 1;
        Some((text, run.paint.unwrap_or(cell.paint)))
    }
}

impl From<&str> for Cell {
    fn from(text: &str) -> Self {
        Cell::new(text)
    }
}

impl From<String> for Cell {
    fn from(text: String) -> Self {
        Cell::new(text)
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Mark {
    pub(crate) text: String,
    pub(crate) paint: Paint,
}

impl Mark {
    pub fn new(text: impl Into<String>) -> Self {
        Mark {
            text: text.into(),
            paint: Paint::Ink,
        }
    }

    pub fn faint(mut self) -> Self {
        self.paint = Paint::Faint;
        self
    }

    pub fn style(mut self, style: Style) -> Self {
        self.paint = Paint::Own(style);
        self
    }

    pub fn text(&self) -> &str {
        &self.text
    }

    pub fn set_text(&mut self, text: &str) {
        self.text.clear();
        self.text.push_str(text);
    }

    pub fn set_style(&mut self, style: Style) {
        self.paint = Paint::Own(style);
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum Kind {
    #[default]
    Item,
    Heading,
    Blank,
}

#[derive(Debug, Clone, PartialEq, Eq, Default)]
#[non_exhaustive]
pub struct Row {
    pub(crate) mark: Option<Mark>,
    pub(crate) cells: Vec<Cell>,
    pub(crate) kind: Kind,
}

impl Row {
    pub fn new<I, C>(cells: I) -> Self
    where
        I: IntoIterator<Item = C>,
        C: Into<Cell>,
    {
        Row {
            mark: None,
            cells: cells.into_iter().map(Into::into).collect(),
            kind: Kind::Item,
        }
    }

    pub fn heading<I, P>(parts: I) -> Self
    where
        I: IntoIterator<Item = P>,
        P: Into<Part>,
    {
        Row {
            mark: None,
            cells: vec![Cell::parts(parts)],
            kind: Kind::Heading,
        }
    }

    pub fn blank() -> Self {
        Row {
            mark: None,
            cells: Vec::new(),
            kind: Kind::Blank,
        }
    }

    pub fn mark(mut self, mark: Mark) -> Self {
        self.mark = Some(mark);
        self
    }

    pub fn cells(&self) -> &[Cell] {
        &self.cells
    }

    pub fn cells_mut(&mut self) -> &mut [Cell] {
        &mut self.cells
    }

    pub fn leading(&self) -> Option<&Mark> {
        self.mark.as_ref()
    }

    pub fn leading_mut(&mut self) -> Option<&mut Mark> {
        self.mark.as_mut()
    }

    pub fn selectable(&self) -> bool {
        self.kind == Kind::Item
    }

    pub(crate) fn mark_width(&self) -> u16 {
        match (&self.mark, self.kind) {
            (Some(mark), Kind::Item) => crate::list::text::cells(&mark.text),
            _ => 0,
        }
    }
}
