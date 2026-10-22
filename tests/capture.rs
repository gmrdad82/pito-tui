use std::fs;
use std::io::ErrorKind;
use std::path::{Path, PathBuf};
use std::time::Duration;

use pito_tui::capture::{Difference, Look, Recorder, Script, Walk, capture, compare, compare_dirs};
use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::Key;
use pito_tui::header::Section;
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Paragraph,
};
use pito_tui::{Cx, Palette, Screen, Tui, Words};

const HEAD: &str = "pito-tui frame 1";
const SIZES: [&str; 4] = ["120x34", "200x60", "40x12", "80x24"];

struct Sign {
    text: &'static str,
    typed: usize,
}

impl Screen<u32> for Sign {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        let line = format!("{} {}", self.text, self.typed);
        let style = Style::new().fg(Color::Green).add_modifier(Modifier::BOLD);
        frame.render_widget(Paragraph::new(line).style(style), area);
    }

    fn key(&mut self, key: KeyEvent, _cx: &mut Cx<'_, u32>) {
        if let KeyCode::Char(_) = key.code {
            self.typed += 1;
        }
    }
}

fn sign(text: &'static str) -> Sign {
    Sign { text, typed: 0 }
}

fn app(home: &'static str) -> impl Fn() -> Tui<u32> {
    move || {
        Tui::new("probe", "1.2.3", Color::Blue)
            .words(Words::new().help("? keys").again("again to leave"))
            .screen(Section::new("Home"), sign(home))
            .screen(Section::new("What to do, next"), sign("next"))
            .overlay(Section::new("Drill In"), &[Key::Char('D')], sign("drill"))
            .app(sign("app"))
    }
}

fn ctrl_c() -> KeyEvent {
    KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL)
}

fn x() -> KeyEvent {
    KeyEvent::new(KeyCode::Char('x'), KeyModifiers::NONE)
}

fn scripted() -> Walk {
    Walk::new()
        .script(Script::new("Quit  guard", [ctrl_c()]))
        .script(Script::new("Type: X/X", [x(), x()]))
}

fn root(name: &str) -> PathBuf {
    let dir = Path::new(env!("CARGO_TARGET_TMPDIR"))
        .join("capture-cover")
        .join(name);
    let _ = fs::remove_dir_all(&dir);
    dir
}

fn listed(dir: &Path) -> Vec<String> {
    let mut names: Vec<String> = fs::read_dir(dir)
        .unwrap()
        .map(|entry| entry.unwrap().file_name().to_string_lossy().into_owned())
        .collect();
    names.sort();
    names
}

fn look(symbol: &str, fg: &str, bg: &str, modifier: u16) -> Look {
    let mut look = Look::default();
    look.symbol = symbol.to_string();
    look.fg = fg.to_string();
    look.bg = bg.to_string();
    look.modifier = modifier;
    look
}

fn blank(width: u16, height: u16) -> Buffer {
    Buffer::empty(Rect::new(0, 0, width, height))
}

fn escapes() -> Buffer {
    let mut buffer = blank(4, 2);
    buffer[(0, 0)]
        .set_symbol("\\")
        .set_fg(Color::Rgb(0x5B, 0x8C, 0xFF))
        .set_style(Style::new().add_modifier(Modifier::BOLD));
    buffer[(1, 0)].set_symbol("\t");
    buffer[(2, 0)].set_symbol("\n");
    buffer[(3, 0)].set_symbol("\r");
    buffer.set_string(0, 1, "a  b", Style::new().add_modifier(Modifier::DIM));
    buffer[(3, 1)].set_bg(Color::Indexed(17));
    buffer
}

const ESCAPES: &str = concat!(
    "pito-tui frame 1\n",
    "4 2\n",
    "\\\\\t\\t\t\\n\t\\r\n",
    "1 #5B8CFF Reset 1\t3 Reset Reset 0\n",
    "a\t \t \tb\n",
    "3 Reset Reset 2\t1 Reset 17 2\n",
);

fn joined(line: &str) -> String {
    line.replace('\t', "")
}

