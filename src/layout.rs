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

#[cfg(test)]
mod tests {
    use super::*;

    const EDGES: [Edge; 4] = [Edge::Start, Edge::Third, Edge::Middle, Edge::End];

    const SPOTS: [Spot; 10] = [
        Spot::TOP_LEFT,
        Spot::TOP,
        Spot::TOP_RIGHT,
        Spot::LEFT,
        Spot::MIDDLE,
        Spot::RIGHT,
        Spot::BOTTOM_LEFT,
        Spot::BOTTOM,
        Spot::BOTTOM_RIGHT,
        Spot::THIRD,
    ];

    fn inside(placed: Rect, area: Rect) -> bool {
        placed.x >= area.x
            && placed.y >= area.y
            && placed.right() <= area.right()
            && placed.bottom() <= area.bottom()
    }

    #[test]
    fn named_spots_put_a_box_at_their_corner_edge_or_centre() {
        let area = Rect::new(10, 5, 20, 10);
        let at = |spot: Spot| spot.place(area, 4, 2);
        assert_eq!(at(Spot::TOP_LEFT), Rect::new(10, 5, 4, 2));
        assert_eq!(at(Spot::TOP), Rect::new(18, 5, 4, 2));
        assert_eq!(at(Spot::TOP_RIGHT), Rect::new(26, 5, 4, 2));
        assert_eq!(at(Spot::LEFT), Rect::new(10, 9, 4, 2));
        assert_eq!(at(Spot::MIDDLE), Rect::new(18, 9, 4, 2));
        assert_eq!(at(Spot::RIGHT), Rect::new(26, 9, 4, 2));
        assert_eq!(at(Spot::BOTTOM_LEFT), Rect::new(10, 13, 4, 2));
        assert_eq!(at(Spot::BOTTOM), Rect::new(18, 13, 4, 2));
        assert_eq!(at(Spot::BOTTOM_RIGHT), Rect::new(26, 13, 4, 2));
        assert_eq!(at(Spot::THIRD), Rect::new(18, 7, 4, 2));
    }

    #[test]
    fn a_third_sits_a_third_of_the_spare_room_in() {
        let area = Rect::new(0, 0, 30, 30);
        let placed = Spot::new(Edge::Third, Edge::Third).place(area, 6, 3);
        assert_eq!(placed, Rect::new(8, 9, 6, 3));
    }

    #[test]
    fn odd_spare_room_rounds_towards_the_start() {
        let area = Rect::new(0, 0, 5, 5);
        let x = |edge: Edge| Spot::new(edge, Edge::Start).place(area, 2, 1).x;
        assert_eq!(x(Edge::Start), 0);
        assert_eq!(x(Edge::Third), 1);
        assert_eq!(x(Edge::Middle), 1);
        assert_eq!(x(Edge::End), 3);
    }

    #[test]
    fn a_box_that_fills_the_area_lands_on_it_at_every_spot() {
        let area = Rect::new(3, 4, 12, 6);
        for spot in SPOTS {
            assert_eq!(spot.place(area, 12, 6), area);
        }
    }

    #[test]
    fn a_box_larger_than_the_area_shrinks_to_the_area() {
        let area = Rect::new(3, 4, 12, 6);
        for spot in SPOTS {
            assert_eq!(spot.place(area, 50, 50), area);
            assert_eq!(spot.place(area, u16::MAX, u16::MAX), area);
        }
        let wide = Spot::BOTTOM_RIGHT.place(area, 40, 2);
        assert_eq!(wide, Rect::new(3, 8, 12, 2));
    }

    #[test]
    fn zero_areas_and_zero_boxes_never_panic() {
        let empty = Rect::new(3, 4, 0, 0);
        for spot in SPOTS {
            assert_eq!(spot.place(empty, 5, 5), Rect::new(3, 4, 0, 0));
            assert_eq!(spot.place(empty, 0, 0), Rect::new(3, 4, 0, 0));
        }
        let area = Rect::new(10, 10, 20, 10);
        assert_eq!(Spot::MIDDLE.place(area, 0, 0), Rect::new(20, 15, 0, 0));
        assert_eq!(Spot::BOTTOM_RIGHT.place(area, 0, 0), Rect::new(30, 20, 0, 0));
        let flat = Rect::new(0, 0, 8, 0);
        assert_eq!(Spot::MIDDLE.place(flat, 2, 2), Rect::new(3, 0, 2, 0));
        assert_eq!(Spot::MIDDLE.place(Rect::ZERO, 1, 1), Rect::ZERO);
    }

    #[test]
    fn every_placement_lies_inside_its_area() {
        let areas = [
            Rect::new(0, 0, 0, 0),
            Rect::new(0, 0, 1, 1),
            Rect::new(5, 7, 1, 9),
            Rect::new(2, 3, 80, 24),
            Rect::new(100, 50, 7, 3),
            Rect {
                x: u16::MAX - 10,
                y: u16::MAX - 10,
                width: 10,
                height: 10,
            },
        ];
        let sizes = [0, 1, 2, 3, 7, 10, 24, 81, u16::MAX];
        for area in areas {
            for x in EDGES {
                for y in EDGES {
                    for &width in &sizes {
                        for &height in &sizes {
                            let placed = Spot::new(x, y).place(area, width, height);
                            assert!(inside(placed, area), "{placed:?} in {area:?}");
                            assert_eq!(placed.width, width.min(area.width));
                            assert_eq!(placed.height, height.min(area.height));
                        }
                    }
                }
            }
        }
    }

    #[test]
    fn edges_order_from_start_to_end() {
        let area = Rect::new(0, 0, 61, 61);
        for size in [0, 1, 10, 60] {
            let at = |edge: Edge| Spot::new(edge, edge).place(area, size, size);
            let starts: Vec<u16> = EDGES.iter().map(|&edge| at(edge).x).collect();
            assert!(starts.windows(2).all(|pair| pair[0] <= pair[1]), "{starts:?}");
            assert_eq!(at(Edge::End).right(), area.right());
        }
    }

    #[test]
    fn the_default_layout_is_the_shells_standard_spacing() {
        let layout = Layout::default();
        assert_eq!(layout, Layout::new());
        assert_eq!(layout.top, TOP);
        assert_eq!(layout.bottom, 0);
        assert_eq!(layout.side, SIDE);
        assert_eq!(layout.pad, PAD);
        assert_eq!(layout.head_gap, 0);
        assert_eq!(layout.gap, GAP);
        assert_eq!(layout.small, Spot::THIRD);
        assert_eq!((TOP, SIDE, PAD, GAP), (1, 1, 2, 1));
    }

    #[test]
    fn each_layout_setter_changes_its_own_field_alone() {
        let base = Layout::new();
        assert_eq!(base.top(4), Layout { top: 4, ..base });
        assert_eq!(base.bottom(2), Layout { bottom: 2, ..base });
        assert_eq!(base.side(0), Layout { side: 0, ..base });
        assert_eq!(base.pad(6), Layout { pad: 6, ..base });
        assert_eq!(base.head_gap(3), Layout { head_gap: 3, ..base });
        assert_eq!(base.gap(0), Layout { gap: 0, ..base });
        assert_eq!(
            base.small(Spot::BOTTOM_RIGHT),
            Layout {
                small: Spot::BOTTOM_RIGHT,
                ..base
            }
        );
        let chained = base.top(0).bottom(1).side(2).pad(3).head_gap(4).gap(5);
        assert_eq!(
            (chained.top, chained.bottom, chained.side, chained.pad, chained.head_gap, chained.gap),
            (0, 1, 2, 3, 4, 5)
        );
    }
}
