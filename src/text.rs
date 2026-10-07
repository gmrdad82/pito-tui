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
}