#[test]
fn a_walk_writes_one_folder_per_scenario_named_as_documented_and_one_file_per_size() {
    let dir = root("walk");
    let frames = capture(&dir, &scripted(), app("home")).unwrap();
    assert_eq!(frames, 5 * 4);
    assert_eq!(
        listed(&dir),
        ["00-home", "01-what-to-do-next", "02-drill-in", "quit-guard", "type-x-x"]
    );
    for scenario in listed(&dir) {
        assert_eq!(listed(&dir.join(&scenario)), SIZES, "{scenario}");
    }
}

#[test]
fn every_written_frame_holds_its_head_size_and_two_lines_a_row() {
    let dir = root("shape");
    capture(&dir, &scripted(), app("home")).unwrap();
    for scenario in listed(&dir) {
        for name in SIZES {
            let (width, height) = pito_tui::dump::size(name).unwrap();
            let file = fs::read_to_string(dir.join(&scenario).join(name)).unwrap();
            assert!(file.ends_with('\n'));
            let lines: Vec<&str> = file.lines().collect();
            assert_eq!(lines[0], HEAD);
            assert_eq!(lines[1], format!("{width} {height}"));
            assert_eq!(lines.len(), 2 + 2 * usize::from(height), "{scenario}/{name}");
            for row in lines[2..].chunks(2) {
                assert_eq!(row[0].split('\t').count(), usize::from(width));
                let mut cells = 0;
                for run in row[1].split('\t') {
                    let parts: Vec<&str> = run.split(' ').collect();
                    assert_eq!(parts.len(), 4, "{run:?}");
                    let count: usize = parts[0].parse().unwrap();
                    assert!(count > 0);
                    parts[3].parse::<u16>().unwrap();
                    cells += count;
                }
                assert_eq!(cells, usize::from(width));
            }
        }
    }
}

#[test]
fn each_scenario_draws_its_own_screen_or_script() {
    let dir = root("drawn");
    let walk = scripted()
        .sizes([(60, 16)])
        .script(Script::new("Later", [ctrl_c()]).at(Duration::from_secs(5)));
    capture(&dir, &walk, app("home")).unwrap();
    let text = |scenario: &str| joined(&fs::read_to_string(dir.join(scenario).join("60x16")).unwrap());
    assert!(text("00-home").contains("home 0"));
    assert!(text("01-what-to-do-next").contains("next 0"));
    assert!(text("02-drill-in").contains("drill 0"));
    assert!(text("quit-guard").contains("again to leave"));
    assert!(text("type-x-x").contains("home 2"));
    assert!(!text("later").contains("again to leave"));
}

#[test]
fn a_walk_without_screens_writes_only_its_scripts_at_its_own_sizes() {
    let dir = root("scripts-only");
    let walk = scripted().screens(false).sizes([(60, 16), (40, 12)]);
    assert_eq!(capture(&dir, &walk, app("home")).unwrap(), 4);
    assert_eq!(listed(&dir), ["quit-guard", "type-x-x"]);
    assert_eq!(listed(&dir.join("quit-guard")), ["40x12", "60x16"]);
}

#[test]
fn the_default_walk_takes_every_screen_at_the_four_documented_sizes() {
    let walk = Walk::new();
    assert_eq!(walk, Walk::default());
    assert!(walk.screens);
    assert!(walk.scripts.is_empty());
    assert_eq!(walk.sizes, [(40, 12), (80, 24), (120, 34), (200, 60)]);
    let script = Script::new("s", [x()]);
    assert!(script.settle);
    assert_eq!(script.at, None);
    assert!(!script.clone().loading().settle);
    assert_eq!(script.at(Duration::from_secs(1)).at, Some(Duration::from_secs(1)));
}

#[test]
fn a_walk_into_a_missing_folder_makes_it_and_one_into_a_used_folder_is_refused() {
    let dir = root("used").join("deep").join("er");
    let walk = scripted().sizes([(40, 12)]);
    assert_eq!(capture(&dir, &walk, app("home")).unwrap(), 5);
    let again = capture(&dir, &walk, app("home")).unwrap_err();
    assert_eq!(again.kind(), ErrorKind::AlreadyExists);

    let empty = root("empty");
    fs::create_dir_all(&empty).unwrap();
    assert_eq!(capture(&empty, &walk, app("home")).unwrap(), 5);
}

