use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::{Hint, Key, Tone as Said};
use pito_tui::layer::Event;
use pito_tui::ratatui::{
    Frame, buffer::Buffer, layout::Rect, style::Color, style::Modifier, style::Style,
    widgets::Paragraph,
};
use pito_tui::{
    Choice, Cx, Field, Flow, Head, Item, Job, Label, Modal, Numbers, Palette, Phase, Row, Screen,
    Takeover, Toast, Tui, Waker, Words, dump,
};

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn ctrl(c: char) -> KeyEvent {
    KeyEvent::new(KeyCode::Char(c), KeyModifiers::CONTROL)
}

fn text(tui: &mut Tui<u32>) -> String {
    dump::text(&tui.frame(80, 20))
}

#[derive(Default)]
struct Plain<const ID: u8> {
    keys: Vec<KeyCode>,
    entered: usize,
    waiting: bool,
}

impl<const ID: u8> Screen<u32> for Plain<ID> {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new(format!("screen {ID}")), area);
    }

    fn entered(&mut self, _cx: &mut Cx<'_, u32>) {
        self.entered += 1;
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        if key.code == KeyCode::Char('h') {
            cx.show(|item| item != Item::Tab(0));
        }
        self.keys.push(key.code);
    }

    fn phase(&self) -> Phase<'_> {
        if self.waiting {
            Phase::Loading("Reading the feed".into())
        } else {
            Phase::Ready
        }
    }
}

#[derive(Default)]
struct App {
    heard: Vec<String>,
}

impl Screen<u32> for App {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn claim(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) -> bool {
        if key == ctrl('k') {
            let modal = Modal::new(1, "Go to")
                .field(Field::new("Find"))
                .list([Choice::new("Home"), Choice::new("Lock")])
                .filter(0, 1);
            cx.open(modal);
            return true;
        }
        if key == ctrl('t') {
            let takeover = Takeover::new(2, "Switching")
                .label("one moment")
                .cancel(&[Key::Esc]);
            cx.takeover(takeover);
            return true;
        }
        false
    }

    fn layer(&mut self, id: u64, event: Event, cx: &mut Cx<'_, u32>) {
        if let Event::Chosen { .. } = event {
            cx.close(id);
            cx.toast(Toast::new("Chosen it"));
        }
        self.heard.push(format!("{id} {event:?}"));
    }

    fn status_parts(&self, _room: u16) -> Vec<(String, Style)> {
        vec![("online".to_string(), Style::new())]
    }
}

#[derive(Default)]
struct Gate {
    keys: Vec<KeyCode>,
}

impl Screen<u32> for Gate {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new("locked"), area);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        if key.code == KeyCode::Enter {
            cx.gate(false);
        }
        self.keys.push(key.code);
    }
}

#[test]
fn layers_take_keys_takeover_first_then_modals_then_the_apps_claim_then_the_gate() {
    let mut tui = Tui::new("probe", "1.0.0", Color::Blue)
        .screen("One", Plain::<1>::default())
        .app(App::default())
        .gate(Gate::default(), true);
    tui.start();
    let shown = text(&mut tui);
    assert!(shown.contains("locked") && shown.contains("online"));
    assert!(!shown.contains("One"), "no tabs over a gate");
    tui.key(press(KeyCode::Char('x')));
    assert_eq!(tui.find::<Gate>().unwrap().keys, [KeyCode::Char('x')]);
    tui.key(ctrl('k'));
    assert!(
        text(&mut tui).contains("Go to"),
        "the app's modal opens over the gate"
    );
    tui.key(press(KeyCode::Char('l')));
    tui.key(press(KeyCode::Enter));
    let heard = &tui.find::<App>().unwrap().heard;
    assert_eq!(heard.last().unwrap(), "1 Chosen { part: 1, choice: 1 }");
    let shown = text(&mut tui);
    assert!(!shown.contains("Go to") && shown.contains("Chosen it"));
    assert_eq!(tui.find::<Gate>().unwrap().keys, [KeyCode::Char('x')]);
    tui.key(press(KeyCode::Enter));
    assert!(text(&mut tui).contains("One"), "the gate reveals the app");
    assert_eq!(tui.find::<Plain<1>>().unwrap().entered, 1);
    tui.key(ctrl('t'));
    let shown = text(&mut tui);
    assert!(shown.contains("Switching") && shown.contains("one moment"));
    assert!(!shown.contains("One"));
    tui.key(press(KeyCode::Char('y')));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay, "the quit guard still arms");
    tui.key(press(KeyCode::Esc));
    assert_eq!(tui.find::<App>().unwrap().heard.last().unwrap(), "2 Closed");
    assert!(tui.find::<Plain<1>>().unwrap().keys.is_empty());
    assert!(text(&mut tui).contains("One"));
    tui.advance(Duration::from_secs(4));
    assert!(
        !text(&mut tui).contains("Chosen it"),
        "a toast leaves on its own"
    );
}

