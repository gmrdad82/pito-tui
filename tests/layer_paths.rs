use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::Key;
use pito_tui::layer::Event;
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Style},
    text::Line,
};
use pito_tui::{
    Choice, Cx, Edge, Field, Modal, Palette, Screen, Spot, Takeover, Toast, Tone, Tui, dump,
};

const WIDTH: u16 = 80;
const HEIGHT: u16 = 24;

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

enum Act {
    Toast(Toast),
    Open(Modal),
    Alert(u64, &'static str, &'static str),
    Takeover(Takeover),
}

#[derive(Default)]
struct Host {
    queue: Vec<Act>,
    heard: Vec<(u64, Event)>,
    keys: Vec<KeyCode>,
}

impl Screen<u32> for Host {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        self.keys.push(key.code);
        if key.code != KeyCode::Char('x') {
            return;
        }
        for act in self.queue.drain(..) {
            match act {
                Act::Toast(toast) => cx.toast(toast),
                Act::Open(modal) => cx.open(modal),
                Act::Alert(id, title, line) => cx.alert(id, title, [Line::from(line)]),
                Act::Takeover(takeover) => cx.takeover(takeover),
            }
        }
    }

    fn layer(&mut self, id: u64, event: Event, _cx: &mut Cx<'_, u32>) {
        self.heard.push((id, event));
    }
}

fn run(queue: Vec<Act>) -> Tui<u32> {
    let host = Host {
        queue,
        ..Host::default()
    };
    let mut tui = Tui::new("probe", "1.0.0", Color::Blue).screen("One", host);
    tui.start();
    tui.key(press(KeyCode::Char('x')));
    tui
}

fn more(tui: &mut Tui<u32>, queue: Vec<Act>) {
    if let Some(host) = tui.find_mut::<Host>() {
        host.queue = queue;
    }
    tui.key(press(KeyCode::Char('x')));
}

fn heard(tui: &Tui<u32>) -> Vec<(u64, Event)> {
    tui.find::<Host>().map(|host| host.heard.clone()).unwrap_or_default()
}

fn frame(tui: &mut Tui<u32>) -> Buffer {
    tui.frame(WIDTH, HEIGHT)
}