#[test]
fn a_recorder_writes_byte_for_byte_what_a_walk_writes() {
    let walked = root("walked");
    let walk = scripted().sizes([(40, 12), (60, 16)]);
    capture(&walked, &walk, app("home")).unwrap();

    let recorded = root("recorded");
    let make = app("home");
    let mut recorder = Recorder::new(&recorded).unwrap();
    for (width, height) in [(40, 12), (60, 16)] {
        recorder
            .screen(0, "Home", &make().shot(width, height, &[], true))
            .unwrap();
        let mut next = make();
        next.shot(width, height, &[], true);
        assert!(next.go(1));
        next.settle();
        recorder
            .screen(1, "What to do, next", &next.frame(width, height))
            .unwrap();
        let mut drill = make();
        drill.shot(width, height, &[], true);
        assert!(drill.go(2));
        drill.settle();
        recorder
            .screen(2, "Drill In", &drill.frame(width, height))
            .unwrap();
        recorder
            .record("Quit  guard", &make().shot(width, height, &[ctrl_c()], true))
            .unwrap();
        recorder
            .record("Type: X/X", &make().shot(width, height, &[x(), x()], true))
            .unwrap();
    }
    assert_eq!(recorder.frames(), 10);
    assert_eq!(recorder.finish().unwrap(), []);

    assert_eq!(listed(&recorded), listed(&walked));
    for scenario in listed(&walked) {
        for size in ["40x12", "60x16"] {
            assert_eq!(
                fs::read(recorded.join(&scenario).join(size)).unwrap(),
                fs::read(walked.join(&scenario).join(size)).unwrap(),
                "{scenario}/{size}"
            );
        }
    }
    assert_eq!(compare_dirs(&walked, &recorded).unwrap(), []);
    assert_eq!(compare(&recorded, &walk, app("home")).unwrap(), []);
}

#[test]
fn a_frame_file_is_written_exactly_as_documented_with_escapes_and_style_runs() {
    let dir = root("exact");
    let mut recorder = Recorder::new(&dir).unwrap();
    recorder.record("Escapes", &escapes()).unwrap();
    assert_eq!(recorder.frames(), 1);
    assert_eq!(fs::read_to_string(dir.join("escapes").join("4x2")).unwrap(), ESCAPES);
}

#[test]
fn a_frame_file_reads_back_as_written_through_compare() {
    let dir = root("round-trip");
    Recorder::new(&dir).unwrap().record("Escapes", &escapes()).unwrap();
    let mut same = Recorder::against(&dir).unwrap();
    same.record("Escapes", &escapes()).unwrap();
    assert_eq!(same.frames(), 1);
    assert_eq!(same.finish().unwrap(), []);

    let mut changed = escapes();
    changed[(1, 0)].set_symbol("X");
    changed[(0, 1)].set_style(Style::new().add_modifier(Modifier::ITALIC));
    changed[(3, 1)].set_bg(Color::Rgb(1, 2, 3));
    let mut against = Recorder::against(&dir).unwrap();
    against.record("Escapes", &changed).unwrap();
    assert_eq!(
        against.finish().unwrap(),
        [
            Difference::Cell {
                scenario: "escapes".into(),
                size: (4, 2),
                x: 1,
                y: 0,
                before: look("\t", "Reset", "Reset", 0),
                after: look("X", "Reset", "Reset", 0),
            },
            Difference::Cell {
                scenario: "escapes".into(),
                size: (4, 2),
                x: 0,
                y: 1,
                before: look("a", "Reset", "Reset", 2),
                after: look("a", "Reset", "Reset", 6),
            },
            Difference::Cell {
                scenario: "escapes".into(),
                size: (4, 2),
                x: 3,
                y: 1,
                before: look("b", "Reset", "17", 2),
                after: look("b", "Reset", "#010203", 2),
            },
        ]
    );
}

