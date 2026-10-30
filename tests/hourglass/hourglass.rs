use std::time::Duration;

use pito_tui::hourglass::{CYCLE, FRAMES, GLASS_ROWS, Hourglass, WIDTH, band_at, frame_at};
use ratatui::{
    Terminal,
    backend::TestBackend,
    buffer::{Buffer, Cell, CellWidth},
    layout::{Position, Rect},
    style::{Color, Modifier, Style},
    widgets::Widget,
};
use unicode_width::UnicodeWidthStr;

const GLASS: Style = Style::new().fg(Color::Yellow);
const ACCENT: Style = Style::new().fg(Color::Magenta);
const MUTED: Style = Style::new().fg(Color::DarkGray).add_modifier(Modifier::DIM);
const LABEL: &str = "Loading...";
const HINT: &str = "esc stops waiting";

fn ms(n: u64) -> Duration {
    Duration::from_millis(n)
}

fn styled(elapsed: Duration) -> Hourglass<'static> {
    Hourglass::new(elapsed)
        .glass(GLASS)
        .accent(ACCENT)
        .muted(MUTED)
}

fn hourglass(elapsed: Duration) -> Hourglass<'static> {
    styled(elapsed).label(LABEL).hint(Some(HINT))
}

struct Drawn {
    text: Vec<String>,
    marks: Vec<String>,
}

impl Drawn {
    fn blank(&self) -> bool {
        self.text.iter().all(String::is_empty) && self.marks.iter().all(String::is_empty)
    }
}

fn draw(widget: Hourglass<'_>, width: u16, height: u16, area: Rect) -> Drawn {
    let mut terminal = Terminal::new(TestBackend::new(width, height)).unwrap();
    terminal
        .draw(|frame| frame.render_widget(widget, area))
        .unwrap();
    read(terminal.backend().buffer())
}

fn whole(widget: Hourglass<'_>, width: u16, height: u16) -> Drawn {
    draw(widget, width, height, Rect::new(0, 0, width, height))
}

fn mark(fg: Color, modifier: Modifier) -> char {
    match (fg, modifier) {
        (Color::Reset, m) if m.is_empty() => ' ',
        (Color::Yellow, m) if m.is_empty() => 'g',
        (Color::Magenta, Modifier::BOLD) => 'A',
        (Color::Magenta, m) if m.is_empty() => 'a',
        (Color::DarkGray, Modifier::DIM) => 'm',
        _ => '?',
    }
}

fn read(buffer: &Buffer) -> Drawn {
    let width = usize::from(buffer.area.width).max(1);
    let mut text = Vec::new();
    let mut marks = Vec::new();
    for row in buffer.content.chunks(width) {
        let mut line = String::new();
        let mut hidden = 0;
        for cell in row {
            if hidden > 0 {
                hidden -= 1;
                continue;
            }
            line.push_str(cell.symbol());
            hidden = cell.symbol().cell_width().saturating_sub(1);
        }
        let marked: String = row
            .iter()
            .map(|cell| mark(cell.fg, cell.modifier))
            .collect();
        text.push(line.trim_end().to_string());
        marks.push(marked.trim_end().to_string());
    }
    Drawn { text, marks }
}

fn glass_marks() -> String {
    format!("     {}", "g".repeat(11))
}

#[test]
fn every_frame_is_eleven_cells_wide_and_six_rows_high() {
    assert_eq!(FRAMES.len(), 13);
    assert_eq!(WIDTH, 11);
    assert_eq!(GLASS_ROWS, 6);
    for frame in FRAMES {
        assert_eq!(frame.len(), 6);
        for line in frame {
            assert_eq!(line.width(), 11, "{line}");
            assert_eq!(line.chars().count(), 11, "{line}");
        }
    }
    assert_eq!(FRAMES[0], FRAMES[12]);
}

#[test]
fn the_frame_follows_the_elapsed_time() {
    let expected = [
        (0, 0),
        (428, 0),
        (429, 1),
        (2999, 6),
        (3000, 7),
        (3199, 7),
        (3200, 8),
        (4199, 12),
        (4200, 0),
        (4200 + 3000, 7),
        (8400 + 429, 1),
    ];
    for (at, frame) in expected {
        assert_eq!(frame_at(ms(at)), frame, "at {at} ms");
    }
}