fn shown(tui: &mut Tui<u32>) -> String {
    dump::text(&frame(tui))
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

fn palette() -> Palette {
    Palette::new(Color::Blue)
}

#[test]
fn a_takeover_draws_its_title_and_label_and_leaves_only_on_its_cancel_keys() {
    let mut tui = run(vec![Act::Takeover(
        Takeover::new(5, "Moving house").label("hang on"),
    )]);
    let text = shown(&mut tui);
    assert!(text.contains("Moving house") && text.contains("hang on"), "{text}");
    assert!(!text.contains("One"), "a takeover hides the tabs");
    tui.key(press(KeyCode::Esc));
    tui.key(press(KeyCode::Char('y')));
    assert!(shown(&mut tui).contains("Moving house"), "no cancel keys, no way out");
    assert!(heard(&tui).is_empty());
    assert_eq!(
        tui.find::<Host>().map(|host| host.keys.len()),
        Some(1),
        "keys under a takeover never reach the screen"
    );
    let mut tui = run(vec![Act::Takeover(
        Takeover::new(6, "Moving").cancel(&[Key::Char('n')]),
    )]);
    tui.key(press(KeyCode::Esc));
    assert!(shown(&mut tui).contains("Moving"));
    tui.key(press(KeyCode::Char('n')));
    assert_eq!(heard(&tui), [(6, Event::Closed)]);
    let text = shown(&mut tui);
    assert!(!text.contains("Moving") && text.contains("One"));
}

#[test]
fn a_takeover_title_keeps_the_styles_of_its_spans() {
    let red = Style::new().fg(Color::Red);
    let mut tui = run(vec![Act::Takeover(
        Takeover::new(5, "ignored").title_spans([("Mov", red), ("ing", Style::new())]),
    )]);
    let buffer = frame(&mut tui);
    let text = dump::text(&buffer);
    assert!(text.contains("Moving") && !text.contains("ignored"), "{text}");
    let at = found(&buffer, "Moving");
    assert_eq!(at.map(|at| buffer[at].fg), Some(Color::Red));
    let after = at.map(|(x, y)| buffer[(x + 3, y)].fg);
    assert_ne!(after, Some(Color::Red), "the plain span is not red");
}

#[test]
fn a_takeover_covers_an_open_modal_and_its_toasts_until_it_ends() {
    let mut tui = run(vec![
        Act::Open(Modal::new(1, "Rename").field(Field::new("Name"))),
        Act::Toast(Toast::new("Saved it")),
        Act::Takeover(Takeover::new(2, "Busy").cancel(&[Key::Esc])),
    ]);
    let text = shown(&mut tui);
    assert!(text.contains("Busy") && !text.contains("Rename") && !text.contains("Saved it"));
    tui.key(press(KeyCode::Char('a')));
    tui.paste("bc");
    assert!(heard(&tui).is_empty(), "the modal under a takeover hears nothing");
    tui.key(press(KeyCode::Esc));
    assert_eq!(heard(&tui), [(2, Event::Closed)]);
    let text = shown(&mut tui);
    assert!(text.contains("Rename") && text.contains("Saved it"), "{text}");
    assert!(!text.contains("Nameabc"));
}

#[test]
fn a_modal_in_the_shell_takes_paste_keys_and_closes_with_its_event() {
    let mut tui = run(vec![Act::Open(
        Modal::new(3, "Go to")
            .field(Field::new("Find"))
            .list([
                Choice::new("Lock"),
                Choice::spans([
                    ("Ho".to_string(), Style::new().fg(Color::Green)),
                    ("me".to_string(), Style::new()),
                ]),
            ])
            .filter(0, 1),
    )]);
    tui.paste("lo");
    assert_eq!(heard(&tui), [(3, Event::Edited { part: 0 })]);
    let text = shown(&mut tui);
    assert!(text.contains("lo") && text.contains("Lock") && !text.contains("Home"));
    tui.key(press(KeyCode::Backspace));
    tui.key(press(KeyCode::Backspace));
    let buffer = frame(&mut tui);
    let at = found(&buffer, "Home");
    assert_eq!(at.map(|at| buffer[at].fg), Some(Color::Green), "a choice keeps its span style");
    tui.key(press(KeyCode::Enter));
    assert_eq!(
        heard(&tui).last(),
        Some(&(3, Event::Chosen { part: 1, choice: 0 }))
    );
    tui.key(press(KeyCode::Esc));
    assert_eq!(heard(&tui).last(), Some(&(3, Event::Closed)));
    assert!(!shown(&mut tui).contains("Go to"));
}

#[test]
fn a_skipped_choice_is_never_chosen_and_a_confirm_choice_asks_before_it_is() {
    let mut tui = run(vec![Act::Open(Modal::new(4, "Act").list([
        Choice::new("Danger zone").skip(),
        Choice::new("Erase").confirm("enter again erases"),
    ]))]);
    tui.key(press(KeyCode::Up));
    tui.key(press(KeyCode::Enter));
    assert!(heard(&tui).is_empty(), "the heading cannot be chosen; erase only arms");
    let buffer = frame(&mut tui);
    let at = found(&buffer, "enter again erases");
    assert_eq!(
        at.map(|at| buffer[at].fg),
        Some(palette().warn.fg.unwrap_or_default())
    );
    tui.key(press(KeyCode::Enter));
    assert_eq!(heard(&tui), [(4, Event::Chosen { part: 0, choice: 1 })]);
}

#[test]
fn an_alert_opened_through_cx_closes_on_q() {
    let mut tui = run(vec![Act::Alert(8, "Heads up", "disk full")]);
    let text = shown(&mut tui);
    assert!(text.contains("Heads up") && text.contains("disk full"));
    tui.key(press(KeyCode::Char('q')));
    assert_eq!(heard(&tui), [(8, Event::Closed)]);
    assert!(!shown(&mut tui).contains("disk full"));
}

fn spot_name(spot: Spot) -> &'static str {
    match (spot.x, spot.y) {
        (Edge::Start, Edge::Start) => "nw",
        (Edge::Middle, Edge::Start) => "n",
        (Edge::End, Edge::Start) => "ne",
        (Edge::Start, Edge::Middle) => "w",
        (Edge::Middle, Edge::Middle) => "c",
        (Edge::End, Edge::Middle) => "e",
        (Edge::Start, Edge::End) => "sw",
        (Edge::Middle, Edge::End) => "s",
        (Edge::End, Edge::End) => "se",
        (Edge::Middle, Edge::Third) => "t",
        _ => "?",
    }
}