#[test]
fn every_escaped_symbol_and_the_first_cell_read_back_whole() {
    let dir = root("escaped");
    Recorder::new(&dir).unwrap().record("e", &escapes()).unwrap();
    let mut gone = escapes();
    for (x, y) in [(0, 0), (1, 0), (2, 0), (3, 0)] {
        gone[(x, y)].set_symbol("_");
    }
    let mut against = Recorder::against(&dir).unwrap();
    against.record("e", &gone).unwrap();
    let before: Vec<String> = against
        .finish()
        .unwrap()
        .into_iter()
        .map(|difference| match difference {
            Difference::Cell { before, .. } => before.symbol,
            other => panic!("{other}"),
        })
        .collect();
    assert_eq!(before, ["\\", "\t", "\n", "\r"]);
}

#[test]
fn differences_print_one_a_line_as_documented() {
    let cell = Difference::Cell {
        scenario: "00-home".into(),
        size: (80, 24),
        x: 0,
        y: 0,
        before: look("X", "Reset", "Reset", 0),
        after: look(" ", "Reset", "Reset", 0),
    };
    assert_eq!(
        cell.to_string(),
        "00-home 80x24 0,0: \"X\" Reset Reset NONE → \" \" Reset Reset NONE"
    );
    let added = Difference::Added {
        scenario: "about".into(),
        size: (40, 12),
    };
    assert_eq!(added.to_string(), "about 40x12: +");
    let removed = Difference::Removed {
        scenario: "gone".into(),
        size: (80, 24),
    };
    assert_eq!(removed.to_string(), "gone 80x24: -");
    let styled = Difference::Cell {
        scenario: "s".into(),
        size: (4, 2),
        x: 3,
        y: 1,
        before: look("\t", "#5B8CFF", "17", 1),
        after: look("b", "Red", "Reset", 1 | 4),
    };
    assert_eq!(
        styled.to_string(),
        "s 4x2 3,1: \"\\t\" #5B8CFF 17 BOLD → \"b\" Red Reset BOLD | ITALIC"
    );
}

#[test]
fn compare_dirs_lists_added_and_changed_frames_in_order_then_the_removed() {
    let root = root("ordered");
    let (before, after) = (root.join("before"), root.join("after"));
    let mut old = Recorder::new(&before).unwrap();
    old.record("b", &blank(3, 2)).unwrap();
    old.record("gone", &blank(40, 12)).unwrap();
    old.record("gone", &blank(120, 34)).unwrap();

    let mut new = Recorder::new(&after).unwrap();
    let mut changed = blank(3, 2);
    changed[(1, 0)].set_symbol("1");
    changed[(0, 1)].set_symbol("2");
    new.record("b", &changed).unwrap();
    new.record("a", &blank(120, 34)).unwrap();
    new.record("a", &blank(40, 12)).unwrap();

    let differences = compare_dirs(&before, &after).unwrap();
    assert_eq!(differences.len(), 6);
    assert_eq!(
        differences[..4],
        [
            Difference::Added {
                scenario: "a".into(),
                size: (40, 12),
            },
            Difference::Added {
                scenario: "a".into(),
                size: (120, 34),
            },
            Difference::Cell {
                scenario: "b".into(),
                size: (3, 2),
                x: 1,
                y: 0,
                before: look(" ", "Reset", "Reset", 0),
                after: look("1", "Reset", "Reset", 0),
            },
            Difference::Cell {
                scenario: "b".into(),
                size: (3, 2),
                x: 0,
                y: 1,
                before: look(" ", "Reset", "Reset", 0),
                after: look("2", "Reset", "Reset", 0),
            },
        ]
    );
    let mut removed = differences[4..].to_vec();
    removed.sort_by_key(|difference| match difference {
        Difference::Removed { size, .. } => *size,
        other => panic!("{other}"),
    });
    assert_eq!(
        removed,
        [
            Difference::Removed {
                scenario: "gone".into(),
                size: (40, 12),
            },
            Difference::Removed {
                scenario: "gone".into(),
                size: (120, 34),
            },
        ]
    );
    assert_eq!(compare_dirs(&before, &after).unwrap(), differences);
}

