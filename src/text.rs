use unicode_segmentation::UnicodeSegmentation;
use unicode_width::UnicodeWidthStr;

const ELLIPSIS: &str = "…";
const TAB: &str = "    ";

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
    if cells(text) <= width {
        return text.to_string();
    }
    if width == 0 {
        return String::new();
    }
    let (kept, _) = split(text, width - 1);
    format!("{kept}{ELLIPSIS}")
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
            lines.push(rest[..cut].trim_end().to_string());
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

    const SAMPLES: [&str; 12] = [
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
}
