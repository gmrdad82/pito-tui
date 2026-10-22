use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::ratatui::{Frame, buffer::Buffer, layout::Rect, style::Color};
use pito_tui::{Cx, Edge, Palette, Screen, Spot, Toast, Tui, dump};

const TOAST: u16 = 3;

#[derive(Default)]
struct Host {
    queue: Vec<Toast>,
}

impl Screen<u32> for Host {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        if key.code != KeyCode::Char('x') {
            return;
        }
        for toast in self.queue.drain(..) {
            cx.toast(toast);
        }
    }
}

fn tui(queue: Vec<Toast>, small: bool) -> Tui<u32> {
    let mut tui = Tui::new("probe", "1.0.0", Color::Blue).screen("One", Host { queue });
    if small {
        tui = tui.min_size(1, 1);
    }
    tui.start();
    tui.key(KeyEvent::new(KeyCode::Char('x'), KeyModifiers::NONE));
    tui
}

fn label(n: usize) -> String {
    format!("tt{n:02}")
}

fn many(spot: Spot, count: usize, first: Duration) -> Vec<Toast> {
    (0..count)
        .map(|n| {
            let toast = Toast::new(label(n)).at(spot);
            if n == 0 { toast.lasting(first) } else { toast }
        })
        .collect()
}

fn found(buffer: &Buffer, needle: &str) -> Option<(u16, u16)> {
    let area = buffer.area;
    (area.top()..area.bottom()).find_map(|y| {
        let line: String = (area.left()..area.right())
            .map(|x| buffer[(x, y)].symbol().to_string())
            .collect();
        let at = line.find(needle)?;
        let x = u16::try_from(line[..at].chars().count()).ok()?;
        Some((x, y))
    })
}

fn whole(buffer: &Buffer, (x, y): (u16, u16), text: &str) -> bool {
    let wide = u16::try_from(text.len()).unwrap_or(u16::MAX);
    if x < 2 || y < 1 || y + 1 >= buffer.area.bottom() {
        return false;
    }
    let (left, right) = (x - 2, x + wide + 1);
    if right >= buffer.area.right() {
        return false;
    }
    let at = |cx: u16, cy: u16| buffer[(cx, cy)].symbol();
    at(left, y - 1) == "╭"
        && at(right, y - 1) == "╮"
        && at(left, y) == "│"
        && at(right, y) == "│"
        && at(left, y + 1) == "╰"
        && at(right, y + 1) == "╯"
        && (x..x + wide).all(|cx| at(cx, y - 1) == "─" && at(cx, y + 1) == "─")
}

struct Area {
    top: u16,
    bottom: u16,
}

fn area(width: u16, height: u16) -> Area {
    let row = |spot: Spot| {
        let mut tui = tui(vec![Toast::new(label(0)).at(spot)], false);
        let buffer = tui.frame(width, height);
        let Some((_, y)) = found(&buffer, &label(0)) else {
            panic!("a lone toast shows:\n{}", dump::text(&buffer));
        };
        y
    };
    Area {
        top: row(Spot::TOP) - 1,
        bottom: row(Spot::BOTTOM) + 2,
    }
}

fn start(spot: Spot, width: u16, height: u16) -> u16 {
    let mut tui = tui(vec![Toast::new(label(0)).at(spot)], false);
    let buffer = tui.frame(width, height);
    found(&buffer, &label(0)).map_or_else(
        || panic!("a lone toast shows:\n{}", dump::text(&buffer)),
        |(_, y)| y - 1,
    )
}

fn fits(spot: Spot, width: u16, height: u16) -> usize {
    let area = area(width, height);
    let first = start(spot, width, height);
    let rows = match spot.y {
        Edge::End => first + TOAST - area.top,
        _ => area.bottom - first,
    };
    usize::from(rows / TOAST)
}

