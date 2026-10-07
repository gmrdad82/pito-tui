use std::fs;
use std::path::Path;
use std::time::{Duration, Instant};

use pito_tui::activity::{LINGER, State};
use pito_tui::capture::{Difference, Script, Walk, capture, compare};
use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::{Input, InputBar, Key};
use pito_tui::header::Section;
use pito_tui::ratatui::{
    Frame,
    buffer::{Buffer, Cell},
    layout::Rect,
    style::{Color, Modifier, Style},
    text::Line,
};
use pito_tui::{Activities, Activity, Cx, Flow, Job, Palette, Phase, Screen, Tui, Words, dump};

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

fn rows(buffer: &Buffer, text: &str, look: impl Fn(&Cell) -> bool) -> usize {
    let area = buffer.area;
    (area.top()..area.bottom())
        .filter(|&y| {
            let row: String = (area.left()..area.right())
                .map(|x| buffer[(x, y)].symbol())
                .collect();
            row.find(text).is_some_and(|at| {
                let x = area.left() + u16::try_from(row[..at].chars().count()).unwrap();
                (x..x + u16::try_from(text.chars().count()).unwrap())
                    .filter(|x| buffer[(*x, y)].symbol() != " ")
                    .all(|x| look(&buffer[(x, y)]))
            })
        })
        .count()
}

fn lit(buffer: &Buffer, text: &str) -> bool {
    rows(buffer, text, |cell| {
        cell.fg == Color::Blue && cell.modifier == Modifier::BOLD
    }) > 0
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

struct Run;

impl Screen<u32> for Run {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn facts(&self) -> Vec<(String, Style)> {
        let red = Style::new().fg(Color::Red);
        vec![("3 passed".into(), Style::new()), ("1 failed".into(), red)]
    }

    fn status_parts(&self, _room: u16) -> Vec<(String, Style)> {
        let red = Style::new().fg(Color::Red);
        vec![
            ("3 passed · ".into(), Style::new()),
            ("1 failed".into(), red),
        ]
    }

    fn crumb(&self) -> Option<String> {
        Some("build".into())
    }
}

#[test]
fn a_drilled_in_screen_keeps_its_facts_row_on_ask_and_its_status_keeps_each_style() {
    let red = |cell: &Cell| cell.fg == Color::Red;
    for (keep, shown) in [(false, 1), (true, 2)] {
        let mut tui = Tui::<u32>::new("probe", "1.2.3", Color::Blue)
            .keep_facts(keep)
            .screen(Section::new("Runs"), Run);
        tui.start();
        let frame = tui.frame(60, 16);
        let text = dump::text(&frame);
        assert!(text.contains("build"), "{text}");
        assert_eq!(rows(&frame, "1 failed", red), shown, "{text}");
        assert_eq!(rows(&frame, "3 passed", red), 0, "{text}");
    }
}

struct Feed;

impl Screen<u32> for Feed {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn start(&mut self, cx: &mut Cx<'_, u32>) {
        let now = cx.now();
        cx.activity(Activity::new(2, "sync", now).state(State::Done));
        cx.activity(
            Activity::new(1, "build", now + Duration::from_millis(1))
                .progress(0.5)
                .detail([Line::from("step one")]),
        );
    }
}

#[test]
fn activities_sit_on_the_footer_and_open_over_any_screen_with_only_the_back_keys() {
    let mut tui = Tui::<u32>::new("probe", "1.2.3", Color::Blue)
        .activities(Activities::new(Section::new("Jobs")).keys(&[Key::Char('o')]))
        .screen(Section::new("One"), Feed)
        .screen(Section::new("Two"), Probe::<2>::default());
    tui.start();
    let text = dump::text(&tui.frame(60, 16));
    let lines: Vec<&str> = text.lines().collect();
    let last = lines.iter().position(|line| line.contains("sync")).unwrap();
    assert!(lines[last - 1].contains("build"), "{text}");
    assert!(lines[last + 1].trim_start().starts_with('─'), "{text}");
    tui.advance(LINGER);
    assert!(!dump::text(&tui.frame(60, 16)).contains("sync"));

    tui.key(press(KeyCode::Char('o')));
    tui.key(press(KeyCode::Char('2')));
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current().map(|index| tui.names()[index]), Some("Jobs"));
    tui.key(press(KeyCode::Enter));
    let text = dump::text(&tui.frame(60, 16));
    assert!(
        text.contains("Jobs / build") && text.contains("step one"),
        "{text}"
    );
    tui.key(press(KeyCode::Esc));
    assert_eq!(tui.current().map(|index| tui.names()[index]), Some("Jobs"));
    tui.key(press(KeyCode::Esc));
    assert_eq!(tui.current().map(|index| tui.names()[index]), Some("One"));
    assert!(tui.find::<Probe<2>>().unwrap().keys.is_empty());
}

#[derive(Default)]
struct Clock {
    due: Option<Instant>,
    ticks: usize,
}

impl Screen<u32> for Clock {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn start(&mut self, cx: &mut Cx<'_, u32>) {
        self.due = Some(cx.now() + Duration::from_secs(1));
    }

    fn deadline(&self) -> Option<Instant> {
        self.due
    }

    fn tick(&mut self, cx: &mut Cx<'_, u32>) {
        self.ticks += 1;
        if self.ticks == 1 {
            self.due = Some(cx.now() + Duration::from_secs(1));
        }
    }
}

#[test]
fn a_deadline_ticks_its_screen_once_when_the_moment_reaches_it() {
    let mut tui = Tui::<u32>::new("probe", "1.2.3", Color::Blue)
        .screen(Section::new("Clock"), Clock::default());
    tui.start();
    tui.settle();
    tui.advance(Duration::from_millis(999));
    assert_eq!(tui.find::<Clock>().unwrap().ticks, 0);
    tui.advance(Duration::from_millis(1));
    assert_eq!(tui.find::<Clock>().unwrap().ticks, 1);
    tui.advance(Duration::from_secs(1));
    tui.advance(Duration::from_secs(5));
    tui.settle();
    assert_eq!(
        tui.find::<Clock>().unwrap().ticks,
        2,
        "a deadline left in the past ticks once"
    );
}

fn accented(accent: Color) -> impl Fn() -> Tui<u32> {
    move || {
        Tui::new("probe", "1.2.3", accent)
            .words(Words::new().help("? keys").again("again to leave"))
            .screen(Section::new("One"), Probe::<1>::default())
    }
}

#[test]
fn a_capture_compares_clean_and_reports_each_changed_cell_and_frame() {
    let dir = Path::new(env!("CARGO_TARGET_TMPDIR")).join("capture");
    let _ = fs::remove_dir_all(&dir);
    let walk = Walk::new()
        .sizes([(40, 12)])
        .script(Script::new("guard", [ctrl_c()]));
    assert_eq!(capture(&dir, &walk, accented(Color::Blue)).unwrap(), 2);
    assert!(capture(&dir, &walk, accented(Color::Blue)).is_err());
    assert_eq!(compare(&dir, &walk, accented(Color::Blue)).unwrap(), []);

    let changed = compare(&dir, &walk, accented(Color::Red)).unwrap();
    assert!(!changed.is_empty());
    assert!(changed.iter().all(|difference| matches!(
        difference,
        Difference::Cell { after, .. } if after.fg == "Red" || after.fg == "Reset"
    )));

    let fewer = Walk::new().sizes([(40, 12)]);
    let removed = compare(&dir, &fewer, accented(Color::Blue)).unwrap();
    assert!(matches!(&removed[..], [Difference::Removed { scenario, .. }] if scenario == "guard"));
}
