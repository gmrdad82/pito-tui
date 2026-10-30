use std::time::Instant;

use crate::common::{STYLES, draw, show, widest};
use pito_tui::footer::{
    Confirm, ConfirmBar, Footer, Guard, Help, Hint, Input, InputBar, Key, Mode, Notice, QuitGuard,
    Segment, Wording, Words,
};
use ratatui::{
    layout::Rect,
    style::{Color, Style},
};

const HINTS: [Hint; 6] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "open").rank(1),
    Hint::new("/", "search").rank(3),
    Hint::new("s", "stop").rank(2),
    Hint::new("?", "help").rank(4),
    Hint::new("ctrl+c", "twice quit").pinned(),
];

const SEGMENTS: [Segment; 3] = [
    Segment::text(" NORMAL ", Style::new().fg(Color::White)),
    Segment::hints(&HINTS).rank(2),
    Segment::text("12:30", Style::new().fg(Color::Green))
        .right()
        .rank(1),
];

const PLAIN: [Hint; 2] = [Hint::new("↑↓", "move"), Hint::new("enter", "open")];

const LINE: &str = "↑↓ move · enter open · / search · s stop · ? help · ctrl+c twice quit";
const TAG: &str = "app v0.4.0";
const KEYS: [&str; 7] = ["↑↓", "enter", "/", "s", "?", "ctrl+c", "x"];
const WORDS: Words = Words::new("Yes", "No").hint("y/n choose · enter accept · esc cancel");

fn footer() -> Footer<'static> {
    Footer::new(&HINTS).styles(STYLES)
}

fn tagged(footer: Footer) -> Footer {
    footer.version("app", "0.4.0")
}

fn padded(left: &str, width: usize) -> String {
    let left_width = unicode_width::UnicodeWidthStr::width(left);
    format!("{left}{}{TAG}", " ".repeat(width - left_width - TAG.len()))
}

fn rows(footer: Footer, width: u16) -> Vec<String> {
    let height = footer.height(width);
    if height == 0 {
        return Vec::new();
    }
    draw(footer, width, height).text
}