#[test]
fn the_nav_rebuilds_from_a_predicate_and_number_and_group_keys_follow_what_is_drawn() {
    let underlined = Style::new().add_modifier(Modifier::UNDERLINED);
    let head = Head::new([Row::Title, Row::Groups, Row::Sections])
        .numbers(Numbers::InGroup)
        .lone(true);
    let mut tui = Tui::new("nav", "1.0.0", Color::Blue)
        .head(head)
        .group(Label::spans([("M", underlined), ("ail", Style::new())]))
        .group_keys(&[Key::Char('m')])
        .screen("Inbox", Plain::<1>::default())
        .screen("Sent", Plain::<2>::default())
        .group("Files")
        .group_keys(&[Key::Char('f')])
        .screen("All", Plain::<3>::default());
    tui.start();
    let shown = text(&mut tui);
    assert!(shown.contains("Mail") && shown.contains("Files"));
    assert!(shown.contains("1 Inbox") && shown.contains("2 Sent"));
    tui.key(press(KeyCode::Char('2')));
    assert_eq!(tui.current(), Some(1));
    tui.key(press(KeyCode::Char('f')));
    assert_eq!(tui.current(), Some(2));
    let shown = text(&mut tui);
    assert!(
        shown.contains(" All") && !shown.contains("1 All"),
        "a lone tab is unnumbered"
    );
    tui.key(press(KeyCode::Char('1')));
    assert_eq!(tui.find::<Plain<3>>().unwrap().keys, [KeyCode::Char('1')]);
    tui.key(press(KeyCode::Char('h')));
    assert_eq!(
        tui.current(),
        Some(2),
        "hiding another tab keeps the current one"
    );
    tui.key(press(KeyCode::Char('m')));
    assert_eq!(
        tui.current(),
        Some(1),
        "a group key returns to its last tab"
    );
    let shown = text(&mut tui);
    assert!(!shown.contains("Inbox") && !shown.contains("2 Sent"));
}

#[derive(Default)]
struct Reader {
    loads: u32,
    got: Vec<u32>,
    waker: Option<Waker<u32>>,
}

impl Screen<u32> for Reader {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn load(&mut self) -> Option<Job<u32>> {
        self.loads += 1;
        let read = 100 + self.loads;
        Some(Box::new(move || read))
    }

    fn loaded(&mut self, read: u32, _cx: &mut Cx<'_, u32>) {
        self.got.push(read);
    }

    fn event(&mut self, event: u32, _cx: &mut Cx<'_, u32>) {
        self.got.push(event);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        match key.code {
            KeyCode::Char('j') => cx.detach(|| 7),
            KeyCode::Char('w') => self.waker = Some(cx.waker()),
            KeyCode::Char('e') => cx.epoch(),
            _ => {}
        }
    }
}

#[test]
fn an_epoch_drops_work_from_before_it_and_every_screen_reads_again() {
    let mut tui = Tui::new("epoch", "1.0.0", Color::Blue).screen("One", Reader::default());
    tui.start();
    tui.settle();
    tui.key(press(KeyCode::Char('w')));
    let waker = tui.find::<Reader>().unwrap().waker.clone().unwrap();
    waker.send(5);
    tui.key(press(KeyCode::Char('j')));
    tui.key(press(KeyCode::Char('e')));
    tui.settle();
    assert_eq!(tui.find::<Reader>().unwrap().got, [101, 102]);
    waker.send(9);
    tui.key(press(KeyCode::Char('j')));
    tui.settle();
    assert_eq!(tui.find::<Reader>().unwrap().got, [101, 102, 7, 9]);
}

fn row_of(buffer: &Buffer, text: &str) -> Option<(u16, u16)> {
    let area = buffer.area;
    (area.top()..area.bottom()).find_map(|y| {
        let line: String = (area.left()..area.right())
            .map(|x| buffer[(x, y)].symbol().to_string())
            .collect();
        let at = line.find(text)?;
        Some((u16::try_from(line[..at].chars().count()).ok()?, y))
    })
}

#[test]
fn a_sticky_guard_stays_armed_through_other_keys_in_its_tone_with_the_hints_hidden() {
    let mut tui = Tui::new("guard", "1.0.0", Color::Blue)
        .words(Words::new().again("again to leave"))
        .hints([Hint::new("x", "explode")])
        .quit_sticky(true)
        .quit_hints(false)
        .quit_tone(Said::Alert)
        .screen("One", Plain::<1>::default());
    tui.start();
    assert!(text(&mut tui).contains("explode"));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay);
    tui.key(press(KeyCode::Char('x')));
    let frame = tui.frame(80, 20);
    let shown = dump::text(&frame);
    assert!(shown.contains("again to leave") && !shown.contains("explode"));
    let (x, y) = row_of(&frame, "again").unwrap();
    assert_eq!(frame[(x, y)].fg, Palette::new(Color::Blue).bad.fg.unwrap());
    assert_eq!(tui.find::<Plain<1>>().unwrap().keys, [KeyCode::Char('x')]);
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);
}

#[test]
fn the_hourglass_waits_its_delay_and_once_shown_stays_its_least() {
    let mut tui = Tui::new("glass", "1.0.0", Color::Blue)
        .hourglass_timing(Duration::from_millis(150), Duration::from_millis(500))
        .screen("One", Plain::<1>::default());
    tui.find_mut::<Plain<1>>().unwrap().waiting = true;
    tui.start();
    assert!(
        !text(&mut tui).contains("Reading the feed"),
        "a quick load shows nothing"
    );
    tui.advance(Duration::from_millis(200));
    assert!(text(&mut tui).contains("Reading the feed"));
    tui.find_mut::<Plain<1>>().unwrap().waiting = false;
    tui.advance(Duration::from_millis(100));
    assert!(
        text(&mut tui).contains("Reading the feed"),
        "a shown hourglass never flashes"
    );
    tui.advance(Duration::from_millis(500));
    assert!(text(&mut tui).contains("screen 1"));
}
