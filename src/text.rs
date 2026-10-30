use std::iter;

use ratatui::style::Style;
use unicode_segmentation::UnicodeSegmentation;
use unicode_width::UnicodeWidthStr;

pub(crate) const ELLIPSIS: &str = "…";
const TAB: &str = "    ";

#[derive(Clone, Copy, PartialEq, Eq)]
enum Kind {
    Space,
    Joint,
    Wide,
    Word,
}

fn kind(grapheme: &str, cells: usize) -> Kind {
    if grapheme.chars().all(char::is_whitespace) {
        Kind::Space
    } else if matches!(grapheme, "/" | "-" | "_") {
        Kind::Joint
    } else if cells > 1 {
        Kind::Wide
    } else {
        Kind::Word
    }
}

#[derive(Clone, Copy, Default)]
struct Piece {
    seen: bool,
    letters: bool,
    digits: bool,
    other: bool,
    wide: bool,
}

impl Piece {
    fn add(&mut self, grapheme: &str, cells: usize) {
        self.seen = true;
        self.wide |= cells > 1;
        for c in grapheme.chars() {
            self.letters |= c.is_alphanumeric();
            self.digits |= c.is_numeric();
            self.other |= !(c.is_numeric() || matches!(c, '.' | ',' | '+' | '-' | '_'));
        }
    }

    fn number(self) -> bool {
        self.digits && !self.other
    }

    fn word(self) -> bool {
        (self.letters || self.wide) && !self.number()
    }

    fn separator(self) -> bool {
        self.seen && !self.letters && !self.wide
    }
}

#[derive(Clone, Copy, Default)]
struct Spot {
    count: usize,
    cells: usize,
}

struct Scan {
    limit: usize,
    count: usize,
    used: usize,
    prev: Option<Kind>,
    token: Piece,
    piece: Piece,
    joint: bool,
    last: Option<Spot>,
    after: bool,
    ended: bool,
    parted: bool,
    word: Option<Spot>,
    part: Option<Spot>,
    fill: Spot,
}

impl Scan {
    fn new(limit: usize) -> Self {
        Scan {
            limit,
            count: 0,
            used: 0,
            prev: None,
            token: Piece::default(),
            piece: Piece::default(),
            joint: false,
            last: None,
            after: false,
            ended: false,
            parted: false,
            word: None,
            part: None,
            fill: Spot::default(),
        }
    }

    fn mark(&mut self, spot: Spot) {
        if !self.parted && spot.cells <= self.limit {
            self.word = Some(spot);
        }
    }

    fn end(&mut self, spot: Spot) {
        if self.token.separator() {
            if let Some(last) = self
                .last
                .filter(|last| self.after && last.cells <= self.limit)
            {
                self.part = Some(last);
            }
            self.parted |= self.ended;
            self.after = false;
        } else if self.token.word() {
            self.mark(spot);
            self.last = Some(spot);
            self.after = true;
        } else {
            self.last = None;
            self.after = false;
        }
        self.ended = true;
    }

    fn open(&self) -> bool {
        self.after
            && !self.token.letters
            && !self.token.wide
            && self.last.is_some_and(|last| last.cells <= self.limit)
    }

    fn feed(&mut self, grapheme: &str, cells: usize) {
        let kind = kind(grapheme, cells);
        let here = Spot {
            count: self.count,
            cells: self.used,
        };
        match (self.prev, kind) {
            (Some(prev), Kind::Space) if prev != Kind::Space => self.end(here),
            (Some(Kind::Joint), _) if self.joint => self.mark(here),
            (Some(Kind::Wide), next) if next != Kind::Space => self.mark(here),
            (Some(Kind::Word), Kind::Wide) if self.piece.word() => self.mark(here),
            _ => {}
        }
        match kind {
            Kind::Space => {
                self.token = Piece::default();
                self.piece = Piece::default();
                self.joint = false;
            }
            Kind::Joint => {
                self.joint = self.prev.is_some_and(|prev| prev != Kind::Space) && self.piece.word();
                self.token.add(grapheme, cells);
                self.piece = Piece::default();
            }
            Kind::Wide | Kind::Word => {
                self.joint = false;
                self.token.add(grapheme, cells);
                self.piece.add(grapheme, cells);
            }
        }
        self.prev = Some(kind);
        self.count += 1;
        self.used = self.used.saturating_add(cells);
        if kind != Kind::Space && self.used <= self.limit {
            self.fill = Spot {
                count: self.count,
                cells: self.used,
            };
        }
    }

