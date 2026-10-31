use crate::common::{STYLES, draw, show, widest};
use pito_tui::footer::{Footer, Help, Hint, Key, Notice, Segment, Styles, Tone};
use ratatui::style::{Color, Style};

const HINTS: [Hint; 6] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "open").rank(1),
    Hint::new("/", "search").rank(3),
    Hint::new("s", "stop").rank(2),
    Hint::new("?", "help").rank(4),
    Hint::new("ctrl+c", "twice quit").pinned(),
];

fn footer() -> Footer<'static> {
    Footer::new(&HINTS).styles(STYLES)
}

#[test]
fn hints_draw_key_in_accent_and_label_muted_under_a_rule() {
    let footer = footer().notice(Some(Notice::legend("✓ done · ✗ failed")));
    assert_eq!(footer.height(90), 3);
    let drawn = draw(footer, 90, 3);
    assert_eq!(
        drawn.text,
        [
            "─".repeat(90).as_str(),
            "↑↓ move · enter open · / search · s stop · ? help · ctrl+c twice quit",
            "✓ done · ✗ failed",
        ],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(drawn.marks[0], "r".repeat(90));
    assert_eq!(
        drawn.marks[1],
        "AAmmmmmmmmAAAAAmmmmmmmmAmmmmmmmmmmAmmmmmmmmAmmmmmmmmAAAAAAmmmmmmmmmmm"
    );
    assert_eq!(drawn.marks[2], "m".repeat(17));
}

#[test]
fn the_least_important_hints_drop_first_and_order_is_kept() {
    let line = |width: u16| draw(footer().rule(false), width, 1).text[0].clone();
    assert_eq!(
        line(69),
        "↑↓ move · enter open · / search · s stop · ? help · ctrl+c twice quit"
    );
    assert_eq!(
        line(68),
        "↑↓ move · enter open · / search · s stop · ctrl+c twice quit"
    );
    assert_eq!(
        line(59),
        "↑↓ move · enter open · s stop · ctrl+c twice quit"
    );
    assert_eq!(line(48), "↑↓ move · enter open · ctrl+c twice quit");
    assert_eq!(line(39), "↑↓ move · ctrl+c twice quit");
    assert_eq!(line(26), "ctrl+c twice quit");
    assert_eq!(line(10), "ctrl+c tw…");
}

#[test]
fn ties_drop_the_later_hint() {
    let hints = [
        Hint::new("a", "one"),
        Hint::new("b", "two"),
        Hint::new("c", "three"),
    ];
    let drawn = draw(Footer::new(&hints).rule(false), 16, 1);
    assert_eq!(drawn.text[0], "a one · b two");
}

#[test]
fn help_hides_all_but_pinned_hints() {
    let mut help = Help::default();
    assert!(help.open());
    assert!(!help.key(Key::Char('h')));
    assert!(help.key(Key::Char('?')));
    assert!(!help.open());
    let closed = footer().help(&help);
    assert_eq!(closed.height(80), 2);
    assert_eq!(draw(closed, 80, 2).text[1], "ctrl+c twice quit");
    let bare = [Hint::new("enter", "open")];
    assert_eq!(Footer::new(&bare).open(false).height(80), 0);
    let notice = Footer::new(&bare)
        .open(false)
        .notice(Some(Notice::accent("ctrl+c again to quit")));
    assert_eq!(notice.height(80), 2);
    let help = Help::new(false).toggle_on(Key::Char('h'));
    assert!(help.wants(Key::Char('h')));
    assert!(!help.wants(Key::Char('?')));
}

#[test]
fn wrap_mode_flows_every_hint_over_lines() {
    let footer = footer().wrap(true).separator(" • ");
    assert_eq!(footer.height(30), 4);
    let drawn = draw(footer, 30, 4);
    assert_eq!(
        drawn.text[1..],
        [
            "↑↓ move • enter open",
            "/ search • s stop • ? help",
            "ctrl+c twice quit"
        ],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(footer.height(100), 2);
}

#[test]
fn notice_tones_use_the_app_styles() {
    let tones = [
        (Tone::Ink, 'i'),
        (Tone::Muted, 'm'),
        (Tone::Accent, 'A'),
        (Tone::Alert, 'x'),
    ];
    for (tone, mark) in tones {
        let footer = Footer::new(&[])
            .styles(STYLES)
            .notice(Some(Notice::new("Saved", tone)));
        let drawn = draw(footer, 10, 2);
        assert_eq!(drawn.text[1], "Saved");
        assert_eq!(drawn.marks[1], mark.to_string().repeat(5), "{tone:?}");
    }
}

#[test]
fn a_good_notice_falls_back_to_ink_until_its_style_is_set() {
    let notice = Some(Notice::good("Saved"));
    let footer = Footer::new(&[]).styles(STYLES).notice(notice);
    assert_eq!(Notice::good("Saved").tone, Tone::Good);
    assert_eq!(STYLES.good, None);
    assert_eq!(draw(footer, 10, 2).marks[1], "iiiii");
    let green = STYLES.good(Style::new().fg(Color::Green));
    let footer = footer.styles(green);
    assert_eq!(draw(footer, 10, 2).marks[1], "ggggg");
}

#[test]
fn an_indent_insets_the_hints_and_the_notice_but_not_the_rule() {
    let footer = footer().indent(2).notice(Some(Notice::legend("✓ done")));
    assert_eq!(footer.height(42), 3);
    let drawn = draw(footer, 42, 3);
    assert_eq!(
        drawn.text,
        [
            "─".repeat(42).as_str(),
            "  ↑↓ move · enter open · ctrl+c twice quit",
            "  ✓ done",
        ],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(drawn.marks[0], "r".repeat(42));
    let line = |indent: u16, width: u16| {
        draw(footer.indent(indent).rule(false).notice(None), width, 1).text[0].clone()
    };
    assert_eq!(line(0, 48), "↑↓ move · enter open · ctrl+c twice quit");
    assert_eq!(line(2, 48), "  ↑↓ move · enter open · ctrl+c twice quit");
    assert_eq!(line(2, 41), "  ↑↓ move · ctrl+c twice quit");
    assert!(footer.shown("enter", 42));
    assert!(!footer.shown("enter", 41));
    for width in 1..=80 {
        let drawn = draw(footer, width, 3);
        assert!(widest(&drawn) <= usize::from(width), "{width}");
    }
}

#[test]
fn an_indent_wraps_in_the_inset_width() {
    let footer = footer().wrap(true).separator(" • ").indent(10);
    assert_eq!(footer.height(40), 4);
    assert_eq!(footer.indent(0).height(40), 3);
    let drawn = draw(footer, 40, 4);
    assert_eq!(
        drawn.text,
        [
            "─".repeat(40).as_str(),
            "          ↑↓ move • enter open",
            "          / search • s stop • ? help",
            "          ctrl+c twice quit",
        ],
        "\n{}",
        show(&drawn)
    );
}

#[test]
fn a_long_notice_over_the_hints_wraps_in_their_place() {
    let text = "The import failed: the file could not be read because another program holds it";
    let footer = footer().notice(Some(Notice::alert(text).over()));
    assert_eq!(footer.height(40), 4);
    let drawn = draw(footer, 40, 4);
    assert_eq!(
        drawn.text[1..],
        [
            "The import failed: the file could not be",
            "read because another program holds it",
            "ctrl+c twice quit",
        ],
        "\n{}",
        show(&drawn)
    );
    assert!(!footer.shown("enter", 40));
    let drawn = draw(footer, 30, 4);
    assert_eq!(
        drawn.text[1..],
        [
            "The import failed: the file",
            "could not be read because…",
            "ctrl+c twice quit",
        ],
        "\n{}",
        show(&drawn)
    );
    let short = footer.notice(Some(Notice::alert("Import failed").over()));
    assert_eq!(
        draw(short, 40, 3).text[1..],
        ["↑↓ move · enter open · ctrl+c twice quit", "Import failed"]
    );
}

#[test]
fn a_pinned_hint_stays_under_an_over_notice() {
    let text = "The import failed: the file could not be read because another program holds it";
    let footer = footer().notice(Some(Notice::alert(text).over()));
    for width in [30, 40] {
        assert!(footer.shown("ctrl+c", width), "{width}");
        assert!(!footer.shown("enter", width), "{width}");
        assert!(!footer.shown("x", width), "{width}");
        let rows = footer.height(width);
        let drawn = draw(footer, width, rows);
        assert_eq!(drawn.text.last().unwrap(), "ctrl+c twice quit", "{width}");
        assert_eq!(drawn.marks.last().unwrap(), "AAAAAAmmmmmmmmmmm", "{width}");
    }
    assert!(!footer.open(false).notice(None).shown("enter", 40));
    assert!(footer.open(false).shown("ctrl+c", 40));
}

#[test]
fn an_over_notice_without_a_pinned_hint_still_takes_every_row() {
    const PLAIN: [Hint; 2] = [Hint::new("↑↓", "move"), Hint::new("enter", "open")];
    let text = "The import failed: the file could not be read because another program holds it";
    let footer = Footer::new(&PLAIN)
        .styles(STYLES)
        .notice(Some(Notice::alert(text).over()));
    assert_eq!(footer.height(40), 3);
    assert!(!footer.shown("enter", 40));
    let drawn = draw(footer, 40, 3);
    assert_eq!(
        drawn.text[1..],
        [
            "The import failed: the file could not be",
            "read because another program holds it",
        ],
        "\n{}",
        show(&drawn)
    );
}

#[test]
fn keys_follow_display() {
    let footer = footer();
    assert!(footer.shown("/", 80));
    assert!(!footer.shown("/", 50));
    assert!(footer.shown("s", 50));
    assert!(footer.shown("ctrl+c", 20));
    assert!(!footer.shown("x", 80));
    assert!(!footer.open(false).shown("s", 80));
    assert!(footer.open(false).shown("ctrl+c", 80));
    assert!(footer.wrap(true).shown("/", 20));
}

#[test]
fn status_segments_drop_and_shrink_by_rank() {
    const WHITE: Style = Style::new().fg(Color::White);
    const GREEN: Style = Style::new().fg(Color::Green);
    const MAGENTA: Style = Style::new().fg(Color::Magenta);
    const GRAY: Style = Style::new().fg(Color::DarkGray);
    let segments = [
        Segment::text(" NORMAL ", WHITE),
        Segment::text("3 selected of 40 items", GRAY)
            .shrink()
            .rank(1),
        Segment::hints(&HINTS).rank(5),
        Segment::text("12:30", GREEN).right().rank(3),
        Segment::text("page 1/2", MAGENTA).right().rank(2),
    ];
    let status = Footer::status(&segments).styles(STYLES);
    let line = |width: u16| draw(status, width, 1).text[0].clone();
    assert_eq!(status.height(120), 1);
    assert_eq!(
        line(120),
        " NORMAL  3 selected of 40 items ↑↓ move · enter open · / search · s stop · ? help · ctrl+c twice quit     12:30 page 1/2"
    );
    assert_eq!(
        line(80),
        " NORMAL  3 selected of 40 items ↑↓ move · ctrl+c twice quit       12:30 page 1/2"
    );
    assert_eq!(
        line(50),
        " NORMAL  3 selec… ctrl+c twice quit 12:30 page 1/2"
    );
    assert_eq!(line(30), " NORMAL  3 sel… 12:30 page 1/2");
    assert_eq!(line(8), " NORMAL");
    for width in 1..=130 {
        assert!(
            widest(&draw(status, width, 1)) <= usize::from(width),
            "{width}"
        );
    }
}

#[test]
fn every_width_fits_and_clips_by_cell() {
    let hints = [
        Hint::new("↑↓", "mută"),
        Hint::new("enter", "deschide").rank(1),
        Hint::new("ș", "șterge înregistrarea").rank(2),
        Hint::new("ctrl+c ctrl+c", "ieșire").pinned(),
    ];
    let footer = Footer::new(&hints).styles(STYLES).separator(" • ");
    assert_eq!(
        draw(footer, 80, 2).text[1],
        "↑↓ mută • enter deschide • ș șterge înregistrarea • ctrl+c ctrl+c ieșire"
    );
    assert_eq!(
        draw(footer, 40, 2).text[1],
        "↑↓ mută • ctrl+c ctrl+c ieșire"
    );
    assert_eq!(draw(footer, 15, 2).text[1], "ctrl+c ctrl+c …");
    for width in 1..=90 {
        for wrap in [false, true] {
            let footer = footer.wrap(wrap);
            let drawn = draw(footer, width, footer.height(width).max(1));
            assert!(widest(&drawn) <= usize::from(width), "{width} {wrap}");
        }
    }
    let wide = [Hint::new("日本", "語のテキスト")];
    assert_eq!(
        draw(Footer::new(&wide).rule(false), 9, 1).text[0],
        "日本 語…"
    );
}

#[test]
fn a_lead_word_comes_before_the_key() {
    let hints = [Hint::new("ctrl+c", "again to quit").lead("Press")];
    let drawn = draw(Footer::new(&hints).styles(STYLES).rule(false), 40, 1);
    assert_eq!(drawn.text[0], "Press ctrl+c again to quit");
    assert_eq!(drawn.marks[0], "mmmmmmAAAAAAmmmmmmmmmmmmmm");
}

#[test]
fn a_status_text_carries_several_styled_spans() {
    const GREEN: Style = Style::new().fg(Color::Green);
    const GRAY: Style = Style::new().fg(Color::DarkGray);
    const MAGENTA: Style = Style::new().fg(Color::Magenta);
    let progress = [("copied ", GRAY), ("42%", GREEN), (" of 3 files", GRAY)];
    let dots = [("●", MAGENTA), ("○○", GRAY)];
    let segments = [
        Segment::spans(&progress).shrink(),
        Segment::spans(&dots).right().rank(1),
        Segment::text("v2", GRAY).right().rank(2),
    ];
    let status = Footer::status(&segments).styles(STYLES);
    let drawn = draw(status, 40, 1);
    assert_eq!(drawn.text[0], "copied 42% of 3 files             ●○○ v2");
    assert_eq!(drawn.marks[0], "mmmmmmmgggmmmmmmmmmmm             amm mm");
    let drawn = draw(status, 17, 1);
    assert_eq!(drawn.text[0], "copied…    ●○○ v2");
    assert_eq!(drawn.marks[0], "mmmmmmm    amm mm");
    let drawn = draw(status, 15, 1);
    assert_eq!(drawn.text[0], "copied…  ●○○ v2");
    assert_eq!(drawn.marks[0], "mmmmmmm  amm mm");
    assert_eq!(draw(status, 3, 1).text[0], "co…");
    let empty = [("", GRAY)];
    assert_eq!(Footer::status(&[Segment::spans(&empty)]).height(40), 0);
    for width in 1..=50 {
        assert!(
            widest(&draw(status, width, 1)) <= usize::from(width),
            "{width}"
        );
    }
}

#[test]
fn the_gap_moves_the_hints_that_follow_a_segment() {
    const QUIT: [Hint; 1] = [Hint::new("q", "quit")];
    let segments = [
        Segment::text("main", Style::new().fg(Color::White)),
        Segment::hints(&QUIT),
    ];
    let row = |footer: Footer, width: u16| draw(footer, width, 1).text.remove(0);
    let status = Footer::status(&segments).styles(STYLES);
    assert_eq!(row(status, 30), "main q quit");
    assert_eq!(row(status.gap(3), 30), "main   q quit");
    assert_eq!(row(status.gap(0), 30), "mainq quit");
    assert_eq!(row(status.gap(1), 12), "main q quit");
    assert_eq!(row(status.gap(3), 12), "main   q qu…");
    let right = [
        Segment::text("main", Style::new().fg(Color::White)).right(),
        Segment::hints(&QUIT).right(),
    ];
    let status = Footer::status(&right).styles(STYLES);
    assert_eq!(row(status, 20), "         main q quit");
    assert_eq!(row(status.gap(4), 20), "      main    q quit");
    let tagged = Footer::status(&segments[1..])
        .styles(STYLES)
        .version("app", "1.0");
    assert_eq!(row(tagged, 16), "q quit  app v1.0");
    assert_eq!(row(tagged.gap(4), 16), "q quit");
    assert_eq!(row(tagged.gap(4), 18), "q quit    app v1.0");
    let wrapped = Footer::new(&QUIT)
        .rule(false)
        .wrap(true)
        .version("app", "1.0");
    assert_eq!(wrapped.height(16), 1);
    assert_eq!(draw(wrapped, 16, 1).text, ["q quit  app v1.0"]);
    assert_eq!(wrapped.gap(4).height(16), 2);
    assert_eq!(
        draw(wrapped.gap(4), 16, 2).text,
        ["q quit", "        app v1.0"]
    );
}

#[test]
fn the_alert_style_colours_an_alert_notice_and_nothing_else() {
    const QUIT: [Hint; 1] = [Hint::new("q", "quit")];
    let red = Style::new().fg(Color::Red);
    let base = Styles::new()
        .accent(Style::new().fg(Color::Magenta))
        .muted(Style::new().fg(Color::DarkGray));
    let footer = Footer::new(&QUIT)
        .rule(false)
        .styles(base.alert(red))
        .notice(Some(Notice::alert("boom")));
    assert_eq!(footer.height(20), 2);
    let drawn = draw(footer, 20, 2);
    assert_eq!(drawn.text, ["q quit", "boom"], "\n{}", show(&drawn));
    assert_eq!(drawn.marks, ["Ammmmm", "xxxx"], "\n{}", show(&drawn));
    let plain = draw(footer.styles(base), 20, 2);
    assert_eq!(plain.text, ["q quit", "boom"], "\n{}", show(&plain));
    assert_eq!(plain.marks, ["Ammmmm", ""], "\n{}", show(&plain));
    let legend = draw(footer.notice(Some(Notice::legend("boom"))), 20, 2);
    assert_eq!(legend.marks, ["Ammmmm", "mmmm"], "\n{}", show(&legend));
}

#[test]
fn closed_hints_with_no_room_for_the_tag_take_no_line_and_show_no_key() {
    const QUIT: [Hint; 1] = [Hint::new("q", "quit")];
    let open = Footer::new(&QUIT).rule(false).version("app", "1.0.0");
    assert!(open.shown("q", 40));
    let closed = open.open(false);
    assert_eq!(closed.height(5), 0);
    assert!(!closed.shown("q", 5));
    assert!(!closed.shown("q", 40));
    assert_eq!(closed.height(40), 1);
    assert_eq!(draw(closed, 5, 1).text, [""]);
    assert_eq!(draw(closed, 20, 1).text, ["          app v1.0.0"]);
}

#[test]
fn a_cut_label_keeps_whole_words_and_ends_in_an_ellipsis() {
    let hints = [Hint::new("q", "quit the whole program now")];
    let row = |width: u16| {
        draw(Footer::new(&hints).rule(false), width, 1)
            .text
            .remove(0)
    };
    assert_eq!(row(40), "q quit the whole program now");
    assert_eq!(row(16), "q quit the…");
    assert_eq!(row(12), "q quit the…");
    assert_eq!(row(9), "q quit…");
    assert_eq!(row(4), "q q…");
}

#[test]
fn a_cut_legend_drops_whole_parts_between_separators() {
    let row = |width: u16| {
        let footer = Footer::new(&[])
            .rule(false)
            .notice(Some(Notice::legend("build · deploy api · tests")));
        draw(footer, width, 1).text.remove(0)
    };
    assert_eq!(row(26), "build · deploy api · tests");
    assert_eq!(row(22), "build · deploy api…");
    assert_eq!(row(12), "build…");
    assert_eq!(row(5), "buil…");
}

#[test]
fn a_cut_legend_of_one_long_word_keeps_its_start() {
    let row = |width: u16| {
        let footer = Footer::new(&[])
            .rule(false)
            .notice(Some(Notice::legend("household@example.com")));
        draw(footer, width, 1).text.remove(0)
    };
    assert_eq!(row(21), "household@example.com");
    assert_eq!(row(20), "household@example.c…");
    assert_eq!(row(2), "h…");
    assert_eq!(row(1), "…");
}
