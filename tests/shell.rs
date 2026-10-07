use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::{Input, InputBar};
use pito_tui::header::Section;
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier},
};
use pito_tui::{Cx, Flow, Job, Palette, Phase, Screen, Tui, Words, dump};

#[derive(Default)]
struct Probe<const ID: u8> {
    keys: Vec<KeyCode>,
    answers: Vec<bool>,
    started: usize,
    typing: bool,
    busy: usize,
    loads: u32,
    waiting: bool,
    loaded: Vec<u32>,
    cancelled: bool,
    field: Input,
}

impl<const ID: u8> Screen<u32> for Probe<ID> {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn start(&mut self, _cx: &mut Cx<'_, u32>) {
        self.started += 1;
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        if key.code == KeyCode::Char('d') {
            cx.confirm("Delete it?");
        }
        self.keys.push(key.code);
    }

    fn answer(&mut self, yes: bool, _cx: &mut Cx<'_, u32>) {
        self.answers.push(yes);
    }

    fn typing(&self) -> bool {
        self.typing
    }

    fn input(&self) -> Option<InputBar<'_>> {
        self.typing
            .then(|| InputBar::new("Find", &self.field).hint("enter keep"))
    }

    fn busy(&self) -> usize {
        self.busy
    }

    fn sources(&self) -> &[&str] {
        &["feed"]
    }

    fn phase(&self) -> Phase<'_> {
        if self.waiting {
            Phase::Loading("Reading the feed...".into())
        } else {
            Phase::Ready
        }
    }

    fn load(&mut self) -> Option<Job<u32>> {
        self.loads += 1;
        self.waiting = true;
        let answer = self.loads;
        Some(Box::new(move || answer))
    }

    fn loaded(&mut self, answer: u32, _cx: &mut Cx<'_, u32>) {
        self.waiting = false;
        self.loaded.push(answer);
    }

    fn cancel(&mut self) {
        self.waiting = false;
        self.cancelled = true;
    }
}

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn ctrl_c() -> KeyEvent {
    KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL)
}

fn tui() -> Tui<u32> {
    let mut tui = Tui::new("probe", "1.2.3", Color::Blue)
        .words(
            Words::new()
                .help("? keys")
                .again("again to leave")
                .yes("Yes")
                .no("No")
                .busy(|running| format!("{running} running, leave?")),
        )
        .screen(Section::new("One"), Probe::<1>::default())
        .screen(Section::new("Two"), Probe::<2>::default());
    tui.start();
    tui
}

fn one(tui: &mut Tui<u32>) -> &mut Probe<1> {
    tui.find_mut::<Probe<1>>().unwrap()
}

#[test]
fn ctrl_c_twice_quits_and_asks_first_while_work_runs() {
    let mut idle = tui();
    assert_eq!(idle.key(ctrl_c()), Flow::Stay);
    assert!(dump::text(&idle.frame(60, 16)).contains("again to leave"));
    assert_eq!(idle.key(ctrl_c()), Flow::Quit);

    let mut busy = tui();
    one(&mut busy).busy = 2;
    busy.key(ctrl_c());
    assert_eq!(busy.key(ctrl_c()), Flow::Stay);
    assert!(dump::text(&busy.frame(60, 16)).contains("2 running, leave?"));
    assert_eq!(busy.key(press(KeyCode::Char('y'))), Flow::Quit);
    assert!(one(&mut busy).keys.is_empty());
}

fn lit(buffer: &Buffer, text: &str) -> bool {
    let area = buffer.area;
    (area.top()..area.bottom()).any(|y| {
        let row: String = (area.left()..area.right())
            .map(|x| buffer[(x, y)].symbol())
            .collect();
        row.find(text).is_some_and(|at| {
            let x = area.left() + u16::try_from(row[..at].chars().count()).unwrap();
            (x..x + u16::try_from(text.chars().count()).unwrap())
                .filter(|x| buffer[(*x, y)].symbol() != " ")
                .all(|x| {
                    let cell = &buffer[(x, y)];
                    cell.fg == Color::Blue && cell.modifier == Modifier::BOLD
                })
        })
    })
}

#[test]
fn a_first_ctrl_c_shows_again_on_an_open_bar_in_the_accent_until_the_guard_lapses() {
    let mut tui = tui();
    one(&mut tui).typing = true;
    tui.key(ctrl_c());
    let armed = tui.frame(60, 16);
    assert!(lit(&armed, "again to leave"), "{}", dump::text(&armed));
    assert!(!dump::text(&armed).contains("enter keep"));
    tui.key(press(KeyCode::Char('x')));
    let lapsed = dump::text(&tui.frame(60, 16));
    assert!(lapsed.contains("enter keep") && !lapsed.contains("again to leave"));

    one(&mut tui).typing = false;
    tui.key(press(KeyCode::Char('d')));
    tui.key(ctrl_c());
    let asking = tui.frame(60, 16);
    assert!(dump::text(&asking).contains("Delete it?"));
    assert!(lit(&asking, "again to leave"), "{}", dump::text(&asking));
}

#[test]
fn keys_go_to_the_confirm_then_the_input_then_help_then_nav_then_the_screen() {
    let mut tui = tui();
    tui.key(press(KeyCode::Char('d')));
    tui.key(press(KeyCode::Char('x')));
    tui.key(press(KeyCode::Char('y')));
    assert_eq!(one(&mut tui).keys, [KeyCode::Char('d')]);
    assert_eq!(one(&mut tui).answers, [true]);

    one(&mut tui).typing = true;
    tui.key(press(KeyCode::Char('?')));
    tui.key(press(KeyCode::Char('2')));
    assert_eq!(tui.current(), Some(0));
    one(&mut tui).typing = false;
    assert_eq!(
        one(&mut tui).keys,
        [KeyCode::Char('d'), KeyCode::Char('?'), KeyCode::Char('2')]
    );

    assert!(!dump::text(&tui.frame(60, 16)).contains("? keys"));
    tui.key(press(KeyCode::Char('?')));
    assert!(dump::text(&tui.frame(60, 16)).contains("? keys"));
    tui.key(press(KeyCode::Char('2')));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(one(&mut tui).keys.len(), 3);

    tui.key(press(KeyCode::Char('z')));
    let two = tui.find::<Probe<2>>().unwrap();
    assert_eq!(two.started, 1);
    assert_eq!(two.keys, [KeyCode::Char('z')]);
}

#[test]
fn a_stopped_load_drops_its_late_answer_and_a_changed_source_reads_again() {
    let mut tui = tui();
    tui.shot(60, 16, &[], false);
    assert!(one(&mut tui).waiting);
    assert!(dump::text(&tui.frame(60, 16)).contains("Reading the feed..."));
    assert_eq!(tui.key(press(KeyCode::Esc)), Flow::Stay);
    assert!(one(&mut tui).cancelled);
    assert!(one(&mut tui).keys.is_empty());

    tui.changed("feed");
    tui.settle();
    assert_eq!(one(&mut tui).loaded, [2]);
    assert!(
        !tui.loaded(0, 1, 1),
        "the stopped first read comes back too late"
    );
    assert_eq!(one(&mut tui).loaded, [2]);
}