#[test]
fn compare_dirs_skips_files_that_are_not_frames() {
    let root = root("strays");
    let (before, after) = (root.join("before"), root.join("after"));
    Recorder::new(&before).unwrap().record("e", &escapes()).unwrap();
    Recorder::new(&after).unwrap().record("e", &escapes()).unwrap();
    fs::write(after.join("notes.txt"), "not a frame").unwrap();
    fs::write(after.join("e").join("README"), "not a frame").unwrap();
    fs::write(before.join("e").join("0x0"), "not a frame").unwrap();
    assert_eq!(compare_dirs(&before, &after).unwrap(), []);
}

#[test]
fn a_walk_compares_clean_against_its_own_capture_and_reports_frames_gone_and_new() {
    let dir = root("walk-compare");
    let walk = scripted().sizes([(40, 12)]);
    capture(&dir, &walk, app("home")).unwrap();
    assert_eq!(compare(&dir, &walk, app("home")).unwrap(), []);

    fs::remove_file(dir.join("type-x-x").join("40x12")).unwrap();
    fs::create_dir_all(dir.join("old")).unwrap();
    fs::write(dir.join("old").join("40x12"), ESCAPES).unwrap();
    assert_eq!(
        compare(&dir, &walk, app("home")).unwrap(),
        [
            Difference::Added {
                scenario: "type-x-x".into(),
                size: (40, 12),
            },
            Difference::Removed {
                scenario: "old".into(),
                size: (40, 12),
            },
        ]
    );

    let changed = compare(&dir, &walk, app("HOME")).unwrap();
    assert!(changed.iter().any(|difference| matches!(
        difference,
        Difference::Cell { scenario, after, .. } if scenario == "00-home" && after.symbol == "H"
    )));
    assert!(changed.iter().all(|difference| match difference {
        Difference::Cell { scenario, .. } => scenario != "01-what-to-do-next",
        _ => true,
    }));
}

#[test]
fn a_recorder_refuses_a_used_folder_and_a_repeated_name() {
    let dir = root("repeat");
    let mut recorder = Recorder::new(&dir).unwrap();
    recorder.record("Quit guard", &blank(4, 2)).unwrap();
    recorder.record("Quit guard", &blank(5, 2)).unwrap();
    let repeat = recorder.record("quit   GUARD", &blank(4, 2)).unwrap_err();
    assert_eq!(repeat.kind(), ErrorKind::AlreadyExists);
    recorder.screen(0, "Quit guard", &blank(4, 2)).unwrap();
    assert!(recorder.screen(0, "quit guard", &blank(4, 2)).is_err());
    assert_eq!(recorder.frames(), 3);
    assert_eq!(listed(&dir), ["00-quit-guard", "quit-guard"]);
    assert_eq!(recorder.finish().unwrap(), []);

    let used = Recorder::new(&dir).unwrap_err();
    assert_eq!(used.kind(), ErrorKind::AlreadyExists);

    let file = root("a-file");
    fs::create_dir_all(file.parent().unwrap()).unwrap();
    fs::write(&file, "x").unwrap();
    assert!(Recorder::new(&file).is_err());
}

#[test]
fn a_recorder_against_refuses_a_repeated_name_and_lists_none_for_an_identical_run() {
    let dir = root("against-repeat");
    let mut recorder = Recorder::new(&dir).unwrap();
    recorder.record("one", &escapes()).unwrap();
    recorder.record("two", &blank(3, 3)).unwrap();

    let mut against = Recorder::against(&dir).unwrap();
    against.record("one", &escapes()).unwrap();
    let repeat = against.record("ONE", &escapes()).unwrap_err();
    assert_eq!(repeat.kind(), ErrorKind::AlreadyExists);
    against.record("two", &blank(3, 3)).unwrap();
    assert_eq!(against.frames(), 2);
    assert_eq!(against.finish().unwrap(), []);
    assert_eq!(listed(&dir), ["one", "two"]);
}