#[test]
fn every_frame_shows_for_its_share_of_the_cycle() {
    let mut seen = [0u32; 13];
    for at in 0..4200 {
        seen[frame_at(ms(at))] += 1;
    }
    assert_eq!(&seen[..7], [429, 429, 428, 429, 428, 429, 428]);
    assert_eq!(&seen[7..], [200; 6]);
}

#[test]
fn the_cycle_is_the_frames_total() {
    let mut runs = Vec::new();
    let mut at = 0;
    let mut start = 0;
    let mut frame = frame_at(ms(0));
    loop {
        at += 1;
        let next = frame_at(ms(at));
        if next != frame {
            runs.push((frame, at - start));
            start = at;
            frame = next;
            if frame == 0 {
                break;
            }
        }
    }
    assert_eq!(runs.len(), FRAMES.len());
    assert!(runs.iter().map(|(frame, _)| *frame).eq(0..FRAMES.len()));
    let total: u64 = runs.iter().map(|(_, length)| length).sum();
    assert_eq!(CYCLE, ms(total));
    assert_eq!(CYCLE, ms(4200));
    assert_eq!(frame_at(CYCLE), 0);
    assert_eq!(frame_at(CYCLE - ms(1)), FRAMES.len() - 1);
}

#[test]
fn the_band_follows_the_elapsed_time() {
    let expected = [
        (0, None),
        (200, Some(0)),
        (600, Some(5)),
        (1199, None),
        (1200, None),
        (1499, None),
        (1500, None),
        (1500 + 200, Some(0)),
        (3000 + 600, Some(5)),
    ];
    for (at, band) in expected {
        assert_eq!(band_at(ms(at), 10), band, "at {at} ms");
    }
}

#[test]
fn the_band_enters_at_the_first_character_and_leaves_after_the_last() {
    assert_eq!(band_at(ms(171), 10), None);
    assert_eq!(band_at(ms(172), 10), Some(0));
    assert_eq!(band_at(ms(257), 10), Some(0));
    assert_eq!(band_at(ms(258), 10), Some(1));
    assert_eq!(band_at(ms(942), 10), Some(8));
    assert_eq!(band_at(ms(943), 10), Some(9));
    assert_eq!(band_at(ms(1028), 10), Some(9));
    assert_eq!(band_at(ms(1029), 10), None);
    let visible: Vec<usize> = (0..1500).filter_map(|at| band_at(ms(at), 10)).collect();
    assert_eq!(visible.first(), Some(&0));
    assert_eq!(visible.last(), Some(&9));
    assert!(
        visible
            .windows(2)
            .all(|pair| pair[1] == pair[0] || pair[1] == pair[0] + 1)
    );
}

#[test]
fn an_empty_label_has_no_band() {
    assert!((0..1500).all(|at| band_at(ms(at), 0).is_none()));
    assert_eq!(band_at(ms(479), 1), None);
    assert_eq!(band_at(ms(480), 1), Some(0));
}

#[test]
fn the_full_layout_at_the_start_of_a_cycle() {
    let drawn = whole(hourglass(ms(0)), 21, 10);
    assert_eq!(
        drawn.text,
        [
            "",
            "        ╭───╮",
            "        │⣿⣿⣿│",
            "        ╰╮⣿╭╯",
            "        ╭╯ ╰╮",
            "        │   │",
            "        ╰───╯",
            "     Loading...",
            "  esc stops waiting",
            "",
        ]
    );
    let glass = glass_marks();
    let glass = glass.as_str();
    assert_eq!(
        drawn.marks,
        [
            "",
            glass,
            glass,
            glass,
            glass,
            glass,
            glass,
            "     mmmmmmmmmm",
            "  mmmmmmmmmmmmmmmmm",
            "",
        ]
    );
    assert_eq!(band_at(ms(0), LABEL.len()), None);
}