#[test]
fn a_toast_is_drawn_at_each_spot() {
    let spots = [
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
    for spot in spots {
        let name = spot_name(spot);
        let text = format!("toast {name}");
        let mut tui = run(vec![Act::Toast(Toast::new(text.clone()).at(spot))]);
        let buffer = frame(&mut tui);
        let Some((x, y)) = found(&buffer, &text) else {
            panic!("{name} is drawn:\n{}", dump::text(&buffer));
        };
        let middle = x + 4;
        match spot.x {
            Edge::Start => assert!(middle < WIDTH / 3, "{name} at {x}"),
            Edge::Middle => assert!((WIDTH / 3..2 * WIDTH / 3).contains(&middle), "{name} at {x}"),
            _ => assert!(middle > 2 * WIDTH / 3, "{name} at {x}"),
        }
        match spot.y {
            Edge::Start => assert!(y < HEIGHT / 3, "{name} at {y}"),
            Edge::Third => assert!((HEIGHT / 4..HEIGHT / 2).contains(&y), "{name} at {y}"),
            Edge::Middle => assert!((HEIGHT / 3..2 * HEIGHT / 3).contains(&y), "{name} at {y}"),
            _ => assert!(y > 2 * HEIGHT / 3, "{name} at {y}"),
        }
        assert_eq!(buffer[(x - 2, y)].symbol(), "│", "{name} is boxed");
    }
}

#[test]
fn toasts_at_one_spot_stack_inward_and_close_ranks_when_one_leaves() {
    let mut tui = run(vec![
        Act::Toast(Toast::new("first").lasting(Duration::from_secs(1))),
        Act::Toast(Toast::new("second")),
        Act::Toast(Toast::new("third")),
        Act::Toast(Toast::new("low one").at(Spot::BOTTOM)),
        Act::Toast(Toast::new("low two").at(Spot::BOTTOM)),
    ]);
    let buffer = frame(&mut tui);
    let row = |needle: &str| found(&buffer, needle).map(|(_, y)| y);
    let (first, second, third) = (row("first"), row("second"), row("third"));
    assert_eq!(second, first.map(|y| y + 3));
    assert_eq!(third, first.map(|y| y + 6));
    assert_eq!(row("low two"), row("low one").map(|y| y - 3), "bottom stacks grow up");
    tui.advance(Duration::from_secs(1));
    let buffer = frame(&mut tui);
    assert!(found(&buffer, "first").is_none(), "its second is up");
    assert_eq!(found(&buffer, "second").map(|(_, y)| y), first);
    assert_eq!(found(&buffer, "third").map(|(_, y)| y), second);
}

#[test]
fn a_toast_leaves_exactly_when_its_time_is_up() {
    let mut tui = run(vec![
        Act::Toast(Toast::new("default")),
        Act::Toast(Toast::new("brief").lasting(Duration::from_millis(500)).at(Spot::BOTTOM)),
    ]);
    tui.advance(Duration::from_millis(499));
    let text = shown(&mut tui);
    assert!(text.contains("default") && text.contains("brief"));
    tui.advance(Duration::from_millis(1));
    let text = shown(&mut tui);
    assert!(text.contains("default") && !text.contains("brief"));
    tui.advance(Duration::from_millis(2499));
    assert!(shown(&mut tui).contains("default"), "three seconds by default");
    tui.advance(Duration::from_millis(1));
    assert!(!shown(&mut tui).contains("default"));
}

#[test]
fn a_toast_wears_its_tone_on_text_and_border() {
    for tone in [Tone::Ink, Tone::Good, Tone::Warn, Tone::Bad, Tone::Accent] {
        let mut tui = run(vec![Act::Toast(Toast::new("toned").tone(tone))]);
        let buffer = frame(&mut tui);
        let Some((x, y)) = found(&buffer, "toned") else {
            panic!("{tone:?} toast is drawn");
        };
        let style = palette().tone(tone);
        if let Some(fg) = style.fg {
            assert_eq!(buffer[(x, y)].fg, fg, "{tone:?} text");
            assert_eq!(buffer[(x - 2, y)].fg, fg, "{tone:?} border");
        }
        assert!(buffer[(x, y)].modifier.contains(style.add_modifier));
    }
}

#[test]
fn more_toasts_than_fit_are_dropped_not_drawn_out_of_bounds() {
    let queue = (0..12)
        .map(|n| Act::Toast(Toast::new(format!("note {n:02}"))))
        .collect();
    let mut tui = run(queue);
    let text = shown(&mut tui);
    assert!(text.contains("note 00"));
    assert!(!text.contains("note 11"), "{text}");
    let small = dump::text(&tui.frame(40, 12));
    assert!(small.contains("note 00"));
    let mut tui = run(vec![Act::Toast(Toast::new("a".repeat(200)))]);
    let buffer = frame(&mut tui);
    assert!(dump::text(&buffer).contains(&"a".repeat(20)), "a wide toast is clipped to the area");
}

#[test]
fn a_toast_raised_while_a_modal_is_open_draws_over_it_and_takes_no_keys() {
    let mut tui = run(vec![
        Act::Open(Modal::new(1, "Rename").field(Field::new("Name"))),
        Act::Toast(Toast::new("Heads up").at(Spot::THIRD)),
    ]);
    let text = shown(&mut tui);
    assert!(text.contains("Heads up") && text.contains("Name"));
    tui.key(press(KeyCode::Char('z')));
    assert_eq!(heard(&tui), [(1, Event::Edited { part: 0 })]);
    more(&mut tui, Vec::new());
    assert_eq!(heard(&tui).len(), 2, "x typed into the modal, not the screen");
}
