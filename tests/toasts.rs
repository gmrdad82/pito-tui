use std::cell::Cell;
use std::rc::Rc;
use std::sync::OnceLock;
use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::ratatui::{Frame, buffer::Buffer, layout::Rect, style::Color};
use pito_tui::{Cx, Edge, Palette, Screen, Spot, Toast, Tui, dump};

struct Host {
    queue: Vec<Toast>,
    seen: Rc<Cell<Rect>>,
}

impl Screen<u32> for Host {
    fn draw(&mut self, _frame: &mut Frame, area: Rect, _palette: &Palette) {
        self.seen.set(area);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        if key.code != KeyCode::Char('x') {
            return;
        }
        for toast in self.queue.drain(..) {
            cx.toast(toast);
        }
    }
}

fn probe(queue: Vec<Toast>, small: bool) -> (Tui<u32>, Rc<Cell<Rect>>) {
    let seen = Rc::new(Cell::new(Rect::default()));
    let host = Host {
        queue,
        seen: Rc::clone(&seen),
    };
    let mut tui = Tui::new("probe", "1.0.0", Color::Blue).screen("One", host);
    if small {
        tui = tui.min_size(1, 1);
    }
    tui.start();
    tui.key(KeyEvent::new(KeyCode::Char('x'), KeyModifiers::NONE));
    (tui, seen)
}