#[test]
fn the_full_layout_while_the_glass_flips() {
    let drawn = whole(hourglass(ms(3300)), 21, 10);
    assert_eq!(frame_at(ms(3300)), 8);
    assert_eq!(
        drawn.text,
        [
            "",
            "     ╭─",
            "     │ ──╮",
            "     ╰─   ⣠╭──",
            "       ──╯ ⣾⣿⣿─╮",
            "           ╰──⣿│",
            "              ─╯",
            "     Loading...",
            "  esc stops waiting",
            "",
        ]
    );
    let glass = glass_marks();
    let glass = glass.as_str();
    assert_eq!(
        drawn.marks,
        [
            "",
            glass,
            glass,
            glass,
            glass,
            glass,
            glass,
            "     aAammmmmmm",
            "  mmmmmmmmmmmmmmmmm",
            "",
        ]
    );
    assert_eq!(band_at(ms(3300), LABEL.len()), Some(1));
}

#[test]
fn the_full_layout_mid_sweep() {
    let drawn = whole(hourglass(ms(600)), 21, 10);
    assert_eq!(drawn.text[1], "        ╭───╮");
    assert_eq!(drawn.text[4], "        ╭╯⡇╰╮");
    assert_eq!(drawn.marks[7], "     mmmmaAammm");
    assert_eq!(drawn.marks[8], "  mmmmmmmmmmmmmmmmm");
}

#[test]
fn the_glass_shows_the_frame_for_the_elapsed_time() {
    for at in (0..4200).step_by(50) {
        let drawn = whole(hourglass(ms(at)), 21, 10);
        let frame = FRAMES[frame_at(ms(at))];
        for (row, line) in frame.iter().enumerate() {
            let expected = format!("     {line}");
            assert_eq!(drawn.text[row + 1], expected.trim_end(), "at {at} ms");
        }
    }
}

#[test]
fn the_accent_follows_band_at_and_the_hint_never_shimmers() {
    for at in (0..3000).step_by(10) {
        let drawn = whole(hourglass(ms(at)), 21, 10);
        let label = format!("{:<15}", drawn.marks[7]);
        let cells: Vec<char> = label.chars().skip(5).take(10).collect();
        let bold: Vec<usize> = (0..10).filter(|i| cells[*i] == 'A').collect();
        assert_eq!(
            bold,
            band_at(ms(at), 10).into_iter().collect::<Vec<_>>(),
            "at {at} ms"
        );
        if let Some(centre) = band_at(ms(at), 10) {
            for near in [centre.wrapping_sub(1), centre + 1] {
                if near < 10 {
                    assert_eq!(cells[near], 'a', "at {at} ms");
                }
            }
        }
        let lit = cells.iter().filter(|cell| **cell != 'm').count();
        assert!(lit <= 3, "at {at} ms: {label}");
        assert_eq!(drawn.marks[8], "  mmmmmmmmmmmmmmmmm", "at {at} ms");
    }
}

#[test]
fn the_band_clips_at_both_ends_of_the_label() {
    let entering = whole(hourglass(ms(100)), 21, 10);
    assert_eq!(entering.marks[7], "     ammmmmmmmm");
    let leaving = whole(hourglass(ms(1050)), 21, 10);
    assert_eq!(leaving.marks[7], "     mmmmmmmmma");
    for at in [1200, 1350, 1499] {
        let paused = whole(hourglass(ms(at)), 21, 10);
        assert_eq!(paused.marks[7], "     mmmmmmmmmm", "at {at} ms");
    }
}

#[test]
fn the_block_centres_in_an_area_away_from_the_corner() {
    let corner = whole(hourglass(ms(3300)), 21, 10);
    let away = draw(hourglass(ms(3300)), 25, 12, Rect::new(2, 1, 21, 10));
    assert_eq!(away.text[0], "");
    assert_eq!(away.text[11], "");
    for row in 0..10 {
        let shift = |line: &String| {
            if line.is_empty() {
                String::new()
            } else {
                format!("  {line}")
            }
        };
        assert_eq!(away.text[row + 1], shift(&corner.text[row]));
        assert_eq!(away.marks[row + 1], shift(&corner.marks[row]));
    }
}