#[test]
fn a_wide_bar_draws_the_version_dim_at_its_right_end() {
    let footer = tagged(footer());
    assert_eq!(footer.height(90), 2);
    let drawn = draw(footer, 90, 2);
    assert_eq!(
        drawn.text,
        ["─".repeat(90), padded(LINE, 90)],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(
        drawn.marks[1],
        format!(
            "{}{}{}",
            "AAmmmmmmmmAAAAAmmmmmmmmAmmmmmmmmmmAmmmmmmmmAmmmmmmmmAAAAAAmmmmmmmmmmm",
            " ".repeat(11),
            "m".repeat(10)
        )
    );
    let indented = draw(footer.indent(2), 90, 2);
    assert_eq!(indented.text[1], padded(&format!("  {LINE}"), 90));
}

#[test]
fn a_narrow_bar_drops_the_version_before_any_hint() {
    let footer = tagged(footer());
    assert_eq!(rows(footer, 81)[1], padded(LINE, 81));
    assert_eq!(rows(footer, 80)[1], LINE);
    assert_eq!(rows(footer, 69)[1], LINE);
    assert_eq!(
        rows(footer, 68)[1],
        "↑↓ move · enter open · / search · s stop · ctrl+c twice quit"
    );
    assert_eq!(rows(footer, 10)[1], "ctrl+c …");
    for width in 0..=130 {
        let bare = footer.version("", "");
        assert_eq!(footer.height(width), bare.height(width), "{width}");
        for key in KEYS {
            assert_eq!(
                footer.shown(key, width),
                bare.shown(key, width),
                "{key} {width}"
            );
        }
        let drawn = rows(footer, width);
        if width < 81 {
            assert_eq!(drawn, rows(bare, width), "{width}");
        } else {
            assert!(drawn[1].ends_with(TAG), "{width}");
        }
    }
}

#[test]
fn no_version_draws_nothing() {
    let builds: [fn(Footer<'static>) -> Footer<'static>; 3] = [
        |footer| footer,
        |footer| footer.version("", ""),
        |footer| footer.version("app", ""),
    ];
    let notice = "The import failed: the file could not be read because another program holds it";
    let configs = [
        footer(),
        footer().wrap(true),
        footer().indent(4),
        footer().open(false),
        Footer::new(&PLAIN).styles(STYLES).open(false),
        footer().notice(Some(Notice::alert(notice).over())),
        Footer::status(&SEGMENTS).styles(STYLES),
    ];
    for config in configs {
        for width in 0..=130 {
            let plain = rows(config, width);
            for build in builds {
                let footer = build(config);
                assert_eq!(footer.height(width), config.height(width), "{width}");
                assert_eq!(rows(footer, width), plain, "{width}");
            }
        }
    }
}

#[test]
fn the_version_is_drawn_whole_or_not_at_all() {
    let configs = [
        footer(),
        footer().wrap(true),
        footer().indent(4),
        footer().open(false),
        Footer::new(&PLAIN).styles(STYLES).open(false),
        Footer::status(&SEGMENTS).styles(STYLES),
    ];
    for config in configs {
        let footer = tagged(config);
        for width in 0..=130 {
            let drawn = rows(footer, width);
            assert!(drawn.len() <= usize::from(footer.height(width)), "{width}");
            for row in &drawn {
                assert!(
                    !row.contains("app") || row.ends_with(TAG),
                    "{width}: {row:?}"
                );
                assert!(
                    unicode_width::UnicodeWidthStr::width(row.as_str()) <= usize::from(width),
                    "{width}"
                );
            }
        }
    }
    assert_eq!(rows(tagged(Footer::new(&PLAIN).open(false)), 10)[1], TAG);
    assert_eq!(tagged(Footer::new(&PLAIN).open(false)).height(9), 0);
}

#[test]
fn a_version_keeps_its_own_v_and_needs_no_name() {
    let line = |footer: Footer| rows(footer.open(false).rule(false), 20)[0].clone();
    assert_eq!(
        line(Footer::new(&PLAIN).version("app", "v1.2.3")),
        format!("{}app v1.2.3", " ".repeat(10))
    );
    assert_eq!(line(footer().version("", "0.4.0")), "ctrl+c twice quit");
    assert_eq!(
        line(Footer::new(&PLAIN).version("", "0.4.0")),
        "              v0.4.0"
    );
    assert_eq!(
        line(Footer::new(&PLAIN).version("ăîșț", "0.4.0")),
        format!("{}ăîșț v0.4.0", " ".repeat(9))
    );
}

#[test]
fn wrapped_hints_flow_around_the_version_on_the_last_row() {
    let footer = tagged(footer()).wrap(true).separator(" • ");
    let bare = footer.version("", "");
    assert_eq!(footer.height(30), 4);
    assert_eq!(
        rows(footer, 30)[1..],
        [
            "↑↓ move • enter open",
            "/ search • s stop • ? help",
            "ctrl+c twice quit   app v0.4.0",
        ]
    );
    assert_eq!(
        rows(footer, 40)[1..],
        [
            "↑↓ move • enter open • / search • s stop",
            "? help • ctrl+c twice quit    app v0.4.0",
        ]
    );
    assert_eq!(bare.height(36), 3);
    assert_eq!(footer.height(36), 4);
    assert_eq!(
        rows(footer, 36)[1..],
        [
            "↑↓ move • enter open • / search",
            "s stop • ? help",
            "ctrl+c twice quit         app v0.4.0",
        ]
    );
    assert_eq!(bare.height(20), 5);
    assert_eq!(footer.height(20), 6);
    assert_eq!(
        rows(footer, 20)[1..],
        [
            "↑↓ move • enter open",
            "/ search • s stop",
            "? help",
            "ctrl+c twice quit",
            "          app v0.4.0",
        ]
    );
    assert_eq!(footer.height(9), bare.height(9));
    assert_eq!(rows(footer, 9), rows(bare, 9));
    assert_eq!(footer.height(100), 2);
    assert_eq!(rows(footer, 100)[1], padded(&LINE.replace('·', "•"), 100));
    for width in 1..=100 {
        for key in KEYS {
            assert_eq!(
                footer.shown(key, width),
                bare.shown(key, width),
                "{key} {width}"
            );
        }
        let drawn = draw(footer, width, footer.height(width));
        assert!(widest(&drawn) <= usize::from(width), "{width}");
    }
}

#[test]
fn the_version_keeps_its_row_when_help_hides_every_hint() {
    let mut help = Help::default();
    help.key(Key::Char('?'));
    let hidden = tagged(Footer::new(&PLAIN).styles(STYLES).help(&help));
    assert_eq!(hidden.version("", "").height(40), 0);
    assert_eq!(hidden.height(40), 2);
    let drawn = draw(hidden, 40, 2);
    assert_eq!(drawn.text[1], padded("", 40), "\n{}", show(&drawn));
    assert_eq!(
        drawn.marks[1],
        format!("{}{}", " ".repeat(30), "m".repeat(10))
    );
    assert!(!hidden.shown("enter", 40));
    assert_eq!(rows(hidden.wrap(true), 40), rows(hidden, 40));
    let pinned = tagged(footer().help(&help));
    assert_eq!(pinned.height(40), 2);
    assert_eq!(rows(pinned, 40)[1], padded("ctrl+c twice quit", 40));
    assert!(pinned.shown("ctrl+c", 40));
    assert_eq!(rows(pinned, 28)[1], "ctrl+c twice quit");
    let noticed = hidden.notice(Some(Notice::legend("✓ done")));
    assert_eq!(rows(noticed, 40)[1..], [padded("", 40).as_str(), "✓ done"]);
}

#[test]
fn an_over_notice_keeps_the_version_on_the_last_row() {
    let text = "The import failed: the file could not be read because another program holds it";
    let footer = tagged(footer()).notice(Some(Notice::alert(text).over()));
    assert_eq!(footer.height(40), 4);
    assert_eq!(
        rows(footer, 40)[1..],
        [
            "The import failed: the file could not be",
            "read because another program holds it",
            padded("ctrl+c twice quit", 40).as_str(),
        ]
    );
    let plain = tagged(Footer::new(&PLAIN).styles(STYLES)).notice(Some(Notice::alert(text).over()));
    assert_eq!(plain.version("", "").height(40), 3);
    assert_eq!(plain.height(40), 4);
    assert_eq!(rows(plain, 40)[3], padded("", 40));
    assert!(!plain.shown("enter", 40));
}

#[test]
fn the_version_stays_beside_a_carried_input() {
    let short = Input::new().with("cargo");
    let bar = InputBar::new("Filter", &short).hint("enter filter · esc cancel");
    let filter = tagged(footer()).input(Some(bar));
    assert_eq!(filter.height(50), 3);
    let drawn = draw(filter, 50, 3);
    assert_eq!(
        drawn.text[1..],
        [
            "Filter cargo",
            padded("  enter filter · esc cancel", 50).as_str()
        ],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(drawn.marks[2].len(), 50);
    assert!(drawn.marks[2].ends_with(&"m".repeat(10)));
    assert_eq!(
        rows(filter, 39)[2],
        padded("  enter filter · esc cancel", 39)
    );
    assert_eq!(rows(filter, 38)[2], "  enter filter · esc cancel");
    let area = Rect::new(0, 0, 50, 3);
    assert_eq!(filter.cursor(area), filter.version("", "").cursor(area));
    let bare = tagged(footer()).input(Some(InputBar::new("Filter", &short)));
    assert_eq!(bare.height(30), 2);
    assert_eq!(rows(bare, 25)[1], padded("Filter cargo", 25));
    assert_eq!(rows(bare, 24)[1], "Filter cargo");
    let long = Input::new().with("cargo build --release --all-features");
    let typed = tagged(footer()).input(Some(InputBar::new("Filter", &long)));
    assert_eq!(
        rows(typed, 40)[1],
        "Filter … build --release --all-features"
    );
}

#[test]
fn the_version_stays_beside_a_carried_confirm() {
    let confirm = Confirm::new();
    let bar = ConfirmBar::new("Stop the import?", &confirm, WORDS);
    let asked = tagged(footer()).confirm(Some(bar));
    assert_eq!(asked.height(60), 3);
    assert_eq!(
        rows(asked, 60)[1..],
        [
            "Stop the import?  Yes   ▸ No",
            padded("  y/n choose · enter accept · esc cancel", 60).as_str(),
        ]
    );
    assert_eq!(
        rows(asked, 51)[2],
        "  y/n choose · enter accept · esc cancel"
    );
    let inline = tagged(footer()).confirm(Some(bar.inline(true)));
    assert_eq!(inline.height(80), 2);
    assert_eq!(
        rows(inline, 80)[1],
        padded(
            "Stop the import?  Yes   ▸ No  y/n choose · enter accept · esc cancel",
            80
        )
    );
    let bare = tagged(footer()).confirm(Some(ConfirmBar::new(
        "Stop the import?",
        &confirm,
        Words::new("Yes", "No"),
    )));
    assert_eq!(bare.height(60), 2);
    assert_eq!(
        rows(bare, 60)[1],
        padded("Stop the import?  Yes   ▸ No", 60)
    );
}

fn guarded(guard: &QuitGuard) -> Footer<'_> {
    tagged(footer())
        .notice(guard.notice().map(Notice::accent))
        .confirm(guard.bar(WORDS))
}

#[test]
fn the_quit_guard_keeps_the_version() {
    let start = Instant::now();
    let mut guard =
        QuitGuard::new(Wording::new("ctrl+c again to quit").ask("2 imports running. Quit anyway?"))
            .mode(Mode::Ask);
    assert_eq!(guard.key(Key::Ctrl('c'), start, true), Guard::Held);
    let armed = guarded(&guard);
    assert_eq!(armed.height(90), 3);
    let drawn = draw(armed, 90, 3);
    assert_eq!(
        drawn.text[1..],
        [padded(LINE, 90).as_str(), "ctrl+c again to quit"],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(drawn.marks[2], "A".repeat(20));
    assert_eq!(guard.key(Key::Ctrl('c'), start, true), Guard::Held);
    assert!(guard.asking().is_some());
    let asking = guarded(&guard);
    assert_eq!(
        rows(asking, 90)[1..],
        [
            "2 imports running. Quit anyway?  Yes   ▸ No",
            padded("  y/n choose · enter accept · esc cancel", 90).as_str(),
        ]
    );
}

#[test]
fn a_status_bar_puts_the_version_right_of_every_segment() {
    let status = tagged(Footer::status(&SEGMENTS).styles(STYLES));
    let bare = status.version("", "");
    let line = |footer: Footer, width: u16| draw(footer, width, 1).text[0].clone();
    assert_eq!(
        line(status, 120),
        format!(" NORMAL  {LINE}{}12:30  {TAG}", " ".repeat(25))
    );
    assert_eq!(line(status, 96), format!(" NORMAL  {LINE} 12:30  {TAG}"));
    assert_eq!(line(status, 95), line(bare, 95));
    assert_eq!(
        line(status, 95),
        format!(" NORMAL  {LINE}{}12:30", " ".repeat(12))
    );
    let drawn = draw(status, 120, 1);
    assert!(drawn.marks[0].ends_with(&format!("ggggg  {}", "m".repeat(10))));
    for width in 0..=130 {
        assert_eq!(status.height(width), bare.height(width), "{width}");
        for key in KEYS {
            assert_eq!(
                status.shown(key, width),
                bare.shown(key, width),
                "{key} {width}"
            );
        }
        if width < 96 {
            assert_eq!(rows(status, width), rows(bare, width), "{width}");
        }
    }
}
