use ratatui::layout::Rect;

pub const TOP: u16 = 1;
pub const SIDE: u16 = 1;
pub const PAD: u16 = 2;
pub const GAP: u16 = 1;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Edge {
    Start,
    Third,
    Middle,
    End,
}

impl Edge {
    const fn at(self, start: u16, room: u16, size: u16) -> u16 {
        let spare = room.saturating_sub(size);
        start.saturating_add(match self {
            Edge::Start => 0,
            Edge::Third => spare / 3,
            Edge::Middle => spare / 2,
            Edge::End => spare,
        })
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Spot {
    pub x: Edge,
    pub y: Edge,
}

impl Spot {
    pub const TOP_LEFT: Spot = Spot::new(Edge::Start, Edge::Start);
    pub const TOP: Spot = Spot::new(Edge::Middle, Edge::Start);
    pub const TOP_RIGHT: Spot = Spot::new(Edge::End, Edge::Start);
    pub const LEFT: Spot = Spot::new(Edge::Start, Edge::Middle);
    pub const MIDDLE: Spot = Spot::new(Edge::Middle, Edge::Middle);
    pub const RIGHT: Spot = Spot::new(Edge::End, Edge::Middle);
    pub const BOTTOM_LEFT: Spot = Spot::new(Edge::Start, Edge::End);
    pub const BOTTOM: Spot = Spot::new(Edge::Middle, Edge::End);
    pub const BOTTOM_RIGHT: Spot = Spot::new(Edge::End, Edge::End);
    pub const THIRD: Spot = Spot::new(Edge::Middle, Edge::Third);

    pub const fn new(x: Edge, y: Edge) -> Self {
        Spot { x, y }
    }

    pub fn place(self, area: Rect, width: u16, height: u16) -> Rect {
        let width = width.min(area.width);
        let height = height.min(area.height);
        Rect::new(
            self.x.at(area.x, area.width, width),
            self.y.at(area.y, area.height, height),
            width,
            height,
        )
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Layout {
    pub top: u16,
    pub bottom: u16,
    pub side: u16,
    pub pad: u16,
    pub head_gap: u16,
    pub gap: u16,
    pub small: Spot,
}

impl Layout {
    pub const fn new() -> Self {
        Layout {
            top: TOP,
            bottom: 0,
            side: SIDE,
            pad: PAD,
            head_gap: 0,
            gap: GAP,
            small: Spot::THIRD,
        }
    }

    pub const fn top(mut self, rows: u16) -> Self {
        self.top = rows;
        self
    }

    pub const fn bottom(mut self, rows: u16) -> Self {
        self.bottom = rows;
        self
    }

    pub const fn side(mut self, cells: u16) -> Self {
        self.side = cells;
        self
    }

    pub const fn pad(mut self, cells: u16) -> Self {
        self.pad = cells;
        self
    }

    pub const fn head_gap(mut self, rows: u16) -> Self {
        self.head_gap = rows;
        self
    }

    pub const fn gap(mut self, rows: u16) -> Self {
        self.gap = rows;
        self
    }

    pub const fn small(mut self, spot: Spot) -> Self {
        self.small = spot;
        self
    }
}

impl Default for Layout {
    fn default() -> Self {
        Layout::new()
    }
}