#[test]
fn without_a_hint_seven_rows_hold_the_glass() {
    let drawn = whole(styled(ms(0)).label(LABEL), 30, 7);
    assert_eq!(drawn.text[0], "            ╭───╮");
    assert_eq!(drawn.text[5], "            ╰───╯");
    assert_eq!(drawn.text[6], "          Loading...");
    let short = whole(styled(ms(0)).label(LABEL), 30, 6);
    assert!(!short.text.concat().contains('╭'));
    assert_eq!(short.text[2], "          Loading...");
    let empty = whole(styled(ms(0)).label(LABEL).hint(Some("")), 30, 7);
    assert_eq!(empty.text, drawn.text);
}

#[test]
fn a_narrow_area_drops_the_glass_and_keeps_the_shimmer() {
    let drawn = whole(hourglass(ms(600)), 10, 4);
    assert_eq!(drawn.text, ["", "Loading...", "esc stops", ""]);
    assert_eq!(drawn.marks, ["", "mmmmaAammm", "mmmmmmmmmm", ""]);
    let tall = whole(hourglass(ms(3300)), 10, 30);
    assert!(!tall.text.concat().contains('╭'));
    assert_eq!(tall.text[14], "Loading...");
    assert_eq!(tall.marks[14], "aAammmmmmm");
    assert_eq!(tall.text[15], "esc stops");
}

#[test]
fn a_short_area_drops_the_glass() {
    let drawn = whole(hourglass(ms(3300)), 30, 7);
    assert_eq!(
        drawn.text,
        [
            "",
            "",
            "          Loading...",
            "      esc stops waiting",
            "",
            "",
            "",
        ]
    );
    assert_eq!(drawn.marks[2], "          aAammmmmmm");
    assert_eq!(drawn.marks[3], "      mmmmmmmmmmmmmmmmm");
    let fits = whole(hourglass(ms(3300)), 30, 8);
    assert!(fits.text.concat().contains('╭'));
}

#[test]
fn one_row_holds_the_label_alone() {
    let drawn = whole(hourglass(ms(600)), 30, 1);
    assert_eq!(drawn.text, ["          Loading..."]);
    assert_eq!(drawn.marks, ["          mmmmaAammm"]);
    let inside = draw(hourglass(ms(600)), 30, 3, Rect::new(0, 0, 30, 1));
    assert_eq!(inside.text, ["          Loading...", "", ""]);
    assert_eq!(inside.marks, ["          mmmmaAammm", "", ""]);
}

#[test]
fn a_zero_area_draws_nothing() {
    for area in [
        Rect::new(0, 0, 0, 0),
        Rect::new(0, 0, 0, 4),
        Rect::new(0, 0, 10, 0),
        Rect::new(3, 2, 0, 0),
    ] {
        assert!(draw(hourglass(ms(600)), 10, 4, area).blank(), "{area:?}");
    }
}

#[test]
fn a_long_label_is_cut_at_a_character() {
    let long = "Waiting for the long report";
    let drawn = whole(styled(ms(600)).label(long), 12, 1);
    assert_eq!(drawn.text, ["Waiting for"]);
    assert_eq!(drawn.marks, ["mmmmmaAammmm"]);
    assert_eq!(band_at(ms(600), 12), Some(6));
    let glass = whole(styled(ms(0)).label(long).hint(Some(HINT)), 11, 8);
    assert_eq!(glass.text[0], "   ╭───╮");
    assert_eq!(glass.text[6], "Waiting for");
    assert_eq!(glass.text[7], "esc stops w");
}

#[test]
fn a_wide_glyph_is_never_split() {
    let label = "日本語のラベル";
    assert_eq!(whole(styled(ms(0)).label(label), 5, 1).text, ["日本"]);
    assert_eq!(whole(styled(ms(0)).label(label), 6, 1).text, ["日本語"]);
    assert_eq!(whole(styled(ms(0)).label(label), 3, 1).text, ["日"]);
    assert!(whole(styled(ms(0)).label(label), 1, 1).blank());
}

