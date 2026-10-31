use ratatui::style::Style;
use unicode_segmentation::UnicodeSegmentation;
use unicode_width::UnicodeWidthStr;

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

pub(crate) fn graphemes(text: &str) -> impl Iterator<Item = &str> {
    text.split(char::is_control)
        .filter(|piece| !piece.is_empty())
        .enumerate()
        .flat_map(|(index, piece)| {
            (index > 0)
                .then_some(" ")
                .into_iter()
                .chain(piece.graphemes(true))
        })
}

pub(crate) fn keep<'t>(glyphs: impl Iterator<Item = &'t str>, room: usize) -> Option<usize> {
    let mut scan = Scan::new(room.saturating_sub(1));
    for grapheme in glyphs {
        scan.feed(grapheme, grapheme.width());
        if scan.done(room) {
            break;
        }
    }
    scan.keep(room)
}

pub(crate) fn kept<'t>(
    spans: &[(&'t str, Style)],
    keep: usize,
    mut put: impl FnMut(&'t str, Style),
) -> Style {
    let mut left = keep;
    let mut last = spans.first().map_or_else(Style::new, |(_, style)| *style);
    for (text, style) in spans {
        last = *style;
        for grapheme in graphemes(text) {
            if left == 0 {
                return *style;
            }
            put(grapheme, *style);
            left -= 1;
        }
    }
    last
}

#[cfg(test)]
mod tests {
    use super::*;

    fn head(text: &str, room: usize) -> Option<&str> {
        let keep = keep(graphemes(text), room)?;
        let end = text
            .grapheme_indices(true)
            .nth(keep)
            .map_or(text.len(), |(at, _)| at);
        Some(&text[..end])
    }

    #[test]
    fn text_that_fits_stays_whole() {
        assert_eq!(head("", 0), None);
        assert_eq!(head("deploy api", 10), None);
        assert_eq!(head("日本", 4), None);
    }

    #[test]
    fn cuts_after_whole_words() {
        assert_eq!(head("deploy the api", 12), Some("deploy the"));
        assert_eq!(head("deploy the api", 10), Some("deploy"));
        assert_eq!(head("deployment", 5), Some("depl"));
        assert_eq!(head("abc", 0), Some(""));
        assert_eq!(head("ab", 1), Some(""));
    }

    #[test]
    fn keeps_the_start_of_a_word_when_no_whole_word_fits() {
        assert_eq!(
            head("household@example.com", 20),
            Some("household@example.c")
        );
        assert_eq!(head("first.last@example.com", 8), Some("first.l"));
        assert_eq!(head("me household@example.com", 20), Some("me"));
        assert_eq!(head("12日本", 4), Some("12"));
        assert_eq!(head("1😀x", 3), Some("1"));
        assert_eq!(head("cafe\u{301}st", 5), Some("cafe\u{301}"));
    }

    #[test]
    fn cuts_names_after_slash_dash_underscore() {
        assert_eq!(head("src/view.rs", 8), Some("src/"));
        assert_eq!(head("billing-team-lead", 13), Some("billing-"));
        assert_eq!(head("snake_case_name", 12), Some("snake_case_"));
    }

    #[test]
    fn never_cuts_right_after_a_bare_number() {
        assert_eq!(head("uploaded 12 files", 15), Some("uploaded"));
        assert_eq!(head("12 files", 5), Some("12 f"));
        assert_eq!(head("12.5 kB left", 10), Some("12.5 kB"));
    }

    #[test]
    fn cuts_between_wide_glyphs() {
        assert_eq!(head("日本語のテキスト", 5), Some("日本"));
        assert_eq!(head("日本語", 1), Some(""));
        assert_eq!(head("日本語", 2), Some(""));
    }

    #[test]
    fn drops_whole_parts_between_separators() {
        let parts = "build · deploy api · tests";
        assert_eq!(head(parts, 24), Some("build · deploy api"));
        assert_eq!(head(parts, 18), Some("build"));
        assert_eq!(head(parts, 5), Some("buil"));
    }

    #[test]
    fn kept_hands_out_whole_runs_then_the_cut_one() {
        let bold = Style::new().add_modifier(ratatui::style::Modifier::BOLD);
        let dim = Style::new().add_modifier(ratatui::style::Modifier::DIM);
        let spans = [("deploy", bold), (" the api", dim)];
        let mut out = Vec::new();
        let last = kept(&spans, 6, |text, style| out.push((text, style)));
        assert_eq!(out.len(), 6);
        assert_eq!(out[0], ("d", bold));
        assert_eq!(last, dim);
        let mut out = Vec::new();
        assert_eq!(kept(&spans, 3, |t, s| out.push((t, s))), bold);
        assert_eq!(out, [("d", bold), ("e", bold), ("p", bold)]);
    }

    #[test]
    fn control_characters_become_single_spaces() {
        let glyphs: String = graphemes("a\n\nb\tc").collect();
        assert_eq!(glyphs, "a b c");
    }
}