fn tui(queue: Vec<Toast>, small: bool) -> Tui<u32> {
    probe(queue, small).0
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

fn boxed(buffer: &Buffer, (x, y): (u16, u16), text: &str) -> Option<(u16, u16)> {
    let wide = u16::try_from(text.len()).ok()?;
    let left = x.checked_sub(2)?;
    let right = x + wide + 1;
    if right >= buffer.area.right() {
        return None;
    }
    let at = |cx: u16, cy: u16| buffer[(cx, cy)].symbol();
    let rule = |cy: u16| (x..x + wide).all(|cx| at(cx, cy) == "─");
    let top = (0..y).rev().find(|&r| at(left, r) != "│")?;
    let bottom = (y + 1..buffer.area.bottom()).find(|&r| at(left, r) != "│")?;
    let sides = (top + 1..bottom).all(|r| at(left, r) == "│" && at(right, r) == "│");
    let whole = sides
        && at(left, top) == "╭"
        && at(right, top) == "╮"
        && at(left, bottom) == "╰"
        && at(right, bottom) == "╯"
        && rule(top)
        && rule(bottom);
    whole.then_some((top, bottom))
}

fn lone(spot: Spot, width: u16, height: u16) -> (u16, u16) {
    let mut tui = tui(vec![Toast::new(label(0)).at(spot)], false);
    let buffer = tui.frame(width, height);
    found(&buffer, &label(0))
        .and_then(|at| boxed(&buffer, at, &label(0)))
        .unwrap_or_else(|| panic!("a lone toast shows whole:\n{}", dump::text(&buffer)))
}

fn tall() -> u16 {
    static TALL: OnceLock<u16> = OnceLock::new();
    *TALL.get_or_init(|| {
        let (top, bottom) = lone(Spot::TOP, 120, 40);
        bottom - top + 1
    })
}

struct Area {
    top: u16,
    bottom: u16,
}

fn area(width: u16, height: u16) -> Area {
    let (mut tui, seen) = probe(Vec::new(), false);
    tui.frame(width, height);
    let content = seen.get();
    assert!(!content.is_empty(), "{width}x{height} has a content area");
    Area {
        top: content.top(),
        bottom: content.bottom(),
    }
}

fn fits(spot: Spot, width: u16, height: u16) -> usize {
    let area = area(width, height);
    let tall = tall();
    let (first, _) = lone(spot, width, height);
    let rows = match spot.y {
        Edge::End => first + tall - area.top,
        _ => area.bottom - first,
    };
    usize::from(rows / tall)
}

fn check(buffer: &Buffer, spot: Spot, from: usize, count: usize, fit: usize, area: &Area) {
    let text = dump::text(buffer);
    let tall = tall();
    let shown: Vec<(usize, (u16, u16))> = (from..count)
        .filter_map(|n| found(buffer, &label(n)).map(|at| (n, at)))
        .collect();
    let expected: Vec<usize> = (from..count.min(from + fit)).collect();
    let names: Vec<usize> = shown.iter().map(|(n, _)| *n).collect();
    assert_eq!(names, expected, "the oldest that fit show:\n{text}");
    for pair in shown.windows(2) {
        let ((_, (_, a)), (_, (_, b))) = (pair[0], pair[1]);
        match spot.y {
            Edge::End => assert_eq!(b + tall, a, "each newer one sits above:\n{text}"),
            _ => assert_eq!(a + tall, b, "each newer one sits below:\n{text}"),
        }
    }
    for (n, at) in &shown {
        let Some((top, bottom)) = boxed(buffer, *at, &label(*n)) else {
            panic!("{} is whole:\n{text}", label(*n));
        };
        assert_eq!(
            bottom - top + 1,
            tall,
            "{} is one toast tall:\n{text}",
            label(*n)
        );
        assert!(
            top >= area.top,
            "{} stays off the top rows:\n{text}",
            label(*n)
        );
        assert!(bottom < area.bottom, "{} stays inside:\n{text}", label(*n));
    }
}

fn overflow(spot: Spot, width: u16, height: u16, count: usize) {
    let fit = fits(spot, width, height);
    assert!(
        fit >= 1 && fit < count,
        "{spot:?} at {width}x{height} fits {fit}"
    );
    let area = area(width, height);
    let mut tui = tui(many(spot, count, Duration::from_secs(1)), false);
    let buffer = tui.frame(width, height);
    check(&buffer, spot, 0, count, fit, &area);
    let edge = found(&buffer, &label(0)).map(|(_, y)| y);
    assert_eq!(
        edge,
        Some(lone(spot, width, height).0 + 1),
        "the oldest sits where a lone toast does:\n{}",
        dump::text(&buffer)
    );
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
    for spot in [
        Spot::TOP,
        Spot::TOP_LEFT,
        Spot::TOP_RIGHT,
        Spot::LEFT,
        Spot::MIDDLE,
        Spot::RIGHT,
        Spot::THIRD,
    ] {
        overflow(spot, 60, 12, 8);
        overflow(spot, 120, 40, 20);
    }
}

#[test]
fn a_full_bottom_stack_never_draws_above_its_top() {
    for (width, height, count) in [(60, 12, 12), (60, 13, 12), (60, 14, 12), (120, 40, 30)] {
        let mut bare = tui(Vec::new(), false);
        let plain = bare.frame(width, height);
        let area = area(width, height);
        let mut tui = tui(many(Spot::BOTTOM, count, Duration::from_secs(3)), false);
        let buffer = tui.frame(width, height);
        let text = dump::text(&buffer);
        let stack = (0..count)
            .filter_map(|n| {
                let at = found(&buffer, &label(n))?;
                boxed(&buffer, at, &label(n)).map(|(top, _)| top)
            })
            .min()
            .unwrap_or_else(|| panic!("the stack shows at {width}x{height}:\n{text}"));
        assert!(stack >= area.top, "the stack stays inside:\n{text}");
        let fit = fits(Spot::BOTTOM, width, height);
        check(&buffer, Spot::BOTTOM, 0, count, fit, &area);
        for y in 0..stack {
            for x in 0..width {
                assert_eq!(
                    buffer[(x, y)].symbol(),
                    plain[(x, y)].symbol(),
                    "row {y} above the stack is untouched at {width}x{height}:\n{text}"
                );
            }
        }
    }
}

#[test]
fn a_toast_in_an_area_shorter_than_itself_draws_nothing() {
    let tall = tall();
    let mut short = 0;
    for height in 1..=12 {
        for spot in [Spot::TOP, Spot::MIDDLE, Spot::BOTTOM, Spot::BOTTOM_RIGHT] {
            let (mut bare, seen) = probe(Vec::new(), true);
            let plain = bare.frame(60, height);
            let content = seen.get();
            let mut tui = tui(vec![Toast::new(label(0)).at(spot)], true);
            let buffer = tui.frame(60, height);
            if content.is_empty() || content.height < tall {
                if !content.is_empty() {
                    short += 1;
                }
                assert_eq!(
                    dump::text(&buffer),
                    dump::text(&plain),
                    "{spot:?} in a {}-row area leaves no trace",
                    content.height
                );
                continue;
            }
            let at = found(&buffer, &label(0)).unwrap_or_else(|| {
                panic!(
                    "{spot:?} in a {}-row area shows:\n{}",
                    content.height,
                    dump::text(&buffer)
                )
            });
            assert!(
                boxed(&buffer, at, &label(0)).is_some(),
                "{spot:?} at height {height} is whole:\n{}",
                dump::text(&buffer)
            );
        }
    }
    assert!(
        short > 0,
        "some height leaves a content area shorter than a toast"
    );
}