#[test]
fn a_label_with_diacritics_and_an_ellipsis() {
    let label = "Se încarcă datele…";
    assert_eq!(label.width(), 18);
    let drawn = whole(styled(ms(600)).label(label).hint(Some(HINT)), 21, 10);
    assert_eq!(drawn.text[1], "        ╭───╮");
    assert_eq!(drawn.text[7], " Se încarcă datele…");
    assert_eq!(drawn.marks[7], " mmmmmmmmaAammmmmmm");
    assert_eq!(band_at(ms(600), 18), Some(9));
    let short = whole(styled(ms(600)).label(label), 8, 1);
    assert_eq!(short.text, ["Se încar"]);
}

#[test]
fn a_combining_mark_stays_with_its_letter() {
    let label = "As\u{326}teptăm răspunsul…";
    assert_eq!(label.width(), 19);
    assert_eq!(whole(styled(ms(0)).label(label), 1, 1).text, ["A"]);
    assert_eq!(whole(styled(ms(0)).label(label), 2, 1).text, ["As\u{326}"]);
    assert_eq!(
        whole(styled(ms(0)).label(label), 30, 1).text,
        ["     As\u{326}teptăm răspunsul…"]
    );
    let at = (0..1200)
        .find(|at| band_at(ms(*at), 19) == Some(1))
        .unwrap();
    let drawn = whole(styled(ms(at)).label(label), 19, 1);
    assert_eq!(drawn.marks, [format!("aAa{}", "m".repeat(16))]);
}

#[test]
fn the_defaults_are_the_hey_copy_with_no_hint() {
    let drawn = whole(Hourglass::new(ms(0)), 30, 9);
    assert_eq!(drawn.text[1], "            ╭───╮");
    assert_eq!(drawn.text[7], "          Loading...");
    assert_eq!(drawn.text[8], "");
    let by_reference = {
        let mut terminal = Terminal::new(TestBackend::new(30, 9)).unwrap();
        let widget = Hourglass::new(ms(0));
        terminal
            .draw(|frame| {
                let area = frame.area();
                (&widget).render(area, frame.buffer_mut());
            })
            .unwrap();
        read(terminal.backend().buffer())
    };
    assert_eq!(by_reference.text, drawn.text);
}

fn render_into(widget: Hourglass<'_>, bounds: Rect, area: Rect) -> Buffer {
    let mut buffer = Buffer::empty(bounds);
    widget.render(area, &mut buffer);
    buffer
}

#[test]
fn control_characters_are_dropped_and_the_rest_centres_as_if_they_were_never_there() {
    let dirty_label = "\tLoad\x1b[31ming\u{9b}...\x7f\u{85}\r\n";
    let dirty_hint = "\x1b[0mesc\tstops\nwaiting\x7f";
    let clean_label = "Load[31ming...";
    let clean_hint = "[0mescstopswaiting";
    for (at, width, height) in [(0, 21, 10), (600, 21, 10), (600, 30, 2), (3300, 14, 3)] {
        let dirty = whole(
            styled(ms(at)).label(dirty_label).hint(Some(dirty_hint)),
            width,
            height,
        );
        let clean = whole(
            styled(ms(at)).label(clean_label).hint(Some(clean_hint)),
            width,
            height,
        );
        assert_eq!(dirty.text, clean.text, "at {at} ms in {width}x{height}");
        assert_eq!(dirty.marks, clean.marks, "at {at} ms in {width}x{height}");
    }
    let drawn = whole(
        styled(ms(0)).label(dirty_label).hint(Some(dirty_hint)),
        21,
        10,
    );
    assert_eq!(drawn.text[7], "   Load[31ming...");
    assert_eq!(drawn.text[8], " [0mescstopswaiting");
    assert_eq!(drawn.marks[8], format!(" {}", "m".repeat(18)));
}