fn check(buffer: &Buffer, spot: Spot, from: usize, count: usize, fit: usize, area: &Area) {
    let text = dump::text(buffer);
    let shown: Vec<(usize, (u16, u16))> = (from..count)
        .filter_map(|n| found(buffer, &label(n)).map(|at| (n, at)))
        .collect();
    let expected: Vec<usize> = (from..count.min(from + fit)).collect();
    let names: Vec<usize> = shown.iter().map(|(n, _)| *n).collect();
    assert_eq!(names, expected, "the oldest that fit show:\n{text}");
    for pair in shown.windows(2) {
        let ((_, (_, a)), (_, (_, b))) = (pair[0], pair[1]);
        match spot.y {
            Edge::End => assert_eq!(b + TOAST, a, "each newer one sits above:\n{text}"),
            _ => assert_eq!(a + TOAST, b, "each newer one sits below:\n{text}"),
        }
    }
    for (n, at) in &shown {
        assert!(whole(buffer, *at, &label(*n)), "{} is whole:\n{text}", label(*n));
        assert!(at.1 > area.top, "{} stays off the top rows:\n{text}", label(*n));
        assert!(at.1 + 2 <= area.bottom, "{} stays inside:\n{text}", label(*n));
    }
}

fn overflow(spot: Spot, width: u16, height: u16, count: usize) {
    let fit = fits(spot, width, height);
    assert!(fit >= 1 && fit < count, "{spot:?} at {width}x{height} fits {fit}");
    let area = area(width, height);
    let mut tui = tui(many(spot, count, Duration::from_secs(1)), false);
    let buffer = tui.frame(width, height);
    check(&buffer, spot, 0, count, fit, &area);
    let edge = found(&buffer, &label(0)).map(|(_, y)| y);
    tui.advance(Duration::from_secs(1));
    let buffer = tui.frame(width, height);
    assert!(found(&buffer, &label(0)).is_none(), "the oldest has left");
    check(&buffer, spot, 1, count, fit, &area);
    assert_eq!(
        found(&buffer, &label(1)).map(|(_, y)| y),
        edge,
        "the next one takes the edge"
    );
    assert!(
        found(&buffer, &label(fit)).is_some(),
        "the first one left out shows once there is room:\n{}",
        dump::text(&buffer)
    );
}

#[test]
fn bottom_toasts_past_the_room_are_left_out_on_a_short_terminal() {
    for spot in [Spot::BOTTOM, Spot::BOTTOM_LEFT, Spot::BOTTOM_RIGHT] {
        overflow(spot, 60, 12, 8);
    }
}

#[test]
fn bottom_toasts_past_the_room_are_left_out_on_a_tall_terminal() {
    for spot in [Spot::BOTTOM, Spot::BOTTOM_LEFT, Spot::BOTTOM_RIGHT] {
        overflow(spot, 120, 40, 20);
    }
}

#[test]
fn top_and_middle_toasts_past_the_room_drop_the_newest() {
    for spot in [Spot::TOP, Spot::TOP_RIGHT, Spot::MIDDLE, Spot::THIRD] {
        overflow(spot, 60, 12, 8);
        overflow(spot, 120, 40, 20);
    }
}

#[test]
fn a_full_bottom_stack_never_draws_over_the_top_rows() {
    let mut bare = tui(Vec::new(), false);
    let plain = bare.frame(60, 12);
    let area = area(60, 12);
    let mut tui = tui(many(Spot::BOTTOM, 12, Duration::from_secs(3)), false);
    let buffer = tui.frame(60, 12);
    for y in 0..=area.top {
        for x in 0..60 {
            assert_eq!(
                buffer[(x, y)].symbol(),
                plain[(x, y)].symbol(),
                "row {y} is untouched:\n{}",
                dump::text(&buffer)
            );
        }
    }
}

#[test]
fn a_toast_in_an_area_shorter_than_itself_draws_nothing() {
    for height in 1..=12 {
        for spot in [Spot::TOP, Spot::MIDDLE, Spot::BOTTOM, Spot::BOTTOM_RIGHT] {
            let mut bare = tui(Vec::new(), true);
            let plain = bare.frame(60, height);
            let mut tui = tui(vec![Toast::new(label(0)).at(spot)], true);
            let buffer = tui.frame(60, height);
            match found(&buffer, &label(0)) {
                Some(at) => assert!(
                    whole(&buffer, at, &label(0)),
                    "{spot:?} at height {height} is whole:\n{}",
                    dump::text(&buffer)
                ),
                None => assert_eq!(
                    dump::text(&buffer),
                    dump::text(&plain),
                    "{spot:?} at height {height} leaves no trace"
                ),
            }
        }
    }
}