    fn done(&self, room: usize) -> bool {
        self.used > room && !self.open()
    }

    fn keep(&self, room: usize) -> Option<usize> {
        if self.used <= room {
            return None;
        }
        Some(self.part.or(self.word).unwrap_or(self.fill).count)
    }
}

pub(crate) fn keep<'t>(runs: impl Iterator<Item = &'t str>, room: usize) -> Option<usize> {
    let mut scan = Scan::new(room.saturating_sub(1));
    'runs: for text in runs {
        for grapheme in text.graphemes(true) {
            scan.feed(grapheme, grapheme.width());
            if scan.done(room) {
                break 'runs;
            }
        }
    }
    scan.keep(room)
}

pub(crate) fn kept<'t, S: AsRef<str>>(
    spans: &'t [(S, Style)],
    keep: usize,
    mut put: impl FnMut(&'t str, Style),
) -> Style {
    let mut left = keep;
    let mut last = spans.first().map_or_else(Style::new, |(_, style)| *style);
    for (text, style) in spans {
        let text = text.as_ref();
        last = *style;
        let count = text.graphemes(true).count();
        if left < count {
            let end = text
                .grapheme_indices(true)
                .nth(left)
                .map_or(text.len(), |(at, _)| at);
            put(&text[..end], *style);
            return *style;
        }
        put(text, *style);
        left -= count;
    }
    last
}

pub fn cut(text: &str, width: usize) -> Option<&str> {
    if cells(text) <= width {
        return None;
    }
    let keep = keep(iter::once(text), width).unwrap_or_default();
    let end = text
        .grapheme_indices(true)
        .nth(keep)
        .map_or(text.len(), |(at, _)| at);
    Some(&text[..end])
}

pub fn clean(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    for c in text.chars() {
        match c {
            '\t' => out.push_str(TAB),
            '\n' | '\r' => out.push(' '),
            c if c.is_control() => {}
            c => out.push(c),
        }
    }
    out
}

pub fn cells(text: &str) -> usize {
    text.width()
}

pub fn split(text: &str, width: usize) -> (&str, &str) {
    let mut used = 0;
    let mut end = 0;
    for (index, glyph) in text.grapheme_indices(true) {
        let wide = glyph.width();
        if used + wide > width {
            break;
        }
        used += wide;
        end = index + glyph.len();
    }
    text.split_at(end)
}

pub fn clip(text: &str, width: usize) -> String {
    match cut(text, width) {
        None => text.to_string(),
        Some(_) if width == 0 => String::new(),
        Some(kept) => format!("{kept}{ELLIPSIS}"),
    }
}

pub fn fit(text: &str, width: usize) -> String {
    let clipped = clip(text, width);
    let pad = width.saturating_sub(cells(&clipped));
    format!("{clipped}{}", " ".repeat(pad))
}