#[test]
fn a_label_of_nothing_printable_draws_no_label_row() {
    for label in [
        "\u{200b}\u{200d}\u{2060}\u{feff}",
        "\t\n\r\n\x1b\x7f\0",
        "\u{301}\u{302}",
        "",
    ] {
        let glass = whole(styled(ms(600)).label(label).hint(Some(HINT)), 21, 10);
        assert_eq!(glass.text[1], "        ╭───╮", "{label:?}");
        assert_eq!(glass.text[7], "", "{label:?}");
        assert_eq!(glass.marks[7], "", "{label:?}");
        assert_eq!(glass.text[8], "  esc stops waiting", "{label:?}");
        let narrow = whole(styled(ms(600)).label(label).hint(Some(HINT)), 10, 4);
        assert_eq!(narrow.text, ["", "", "esc stops", ""], "{label:?}");
        assert!(
            whole(styled(ms(600)).label(label), 30, 3).blank(),
            "{label:?}"
        );
        assert!(
            whole(styled(ms(600)).label(label), 1, 1).blank(),
            "{label:?}"
        );
    }
}

#[test]
fn a_wide_hint_is_cut_at_a_whole_glyph_and_stays_muted() {
    let narrow = whole(styled(ms(600)).label(LABEL).hint(Some("日ab本c")), 4, 2);
    assert_eq!(narrow.text, ["Load", "日ab"]);
    assert_eq!(narrow.marks[1], "m mm");
    let odd = whole(styled(ms(600)).label(LABEL).hint(Some("日本語")), 5, 2);
    assert_eq!(odd.text[1], "日本");
    assert_eq!(odd.marks[1], "m m");
    let glass = whole(
        styled(ms(0)).label(LABEL).hint(Some("日本語のラベル日本")),
        11,
        8,
    );
    assert_eq!(glass.text[0], "   ╭───╮");
    assert_eq!(glass.text[7], "日本語のラ");
    assert_eq!(glass.marks[7], "m m m m m");
    let long = whole(hourglass(ms(600)), 12, 2);
    assert_eq!(long.text[1], "esc stops wa");
    assert_eq!(long.marks[1], "m".repeat(12));
}

#[test]
fn halfwidth_sound_marks_take_the_cell_the_terminal_gives_them() {
    let label = "ﾊﾞｶ";
    let fits = whole(styled(ms(0)).label(label), 3, 1);
    assert_eq!(fits.text, ["ﾊﾞｶ"]);
    assert_eq!(fits.marks, ["m m"]);
    assert_eq!(whole(styled(ms(0)).label(label), 2, 1).text, ["ﾊﾞ"]);
    assert!(whole(styled(ms(0)).label(label), 1, 1).blank());
    let centred = whole(styled(ms(0)).label(label), 7, 1);
    assert_eq!(centred.text, ["  ﾊﾞｶ"]);
    let hint = whole(styled(ms(0)).label(LABEL).hint(Some("ﾊﾟﾊﾟ")), 10, 2);
    assert_eq!(hint.text[1], "   ﾊﾟﾊﾟ");
    assert_eq!(hint.marks[1], "   m m");
}

#[test]
fn huge_elapsed_times_stay_in_range_and_keep_the_phase() {
    let huge = [
        Duration::MAX,
        Duration::from_secs(u64::MAX),
        ms(u64::MAX),
        Duration::from_secs(86_400 * 365 * 1000) + ms(3333),
    ];
    for elapsed in huge {
        assert!(frame_at(elapsed) < FRAMES.len(), "{elapsed:?}");
        for n in [0, 1, 10, usize::MAX] {
            if let Some(band) = band_at(elapsed, n) {
                assert!(band < n, "{elapsed:?} n {n}");
            }
        }
        let phase = ms(u64::try_from(elapsed.as_millis() % 21_000).unwrap());
        assert_eq!(frame_at(elapsed), frame_at(phase), "{elapsed:?}");
        assert_eq!(band_at(elapsed, 10), band_at(phase, 10), "{elapsed:?}");
        for (width, height) in [(21, 10), (10, 2), (5, 1)] {
            let drawn = whole(hourglass(elapsed), width, height);
            let same = whole(hourglass(phase), width, height);
            assert_eq!(drawn.text, same.text, "{elapsed:?}");
            assert_eq!(drawn.marks, same.marks, "{elapsed:?}");
        }
    }
}

