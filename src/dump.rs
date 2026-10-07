use std::fmt::Write;

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::buffer::{Buffer, Cell};
use ratatui::style::{Color, Modifier};
use unicode_width::UnicodeWidthStr;

pub fn size(text: &str) -> Option<(u16, u16)> {
    let (width, height) = text.split_once(['x', 'X'])?;
    let size = (width.parse().ok()?, height.parse().ok()?);
    (size.0 > 0 && size.1 > 0).then_some(size)
}

pub fn keys(text: &str) -> Result<Vec<KeyEvent>, String> {
    let mut out = Vec::new();
    let plain = |code| KeyEvent::new(code, KeyModifiers::NONE);
    for word in text.split_whitespace() {
        let named = match word.to_lowercase().as_str() {
            "enter" => Some(plain(KeyCode::Enter)),
            "esc" => Some(plain(KeyCode::Esc)),
            "up" => Some(plain(KeyCode::Up)),
            "down" => Some(plain(KeyCode::Down)),
            "left" => Some(plain(KeyCode::Left)),
            "right" => Some(plain(KeyCode::Right)),
            "pgup" => Some(plain(KeyCode::PageUp)),
            "pgdn" => Some(plain(KeyCode::PageDown)),
            "home" => Some(plain(KeyCode::Home)),
            "end" => Some(plain(KeyCode::End)),
            "backspace" => Some(plain(KeyCode::Backspace)),
            "delete" => Some(plain(KeyCode::Delete)),
            "space" => Some(plain(KeyCode::Char(' '))),
            "tab" => Some(plain(KeyCode::Tab)),
            "backtab" => Some(KeyEvent::new(KeyCode::BackTab, KeyModifiers::SHIFT)),
            _ => None,
        };
        if let Some(key) = named {
            out.push(key);
            continue;
        }
        let modified = [
            ("ctrl+", KeyModifiers::CONTROL),
            ("alt+", KeyModifiers::ALT),
        ]
        .into_iter()
        .find_map(|(prefix, modifier)| Some((word.strip_prefix(prefix)?, modifier)));
        if let Some((rest, modifier)) = modified {
            let mut chars = rest.chars();
            match (chars.next(), chars.next()) {
                (Some(c), None) => {
                    out.push(KeyEvent::new(KeyCode::Char(c), modifier));
                    continue;
                }
                _ => return Err(word.to_string()),
            }
        }
        out.extend(word.chars().map(|c| plain(KeyCode::Char(c))));
    }
    Ok(out)
}

fn rows(buffer: &Buffer) -> impl Iterator<Item = &[Cell]> {
    buffer.content.chunks(usize::from(buffer.area.width).max(1))
}

fn shown(row: &[Cell]) -> impl Iterator<Item = &Cell> {
    let mut skip = 0;
    row.iter().filter(move |cell| {
        if skip > 0 {
            skip -= 1;
            return false;
        }
        skip = cell.symbol().width().saturating_sub(1);
        true
    })
}

pub fn text(buffer: &Buffer) -> String {
    let mut out = String::new();
    for (y, row) in rows(buffer).enumerate() {
        if y > 0 {
            out.push('\n');
        }
        let start = out.len();
        for cell in shown(row) {
            out.push_str(cell.symbol());
        }
        let end = out[start..].trim_end().len() + start;
        out.truncate(end);
    }
    out
}

fn colour(out: &mut String, color: Color, base: u8) {
    let bright = base + 60;
    let _ = match color {
        Color::Reset => return,
        Color::Black => write!(out, ";{base}"),
        Color::Red => write!(out, ";{}", base + 1),
        Color::Green => write!(out, ";{}", base + 2),
        Color::Yellow => write!(out, ";{}", base + 3),
        Color::Blue => write!(out, ";{}", base + 4),
        Color::Magenta => write!(out, ";{}", base + 5),
        Color::Cyan => write!(out, ";{}", base + 6),
        Color::Gray => write!(out, ";{}", base + 7),
        Color::DarkGray => write!(out, ";{bright}"),
        Color::LightRed => write!(out, ";{}", bright + 1),
        Color::LightGreen => write!(out, ";{}", bright + 2),
        Color::LightYellow => write!(out, ";{}", bright + 3),
        Color::LightBlue => write!(out, ";{}", bright + 4),
        Color::LightMagenta => write!(out, ";{}", bright + 5),
        Color::LightCyan => write!(out, ";{}", bright + 6),
        Color::White => write!(out, ";{}", bright + 7),
        Color::Rgb(r, g, b) => write!(out, ";{};2;{r};{g};{b}", base + 8),
        Color::Indexed(n) => write!(out, ";{};5;{n}", base + 8),
    };
}

fn sgr(cell: &Cell) -> String {
    let mut out = String::from("\x1b[0");
    for (flag, code) in [
        (Modifier::BOLD, ";1"),
        (Modifier::DIM, ";2"),
        (Modifier::ITALIC, ";3"),
        (Modifier::UNDERLINED, ";4"),
        (Modifier::REVERSED, ";7"),
        (Modifier::CROSSED_OUT, ";9"),
    ] {
        if cell.modifier.contains(flag) {
            out.push_str(code);
        }
    }
    colour(&mut out, cell.fg, 30);
    colour(&mut out, cell.bg, 40);
    out.push('m');
    out
}

pub fn ansi(buffer: &Buffer) -> String {
    let mut out = String::from("\x1b[?25l\x1b[2J");
    for (y, row) in rows(buffer).enumerate() {
        let _ = write!(out, "\x1b[{};1H", y + 1);
        let mut last = String::new();
        for cell in shown(row) {
            let style = sgr(cell);
            if style != last {
                out.push_str(&style);
                last = style;
            }
            out.push_str(cell.symbol());
        }
        out.push_str("\x1b[0m");
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::layout::Rect;
    use ratatui::style::Style;

    #[test]
    fn sizes_and_keys_read_the_way_every_app_wires_them() {
        assert_eq!(size("120x34"), Some((120, 34)));
        assert_eq!(size("0x9"), None);
        assert_eq!(size("wide"), None);
        let keys = keys("n tab 25 enter ctrl+c alt+x").unwrap();
        assert_eq!(keys.len(), 7);
        assert_eq!(keys[1].code, KeyCode::Tab);
        assert_eq!(keys[5].modifiers, KeyModifiers::CONTROL);
        assert_eq!(keys[6].modifiers, KeyModifiers::ALT);
        assert_eq!(super::keys("ctrl+xy"), Err("ctrl+xy".to_string()));
    }

    #[test]
    fn text_and_ansi_keep_wide_glyphs_whole_and_carry_the_colours() {
        let mut buffer = Buffer::empty(Rect::new(0, 0, 8, 2));
        buffer.set_string(0, 0, "日本 ok", Style::new().fg(Color::Rgb(1, 2, 3)));
        assert_eq!(text(&buffer), "日本 ok\n");
        let ansi = ansi(&buffer);
        assert!(ansi.starts_with("\x1b[?25l\x1b[2J\x1b[1;1H"));
        assert!(ansi.contains("\x1b[2;1H"));
        assert!(ansi.contains("38;2;1;2;3"));
    }
}