pub fn wrap(text: &str, width: usize) -> Vec<String> {
    if width == 0 {
        return Vec::new();
    }
    let mut lines = Vec::new();
    for paragraph in text.split('\n') {
        let paragraph = clean(paragraph);
        let mut rest = paragraph.trim_end();
        if rest.is_empty() {
            lines.push(String::new());
            continue;
        }
        while !rest.is_empty() {
            if cells(rest) <= width {
                lines.push(rest.to_string());
                break;
            }
            let (head, _) = split(rest, width);
            let cut = match head.rfind(' ') {
                Some(space) if space > 0 && rest[head.len()..].starts_with(' ') => head.len(),
                Some(space) if space > 0 => space,
                _ if head.is_empty() => rest
                    .grapheme_indices(true)
                    .nth(1)
                    .map_or(rest.len(), |(index, _)| index),
                _ => head.len(),
            };
            let line = rest[..cut].trim_end();
            if !line.is_empty() {
                lines.push(line.to_string());
            }
            rest = rest[cut..].trim_start();
        }
    }
    lines
}

pub fn hard_wrap(text: &str, width: usize) -> Vec<String> {
    if width == 0 {
        return Vec::new();
    }
    let mut lines = Vec::new();
    for paragraph in text.split('\n') {
        let paragraph = clean(paragraph);
        let mut rest = paragraph.trim_end();
        if rest.is_empty() {
            lines.push(String::new());
            continue;
        }
        while !rest.is_empty() {
            let (head, tail) = split(rest, width);
            let (head, tail) = if head.is_empty() {
                let cut = rest
                    .grapheme_indices(true)
                    .nth(1)
                    .map_or(rest.len(), |(index, _)| index);
                rest.split_at(cut)
            } else {
                (head, tail)
            };
            lines.push(head.to_string());
            rest = tail;
        }
    }
    lines
}

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::style::Modifier;

    #[test]
    fn widths_are_counted_in_cells_and_wide_glyphs_are_never_cut() {
        assert_eq!(cells("ăîșț"), 4);
        assert_eq!(cells("日本"), 4);
        assert_eq!(clip("日本語のテキスト", 5), "日本…");
        assert_eq!(fit("ab", 4), "ab  ");
        assert_eq!(clip("short", 9), "short");
        assert_eq!(wrap("the quick brown fox", 9), ["the quick", "brown fox"]);
        assert_eq!(wrap("abcdefghij", 4), ["abcd", "efgh", "ij"]);
        assert_eq!(wrap("日本語", 1), ["日", "本", "語"]);
        assert_eq!(hard_wrap("the quick brown", 6), ["the qu", "ick br", "own"]);
        assert_eq!(hard_wrap("a日本\n\nb", 2), ["a", "日", "本", "", "b"]);
    }

    const SAMPLES: [&str; 15] = [
        "",
        "plain ascii words here",
        "日本語のテキスト",
        "😀 smile 😀😀",
        "e\u{301}cole cafe\u{301}",
        "a日b本c",
        "averyveryverylongword then short",
        "tab\there",
        "  leading and trailing  ",
        "family 👨\u{200d}👩\u{200d}👧 here",
        "flag 🇷🇴🇷🇴 ro",
        "love ❤\u{fe0f}❤\u{fe0f} you",
        "household@example.com",
        "first.last@company.com and more",
        "12 files left",
    ];

    #[test]
    fn clean_turns_tabs_into_four_spaces_and_breaks_into_spaces_and_drops_controls() {
        assert_eq!(clean(""), "");
        assert_eq!(clean("a\tb"), "a    b");
        assert_eq!(clean("a\nb\rc\r\n"), "a b c  ");
        assert_eq!(clean("x\u{7}y\u{1b}[31mz\u{0}"), "xy[31mz");
        assert_eq!(clean("日本 😀 e\u{301}"), "日本 😀 e\u{301}");
        assert_eq!(clean("\u{7f}"), "");
    }

    #[test]
    fn cells_count_terminal_columns_not_bytes_or_chars() {
        assert_eq!(cells(""), 0);
        assert_eq!(cells("abc"), 3);
        assert_eq!(cells("日本語"), 6);
        assert_eq!(cells("😀"), 2);
        assert_eq!(cells("e\u{301}"), 1);
        assert_eq!(cells("a日b"), 4);
    }

    #[test]
    fn split_keeps_whole_graphemes_that_fit_and_hands_back_the_rest() {
        assert_eq!(split("", 5), ("", ""));
        assert_eq!(split("abc", 0), ("", "abc"));
        assert_eq!(split("abc", 3), ("abc", ""));
        assert_eq!(split("abcd", 3), ("abc", "d"));
        assert_eq!(split("abc", 9), ("abc", ""));
        assert_eq!(split("日本", 3), ("日", "本"));
        assert_eq!(split("日本", 1), ("", "日本"));
        assert_eq!(split("日本", 4), ("日本", ""));
        assert_eq!(split("😀x", 1), ("", "😀x"));
        assert_eq!(split("😀x", 2), ("😀", "x"));
        assert_eq!(split("e\u{301}x", 1), ("e\u{301}", "x"));
        assert_eq!(split("a\u{301}\u{302}b", 1), ("a\u{301}\u{302}", "b"));
    }

    #[test]
    fn split_halves_always_rejoin_into_the_input_and_the_head_fits() {
        for text in SAMPLES {
            for width in 0..=12 {
                let (head, tail) = split(text, width);
                assert_eq!(format!("{head}{tail}"), text, "{text:?} at {width}");
                assert!(cells(head) <= width, "{text:?} at {width}");
            }
        }
    }

    #[test]
    fn clip_keeps_text_that_fits_and_ends_cut_text_with_an_ellipsis() {
        assert_eq!(clip("", 0), "");
        assert_eq!(clip("", 3), "");
        assert_eq!(clip("abc", 3), "abc");
        assert_eq!(clip("abcd", 3), "ab…");
        assert_eq!(clip("a", 0), "");
        assert_eq!(clip("ab", 1), "…");
        assert_eq!(clip("日本語", 4), "日…");
        assert_eq!(clip("日本語", 6), "日本語");
        assert_eq!(clip("a日本", 3), "a…");
        assert_eq!(clip("😀😀", 3), "😀…");
        assert_eq!(clip("cafe\u{301}s", 4), "caf…");
        assert_eq!(clip("household@example.com", 20), "household@example.c…");
        assert_eq!(clip("deploy the api", 12), "deploy the…");
        assert_eq!(clip("deploy the api", 0), "");
        assert_eq!(clip("cafe\u{301}", 4), "cafe\u{301}");
    }

    #[test]
    fn clip_never_exceeds_its_width_and_only_cut_text_gains_an_ellipsis() {
        for text in SAMPLES {
            for width in 0..=40 {
                let clipped = clip(text, width);
                assert!(cells(&clipped) <= width, "{text:?} at {width}");
                if cells(text) <= width {
                    assert_eq!(clipped, text);
                } else if width > 0 {
                    assert!(clipped.ends_with(ELLIPSIS), "{text:?} at {width}");
                    let kept = clipped.trim_end_matches(ELLIPSIS);
                    assert!(text.starts_with(kept), "{text:?} at {width}");
                }
            }
        }
    }

    #[test]
    fn fit_pads_or_clips_to_exactly_the_width_where_a_glyph_allows() {
        assert_eq!(fit("", 3), "   ");
        assert_eq!(fit("", 0), "");
        assert_eq!(fit("abc", 0), "");
        assert_eq!(fit("abc", 3), "abc");
        assert_eq!(fit("abcd", 3), "ab…");
        assert_eq!(fit("deploy the api", 9), "deploy…  ");
        assert_eq!(fit("日本", 3), "日…");
        assert_eq!(fit("a日本", 3), "a… ");
        assert_eq!(fit("日", 5), "日   ");
        for text in SAMPLES {
            for width in 0..=40 {
                assert_eq!(cells(&fit(text, width)), width, "{text:?} at {width}");
            }
        }
    }

    #[test]
    fn wrap_breaks_at_spaces_and_cuts_only_words_longer_than_the_width() {
        assert!(wrap("anything", 0).is_empty());
        assert!(wrap("", 0).is_empty());
        assert_eq!(wrap("", 5), [""]);
        assert_eq!(wrap("abc def", 7), ["abc def"]);
        assert_eq!(wrap("abc defg", 7), ["abc", "defg"]);
        assert_eq!(wrap("abc def ghi", 7), ["abc def", "ghi"]);
        assert_eq!(
            wrap("a verylongword b", 4),
            ["a", "very", "long", "word", "b"]
        );
        assert_eq!(wrap("a\n\nb", 5), ["a", "", "b"]);
        assert_eq!(wrap("abc   ", 10), ["abc"]);
        assert_eq!(wrap("  abc", 10), ["  abc"]);
        assert_eq!(wrap("a\tb", 10), ["a    b"]);
        assert_eq!(wrap("one\r\ntwo", 20), ["one", "two"]);
        assert_eq!(wrap("日本 語", 4), ["日本", "語"]);
        assert_eq!(wrap("😀a", 1), ["😀", "a"]);
        assert_eq!(wrap("e\u{301}e\u{301}", 1), ["e\u{301}", "e\u{301}"]);
        assert_eq!(wrap("x", 1), ["x"]);
    }

    #[test]
    fn wrap_never_opens_an_indented_paragraph_with_a_blank_line() {
        for (text, width) in [
            ("  averylongword", 10),
            ("  leading", 1),
            ("  leading", 2),
            ("   x", 2),
            ("one\n  averylongword", 10),
        ] {
            let lines = wrap(text, width);
            assert!(
                lines.iter().all(|line| !line.is_empty()),
                "{text:?} at {width}: {lines:?}"
            );
            assert!(
                lines.iter().all(|line| cells(line) <= width),
                "{text:?} at {width}: {lines:?}"
            );
            let kept: String = lines.concat().split_whitespace().collect();
            let given: String = text.split_whitespace().collect();
            assert_eq!(kept, given, "{text:?} at {width}");
        }
    }

    #[test]
    fn emoji_sequences_stay_whole_and_within_their_width() {
        for text in ["👨\u{200d}👩\u{200d}👧", "🇷🇴", "❤\u{fe0f}"] {
            let (head, tail) = split(text, cells(text));
            assert_eq!((head, tail), (text, ""), "{text:?}");
            assert_eq!(clip(text, cells(text)), text);
            assert_eq!(cells(&fit(text, 4)), 4, "{text:?}");
            assert_eq!(hard_wrap(text, 1), [text]);
        }
    }

    #[test]
    fn wrap_lines_fit_unless_one_glyph_is_wider_than_the_width_and_lose_no_words() {
        for text in SAMPLES {
            for width in 1..=12 {
                let lines = wrap(text, width);
                assert!(!lines.is_empty(), "{text:?} at {width}");
                for line in &lines {
                    let single = line.graphemes(true).count() == 1;
                    assert!(cells(line) <= width || single, "{line:?} at {width}");
                    assert_eq!(line.trim_end(), line, "{line:?} at {width}");
                }
                if !clean(text).trim().is_empty() {
                    assert!(
                        lines.iter().all(|line| !line.is_empty()),
                        "{text:?} at {width}: {lines:?}"
                    );
                }
                let kept: String = lines.concat().split_whitespace().collect();
                let given: String = clean(text).split_whitespace().collect();
                assert_eq!(kept, given, "{text:?} at {width}");
            }
        }
    }

    #[test]
    fn hard_wrap_cuts_at_the_width_and_keeps_every_character() {
        assert!(hard_wrap("anything", 0).is_empty());
        assert_eq!(hard_wrap("", 3), [""]);
        assert_eq!(hard_wrap("abc", 3), ["abc"]);
        assert_eq!(hard_wrap("abcd", 3), ["abc", "d"]);
        assert_eq!(hard_wrap("abcd", 2), ["ab", "cd"]);
        assert_eq!(hard_wrap("ab cd", 3), ["ab ", "cd"]);
        assert_eq!(hard_wrap("😀", 1), ["😀"]);
        assert_eq!(hard_wrap("\tx", 2), ["  ", "  ", "x"]);
        assert_eq!(hard_wrap("cafe\u{301}!", 4), ["cafe\u{301}", "!"]);
        for text in SAMPLES {
            for width in 1..=12 {
                let lines = hard_wrap(text, width);
                for line in &lines {
                    let single = line.graphemes(true).count() == 1;
                    assert!(cells(line) <= width || single, "{line:?} at {width}");
                }
                assert_eq!(
                    lines.concat(),
                    clean(text).trim_end(),
                    "{text:?} at {width}"
                );
            }
        }
    }

    #[test]
    fn cut_leaves_text_that_fits_whole() {
        assert_eq!(cut("", 0), None);
        assert_eq!(cut("deploy api", 10), None);
        assert_eq!(cut("日本", 4), None);
        assert_eq!(cut("cafe\u{301}", 4), None);
    }

    #[test]
    fn cut_keeps_whole_words_and_drops_the_spaces_before_the_ellipsis() {
        assert_eq!(cut("deploy the api", 12), Some("deploy the"));
        assert_eq!(cut("deploy the api", 11), Some("deploy the"));
        assert_eq!(cut("deploy the api", 10), Some("deploy"));
        assert_eq!(cut("deploy   api", 9), Some("deploy"));
        assert_eq!(cut("exit 1: the build", 8), Some("exit 1:"));
    }

    #[test]
    fn cut_keeps_the_start_of_a_word_when_no_whole_word_fits() {
        assert_eq!(cut("deployment", 5), Some("depl"));
        assert_eq!(cut("1234567", 6), Some("12345"));
        assert_eq!(
            cut("household@example.com", 20),
            Some("household@example.c")
        );
        assert_eq!(
            cut("first.last@company.com", 21),
            Some("first.last@company.c")
        );
        assert_eq!(cut("first.last@company.com", 12), Some("first.last@"));
        assert_eq!(cut("first.last@company.com", 6), Some("first"));
        assert_eq!(cut("first.last@company.com", 2), Some("f"));
        assert_eq!(cut("  indented", 4), Some("  i"));
        assert_eq!(cut("  indented", 3), Some(""));
    }

    #[test]
    fn cut_keeps_only_the_ellipsis_when_no_grapheme_fits() {
        assert_eq!(cut("ab", 1), Some(""));
        assert_eq!(cut("abc", 0), Some(""));
        assert_eq!(cut("日本語", 2), Some(""));
        assert_eq!(cut("😀x", 2), Some(""));
    }

    #[test]
    fn cut_keeps_whole_words_whenever_one_fits_even_beside_a_longer_start() {
        assert_eq!(cut("me household@example.com", 20), Some("me"));
        assert_eq!(cut("deploy the api", 7), Some("deploy"));
        assert_eq!(cut("deploy the api", 6), Some("deplo"));
    }

    #[test]
    fn cut_keeps_wide_and_combined_graphemes_whole_at_a_start_cut() {
        assert_eq!(cut("12日本", 4), Some("12"));
        assert_eq!(cut("12日本", 5), Some("12日"));
        assert_eq!(cut("1😀x", 3), Some("1"));
        assert_eq!(cut("1😀xy", 4), Some("1😀"));
        assert_eq!(cut("cafe\u{301}st", 5), Some("cafe\u{301}"));
        assert_eq!(cut("1👨\u{200d}👩\u{200d}👧x", 3), Some("1"));
    }

    #[test]
    fn cut_breaks_names_after_a_slash_a_dash_or_an_underscore() {
        assert_eq!(cut("src/view.rs", 8), Some("src/"));
        assert_eq!(cut("billing-team-lead", 14), Some("billing-team-"));
        assert_eq!(cut("billing-team-lead", 13), Some("billing-"));
        assert_eq!(cut("snake_case_name", 12), Some("snake_case_"));
        assert_eq!(cut("/usr/local/bin", 9), Some("/usr/"));
        assert_eq!(cut("-flag", 3), Some("-f"));
    }

    #[test]
    fn cut_never_lands_right_after_a_bare_number() {
        assert_eq!(cut("uploaded 12 files", 15), Some("uploaded"));
        assert_eq!(cut("12 files", 5), Some("12 f"));
        assert_eq!(cut("2026-10-26 release", 15), Some("2026-10-26 rel"));
        assert_eq!(cut("12.5 kB left", 10), Some("12.5 kB"));
        assert_eq!(cut("v1.2 build", 8), Some("v1.2"));
        assert_eq!(cut("12日本", 4), Some("12"));
    }

    #[test]
    fn cut_goes_between_wide_glyphs_and_keeps_graphemes_whole() {
        assert_eq!(cut("日本語のテキスト", 5), Some("日本"));
        assert_eq!(cut("日本語", 1), Some(""));
        assert_eq!(cut("日本語", 3), Some("日"));
        assert_eq!(cut("a日本", 3), Some("a"));
        assert_eq!(cut("deploy 日本語", 10), Some("deploy 日"));
        assert_eq!(cut("😀😀 smile", 3), Some("😀"));
        assert_eq!(cut("cafe\u{301} au lait", 8), Some("cafe\u{301} au"));
    }

    #[test]
    fn cut_drops_whole_parts_between_spaced_separators() {
        let parts = "build · deploy api · tests";
        assert_eq!(cut(parts, 24), Some("build · deploy api"));
        assert_eq!(cut(parts, 18), Some("build"));
        assert_eq!(cut(parts, 5), Some("buil"));
        assert_eq!(cut("a · b", 2), Some("a"));
        assert_eq!(cut("build 12 · deploy api · x", 20), Some("build"));
        assert_eq!(
            cut("build 12 · deploy api · x", 23),
            Some("build 12 · deploy api")
        );
        assert_eq!(cut("• item one two", 12), Some("• item one"));
        assert_eq!(cut("v1 → v2 → v3", 9), Some("v1 → v2"));
    }

    #[test]
    fn cut_is_a_prefix_that_fits_with_its_ellipsis_and_grows_with_the_room() {
        for text in SAMPLES {
            let mut kept = 0;
            for width in 0..=40 {
                let now = match cut(text, width) {
                    None => {
                        assert!(cells(text) <= width, "{text:?} at {width}");
                        text.len()
                    }
                    Some(head) => {
                        assert!(text.starts_with(head), "{text:?} at {width}");
                        assert!(cells(head) < width.max(1), "{text:?} at {width}");
                        assert_eq!(head.trim_end(), head, "{text:?} at {width}");
                        head.len()
                    }
                };
                assert!(now >= kept, "{text:?} at {width}");
                kept = now;
            }
        }
    }

    #[test]
    fn kept_hands_out_whole_runs_then_the_cut_one_and_its_style() {
        let bold = Style::new().add_modifier(Modifier::BOLD);
        let dim = Style::new().add_modifier(Modifier::DIM);
        let spans = [("deploy", bold), (" the api", dim)];
        let mut out = Vec::new();
        let last = kept(&spans, 6, |text, style| out.push((text, style)));
        assert_eq!(out, [("deploy", bold), ("", dim)]);
        assert_eq!(last, dim);
        let runs = spans.iter().map(|(text, _)| *text);
        assert_eq!(keep(runs.clone(), 9), Some(6));
        assert_eq!(keep(runs, 14), None);
        let mut out = Vec::new();
        assert_eq!(kept(&spans, 3, |text, style| out.push((text, style))), bold);
        assert_eq!(out, [("dep", bold)]);
        let none: [(&str, Style); 0] = [];
        assert_eq!(kept(&none, 0, |_, _| {}), Style::new());
    }
}