#[test]
fn the_band_for_no_one_and_very_many_characters() {
    for at in [0, 200, 600, 1199, 1200, 1499, u64::MAX] {
        assert_eq!(band_at(ms(at), 0), None, "at {at} ms");
    }
    let one: Vec<usize> = (0..1500).filter_map(|at| band_at(ms(at), 1)).collect();
    assert!(!one.is_empty());
    assert!(one.iter().all(|band| *band == 0));
    assert_eq!(band_at(ms(0), usize::MAX), None);
    assert!(
        band_at(ms(600), usize::MAX)
            .is_some_and(|band| band > usize::MAX / 4 && band < 3 * (usize::MAX / 4))
    );
    assert_eq!(band_at(ms(1200), usize::MAX), None);
    let sweep: Vec<usize> = (0..1200)
        .filter_map(|at| band_at(ms(at), usize::MAX))
        .collect();
    assert!(sweep.windows(2).all(|pair| pair[0] < pair[1]));
    assert!(sweep.last().is_some_and(|band| *band < usize::MAX));
}

#[test]
fn the_owned_and_the_borrowed_widget_draw_the_same() {
    let bounds = Rect::new(0, 0, 24, 12);
    for at in [0, 600, 3300, 4199] {
        for area in [bounds, Rect::new(3, 2, 21, 10), Rect::new(1, 1, 9, 3)] {
            let widget = hourglass(ms(at));
            let owned = render_into(widget, bounds, area);
            let mut borrowed = Buffer::empty(bounds);
            (&widget).render(area, &mut borrowed);
            assert_eq!(owned, borrowed, "at {at} ms in {area:?}");
        }
    }
}

#[test]
fn two_rows_hold_label_and_hint_and_one_row_the_label_alone() {
    let two = whole(hourglass(ms(600)), 30, 2);
    assert_eq!(
        two.text,
        ["          Loading...", "      esc stops waiting"]
    );
    assert_eq!(
        two.marks,
        ["          mmmmaAammm", "      mmmmmmmmmmmmmmmmm"]
    );
    let one = whole(hourglass(ms(600)), 30, 1);
    assert_eq!(one.text, ["          Loading..."]);
    let three = whole(hourglass(ms(600)), 30, 3);
    assert_eq!(three.text[..2], two.text[..]);
    assert_eq!(three.text[2], "");
}

#[test]
fn an_area_past_the_buffer_edge_draws_only_what_overlaps() {
    let edge = draw(hourglass(ms(600)), 21, 10, Rect::new(10, 5, 21, 10));
    assert_eq!(
        edge.text,
        [
            "",
            "",
            "",
            "",
            "",
            "",
            "          Loading...",
            "          esc stops w",
            "",
            "",
        ]
    );
    assert_eq!(edge.marks[7], format!("          {}", "m".repeat(11)));
    let bounds = Rect::new(0, 0, 30, 12);
    let glass = read(&render_into(
        hourglass(ms(0)),
        bounds,
        Rect::new(5, 2, 40, 40),
    ));
    assert_eq!(glass.text[3], format!("{}╭───╮", " ".repeat(15)));
    assert_eq!(glass.text[9], format!("{}Loading...", " ".repeat(12)));
}

#[test]
fn an_area_outside_the_buffer_draws_nothing() {
    for area in [
        Rect::new(21, 0, 10, 10),
        Rect::new(0, 10, 21, 10),
        Rect::new(40, 30, 21, 10),
        Rect::new(u16::MAX - 21, u16::MAX - 10, 21, 10),
        Rect::new(u16::MAX, u16::MAX, 30, 30),
    ] {
        assert!(draw(hourglass(ms(600)), 21, 10, area).blank(), "{area:?}");
    }
    let bounds = Rect::new(5, 3, 21, 10);
    for area in [Rect::new(0, 0, 5, 13), Rect::new(0, 0, 26, 3)] {
        let drawn = render_into(hourglass(ms(600)), bounds, area);
        assert_eq!(drawn, Buffer::empty(bounds), "{area:?}");
    }
}