#[test]
fn a_missing_folder_is_an_error_everywhere_it_is_read() {
    let root = root("missing");
    let present = root.join("present");
    Recorder::new(&present).unwrap().record("e", &escapes()).unwrap();
    let absent = root.join("absent");
    let walk = scripted().sizes([(40, 12)]);
    assert_eq!(
        Recorder::against(&absent).unwrap_err().kind(),
        ErrorKind::NotFound
    );
    assert_eq!(
        compare_dirs(&absent, &present).unwrap_err().kind(),
        ErrorKind::NotFound
    );
    assert_eq!(
        compare_dirs(&present, &absent).unwrap_err().kind(),
        ErrorKind::NotFound
    );
    assert_eq!(
        compare(&absent, &walk, app("home")).unwrap_err().kind(),
        ErrorKind::NotFound
    );
}

fn broken(name: &str, text: &[u8]) -> (PathBuf, PathBuf) {
    let root = root(name);
    let (before, after) = (root.join("before"), root.join("after"));
    fs::create_dir_all(before.join("e")).unwrap();
    fs::write(before.join("e").join("4x2"), text).unwrap();
    Recorder::new(&after).unwrap().record("e", &escapes()).unwrap();
    (before, after)
}

fn refused(name: &str, text: &[u8]) {
    let (before, after) = broken(name, text);
    let error = compare_dirs(&before, &after).unwrap_err();
    assert_eq!(error.kind(), ErrorKind::InvalidData, "{name}");
    let mut against = Recorder::against(&before).unwrap();
    let error = against.record("e", &escapes()).unwrap_err();
    assert_eq!(error.kind(), ErrorKind::InvalidData, "{name}");
    let error = compare_dirs(&after, &before).unwrap_err();
    assert_eq!(error.kind(), ErrorKind::InvalidData, "{name}");
}

#[test]
fn a_frame_with_another_head_is_refused() {
    refused("head", ESCAPES.replace(HEAD, "pito-tui frame 2").as_bytes());
    refused("no-head", ESCAPES.replacen("pito-tui frame 1\n", "", 1).as_bytes());
    refused("empty", b"");
}

#[test]
fn a_frame_whose_size_line_does_not_parse_is_refused() {
    for (name, size) in [
        ("size-x", "4x2"),
        ("size-words", "four two"),
        ("size-one", "4"),
        ("size-blank", ""),
        ("size-negative", "-4 2"),
        ("size-overflow", "70000 2"),
        ("size-tab", "4\t2"),
    ] {
        refused(name, ESCAPES.replacen("4 2\n", &format!("{size}\n"), 1).as_bytes());
    }
    refused("size-missing", format!("{HEAD}\n").as_bytes());
}

#[test]
fn a_truncated_frame_is_refused() {
    let lines: Vec<&str> = ESCAPES.lines().collect();
    for keep in 2..lines.len() {
        let text = lines[..keep].join("\n");
        refused(&format!("truncated-{keep}"), text.as_bytes());
    }
}

#[test]
fn a_style_run_that_does_not_parse_is_refused() {
    for (name, run) in [
        ("run-short", "1 #5B8CFF Reset"),
        ("run-count", "one #5B8CFF Reset 1"),
        ("run-modifier", "1 #5B8CFF Reset bold"),
        ("run-negative", "-1 #5B8CFF Reset 1"),
        ("run-modifier-overflow", "1 #5B8CFF Reset 70000"),
    ] {
        refused(name, ESCAPES.replacen("1 #5B8CFF Reset 1", run, 1).as_bytes());
    }
}

#[test]
fn an_unreadable_frame_is_an_error() {
    let (before, after) = broken("not-utf8", &[0xff, 0xfe, 0x00, 0x80]);
    assert!(compare_dirs(&before, &after).is_err());
    assert!(compare_dirs(&after, &before).is_err());
    assert!(Recorder::against(&before).unwrap().record("e", &escapes()).is_err());

    let root = root("a-folder");
    let (before, after) = (root.join("before"), root.join("after"));
    fs::create_dir_all(before.join("e").join("4x2")).unwrap();
    Recorder::new(&after).unwrap().record("e", &escapes()).unwrap();
    assert!(compare_dirs(&before, &after).is_err());
    assert!(compare_dirs(&after, &before).is_err());
    assert!(Recorder::against(&before).unwrap().record("e", &escapes()).is_err());
}