#[test]
fn a_buffer_away_from_the_origin_centres_in_its_own_area() {
    let corner = whole(hourglass(ms(3300)), 21, 10);
    let bounds = Rect::new(5, 3, 21, 10);
    for area in [bounds, Rect::new(0, 0, 26, 13), Rect::new(5, 3, 40, 40)] {
        let away = read(&render_into(hourglass(ms(3300)), bounds, area));
        assert_eq!(away.text, corner.text, "{area:?}");
        assert_eq!(away.marks, corner.marks, "{area:?}");
    }
}

#[test]
fn a_buffer_at_the_u16_limit_draws_like_one_at_the_origin() {
    let bounds = Rect::new(u16::MAX - 21, u16::MAX - 10, 21, 10);
    assert_eq!((bounds.right(), bounds.bottom()), (u16::MAX, u16::MAX));
    for at in [0, 600, 3300] {
        for widget in [hourglass(ms(at)), styled(ms(at)).label(LABEL)] {
            let corner = whole(widget, 21, 10);
            let limit = read(&render_into(widget, bounds, bounds));
            assert_eq!(limit.text, corner.text, "at {at} ms");
            assert_eq!(limit.marks, corner.marks, "at {at} ms");
        }
        for (width, height) in [(21, 7), (21, 8), (11, 7), (10, 2), (10, 1), (21, 1)] {
            let area = Rect::new(u16::MAX - width, u16::MAX - height, width, height);
            for (widget, block) in [(hourglass(ms(at)), 8), (styled(ms(at)).label(LABEL), 7)] {
                let limit = read(&render_into(widget, bounds, area));
                let mut origin = Buffer::empty(Rect::new(0, 0, 21, 10));
                widget.render(
                    Rect::new(21 - width, 10 - height, width, height),
                    &mut origin,
                );
                let origin = read(&origin);
                let case = format!("at {at} ms in {width}x{height}, block {block}");
                assert_eq!(limit.text, origin.text, "{case}");
                assert_eq!(limit.marks, origin.marks, "{case}");
                assert!(!limit.blank(), "{case}");
                let glass = width >= 11 && height >= block;
                assert_eq!(
                    limit.marks.iter().any(|row| row.contains('g')),
                    glass,
                    "{case}"
                );
                if glass && height == 7 {
                    assert_eq!(limit.text[9].trim(), LABEL, "{case}");
                }
            }
        }
    }
}

#[test]
fn no_render_touches_a_cell_outside_its_area() {
    let sentinel = Cell::new("·");
    let bounds = Rect::new(2, 1, 20, 14);
    let offsets = [(2, 1), (5, 4), (0, 0), (12, 9)];
    let mut renders = 0;
    for width in 0..=14u16 {
        for height in 0..=10u16 {
            let step = usize::from(width) * 11 + usize::from(height);
            let at = CYCLE * u32::try_from(step % 13).unwrap() / 13;
            for (x, y) in offsets {
                let area = Rect::new(x, y, width, height);
                for hint in [None, Some(HINT), Some("日本語のラベル ﾊﾞｶ")] {
                    let mut buffer = Buffer::filled(bounds, sentinel.clone());
                    styled(at).label(LABEL).hint(hint).render(area, &mut buffer);
                    let mut touched = 0;
                    for row in bounds.top()..bounds.bottom() {
                        for column in bounds.left()..bounds.right() {
                            let cell = &buffer[(column, row)];
                            if area.contains(Position::new(column, row)) {
                                touched += usize::from(*cell != sentinel);
                            } else {
                                assert_eq!(*cell, sentinel, "{area:?} at {at:?} hint {hint:?}");
                            }
                        }
                    }
                    assert_eq!(
                        touched > 0,
                        !area.intersection(bounds).is_empty(),
                        "{area:?} at {at:?} hint {hint:?}"
                    );
                    renders += 1;
                }
            }
        }
    }
    assert_eq!(renders, 1980);
}
