use std::time::Duration;

use ratatui::{
    style::{Color, Style},
    text::{Line, Span},
};

pub const SPIN: Duration = Duration::from_millis(80);
pub const SPINNER: [&str; 10] = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];
pub const ABOUT: &str = "≈";

const LADDER: [char; 8] = ['⡀', '⡄', '⡆', '⡇', '⣇', '⣧', '⣷', '⣿'];
const TRACK: char = '⣀';
const CLOSE: f64 = 4.0 * f64::EPSILON;

fn clean(fraction: f64) -> f64 {
    if fraction.is_nan() {
        0.0
    } else {
        fraction.clamp(0.0, 1.0)
    }
}

fn floored(fraction: f64, total: usize) -> usize {
    if total == 0 {
        return 0;
    }
    let fraction = clean(fraction);
    let steps = (fraction * total as f64).floor();
    let next = (steps + 1.0) / total as f64;
    let up = next - fraction <= CLOSE * next;
    (steps as usize + usize::from(up)).min(total)
}

pub fn bar(fraction: f64, cells: usize) -> String {
    let total = cells * 8;
    let steps = floored(fraction, total);
    let (full, part) = (steps / 8, steps % 8);
    let mut text: String = std::iter::repeat_n(LADDER[7], full).collect();
    if part > 0 {
        text.push(LADDER[part - 1]);
    }
    let used = full + usize::from(part > 0);
    text.extend(std::iter::repeat_n(TRACK, cells.saturating_sub(used)));
    text
}

const LADDER_STR: [&str; 8] = ["⡀", "⡄", "⡆", "⡇", "⣇", "⣧", "⣷", "⣿"];
const TRACK_STR: &str = "⣀";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Ramp {
    stops: [Color; 4],
    len: usize,
}

impl Ramp {
    pub fn new(stops: &[Color]) -> Self {
        let mut out = [Color::Reset; 4];
        let len = stops.len().clamp(1, 4);
        for (slot, stop) in out.iter_mut().zip(stops) {
            *slot = *stop;
        }
        if stops.is_empty() {
            out[0] = Color::Reset;
        }
        Ramp { stops: out, len }
    }

    pub const fn load() -> Self {
        Ramp {
            stops: [
                Color::Rgb(0x2d, 0xd4, 0xbf),
                Color::Rgb(0x4a, 0xde, 0x80),
                Color::Rgb(0xfb, 0xbf, 0x24),
                Color::Rgb(0xef, 0x44, 0x44),
            ],
            len: 4,
        }
    }

    pub fn from_accent(accent: Color) -> Self {
        Ramp {
            stops: [
                mix(Color::Rgb(0, 0, 0), accent, 0.45),
                accent,
                mix(accent, Color::Rgb(255, 255, 255), 0.4),
                Color::Reset,
            ],
            len: 3,
        }
    }

    pub fn accent(palette: &crate::Palette) -> Self {
        Self::from_accent(palette.accent.fg.unwrap_or(Color::White))
    }

    pub fn at(&self, position: f64) -> Color {
        let position = clean(position);
        if self.len == 1 {
            return self.stops[0];
        }
        let scaled = position * (self.len - 1) as f64;
        let low = (scaled.floor() as usize).min(self.len - 2);
        mix(self.stops[low], self.stops[low + 1], scaled - low as f64)
    }
}

pub(crate) fn mix(from: Color, to: Color, amount: f64) -> Color {
    match (from, to) {
        (Color::Rgb(ar, ag, ab), Color::Rgb(br, bg, bb)) => {
            let blend = |a: u8, b: u8| {
                (f64::from(a) + (f64::from(b) - f64::from(a)) * amount).round() as u8
            };
            Color::Rgb(blend(ar, br), blend(ag, bg), blend(ab, bb))
        }
        _ if amount < 0.5 => from,
        _ => to,
    }
}

pub fn meter(fraction: f64, cells: usize, ramp: &Ramp, track: Style) -> Line<'static> {
    let mut spans = Vec::with_capacity(cells);
    meter_into(&mut spans, fraction, cells, ramp, track);
    Line::from(spans)
}

pub fn meter_into(
    out: &mut Vec<Span<'static>>,
    fraction: f64,
    cells: usize,
    ramp: &Ramp,
    track: Style,
) {
    out.clear();
    let steps = floored(fraction, cells * 8);
    let (full, part) = (steps / 8, steps % 8);
    for cell in 0..cells {
        if cell < full || (cell == full && part > 0) {
            let glyph = if cell < full { 7 } else { part - 1 };
            let colour = ramp.at((cell as f64 + 0.5) / cells as f64);
            out.push(Span::styled(LADDER_STR[glyph], Style::new().fg(colour)));
        } else {
            out.push(Span::styled(TRACK_STR, track));
        }
    }
}

pub fn percent(fraction: f64) -> u32 {
    floored(fraction, 100) as u32
}

pub fn share(fraction: f64, estimate: bool) -> String {
    let about = if estimate { ABOUT } else { "" };
    format!("{about}{}%", percent(fraction))
}

pub fn spinner(gone: Duration) -> &'static str {
    let frame = gone.as_nanos() / SPIN.as_nanos();
    SPINNER[(frame % SPINNER.len() as u128) as usize]
}

#[cfg(test)]
mod tests {
    use super::*;

    fn chars(text: &str) -> usize {
        text.chars().count()
    }

    const RAMP: Ramp = Ramp::load();

    fn colours(line: &Line<'_>) -> Vec<Option<Color>> {
        line.spans.iter().map(|span| span.style.fg).collect()
    }

    fn glyphs(line: &Line<'_>) -> String {
        line.spans
            .iter()
            .map(|span| span.content.as_ref())
            .collect()
    }

    #[test]
    fn a_meter_draws_the_bar_glyphs_with_a_quiet_track() {
        let track = Style::new().fg(Color::DarkGray);
        for step in 0..=20 {
            let fraction = f64::from(step) / 20.0;
            let line = meter(fraction, 8, &RAMP, track);
            assert_eq!(glyphs(&line), bar(fraction, 8), "{fraction}");
        }
    }

    #[test]
    fn a_meter_at_zero_is_all_track_and_keeps_its_style() {
        let track = Style::new().fg(Color::DarkGray);
        let line = meter(0.0, 6, &RAMP, track);
        assert!(line.spans.iter().all(|span| span.style == track));
        assert_eq!(meter(0.0, 0, &RAMP, track).spans.len(), 0);
    }

    #[test]
    fn a_meter_at_a_third_stays_cool_and_the_track_keeps_its_style() {
        let track = Style::new().fg(Color::DarkGray);
        let line = meter(0.3, 10, &RAMP, track);
        let got = colours(&line);
        for (cell, colour) in got.iter().enumerate().take(3) {
            let want = RAMP.at((cell as f64 + 0.5) / 10.0);
            assert_eq!(*colour, Some(want), "cell {cell}");
        }
        assert_eq!(got[0], Some(RAMP.at(0.05)));
        for span in &line.spans[3..] {
            assert_eq!(span.style, track);
        }
        assert_ne!(got[2], Some(RAMP.at(1.0)));
    }

    #[test]
    fn a_meter_at_seventy_percent_reaches_amber_and_full_reaches_red() {
        let track = Style::new();
        let line = meter(0.7, 10, &RAMP, track);
        let got = colours(&line);
        for (cell, colour) in got.iter().enumerate().take(7) {
            assert_eq!(*colour, Some(RAMP.at((cell as f64 + 0.5) / 10.0)));
        }
        assert_eq!(line.spans[7].style, track);
        let full = colours(&meter(1.0, 10, &RAMP, track));
        assert!(full.iter().all(Option::is_some));
        assert_eq!(full[0], Some(RAMP.at(0.05)));
        assert_eq!(full[9], Some(RAMP.at(0.95)));
        let Some(Color::Rgb(r, g, _)) = full[9] else {
            panic!("rgb");
        };
        assert!(r > 0xe0 && g < 0x60);
    }

    #[test]
    fn a_short_bar_stays_cool_and_a_long_one_runs_hot() {
        let teal = Color::Rgb(0x2d, 0xd4, 0xbf);
        assert_eq!(RAMP.at(0.0), teal);
        assert_eq!(RAMP.at(1.0), Color::Rgb(0xef, 0x44, 0x44));
        assert_eq!(RAMP.at(1.0 / 3.0), Color::Rgb(0x4a, 0xde, 0x80));
        assert_eq!(RAMP.at(2.0 / 3.0), Color::Rgb(0xfb, 0xbf, 0x24));
        let mid = RAMP.at(1.0 / 6.0);
        assert_eq!(mid, Color::Rgb(0x3c, 0xd9, 0xa0));
    }

    #[test]
    fn a_ramp_takes_two_to_four_stops_and_clamps_the_position() {
        let two = Ramp::new(&[Color::Rgb(0, 0, 0), Color::Rgb(100, 200, 50)]);
        assert_eq!(two.at(0.5), Color::Rgb(50, 100, 25));
        assert_eq!(two.at(-3.0), Color::Rgb(0, 0, 0));
        assert_eq!(two.at(9.0), Color::Rgb(100, 200, 50));
        assert_eq!(two.at(f64::NAN), Color::Rgb(0, 0, 0));
        let many = Ramp::new(&[Color::Red; 7]);
        assert_eq!(many.at(1.0), Color::Red);
        let accent = Ramp::from_accent(Color::Rgb(200, 100, 50));
        assert_eq!(accent.at(0.5), Color::Rgb(200, 100, 50));
        assert!(accent.at(0.0) != accent.at(1.0));
    }

    #[test]
    fn a_ramp_from_the_palette_starts_at_its_accent_tone() {
        let palette = crate::Palette::new(Color::Rgb(200, 100, 50));
        assert_eq!(Ramp::accent(&palette).at(0.5), Color::Rgb(200, 100, 50));
    }

    #[test]
    fn an_empty_bar_is_all_track_and_a_full_bar_is_all_full_cells() {
        assert_eq!(bar(0.0, 4), "⣀⣀⣀⣀");
        assert_eq!(bar(1.0, 4), "⣿⣿⣿⣿");
        assert_eq!(bar(0.0, 0), "");
        assert_eq!(bar(1.0, 0), "");
        assert_eq!(bar(0.5, 0), "");
    }

    #[test]
    fn a_partial_bar_climbs_the_ladder_one_eighth_of_a_cell_at_a_time() {
        assert_eq!(bar(0.5, 4), "⣿⣿⣀⣀");
        assert_eq!(bar(1.0 / 32.0, 4), "⡀⣀⣀⣀");
        assert_eq!(bar(0.25, 1), "⡄");
        assert_eq!(bar(0.875, 1), "⣷");
        assert_eq!(bar(0.124, 1), "⣀");
        assert_eq!(bar(0.125, 1), "⡀");
        assert_eq!(bar(0.999, 1), "⣷");
        for eighth in 1..8 {
            assert_eq!(
                bar(f64::from(eighth) / 8.0, 1),
                LADDER[eighth as usize - 1].to_string()
            );
        }
    }

    #[test]
    fn out_of_range_fractions_are_clamped_and_nan_is_empty() {
        assert_eq!(bar(1.5, 3), bar(1.0, 3));
        assert_eq!(bar(f64::INFINITY, 3), bar(1.0, 3));
        assert_eq!(bar(-0.5, 3), bar(0.0, 3));
        assert_eq!(bar(f64::NEG_INFINITY, 3), bar(0.0, 3));
        assert_eq!(bar(f64::NAN, 3), bar(0.0, 3));
    }

    #[test]
    fn a_bar_is_always_exactly_its_cells_wide_and_never_shrinks_as_it_grows() {
        for cells in 0..=12 {
            let mut last = 0;
            for step in 0..=200 {
                let fraction = f64::from(step) / 200.0;
                let drawn = bar(fraction, cells);
                assert_eq!(chars(&drawn), cells, "{fraction} over {cells}");
                let full = drawn.chars().filter(|&glyph| glyph == '⣿').count();
                assert!(full >= last, "{fraction} over {cells}");
                last = full;
            }
            assert_eq!(last, cells);
        }
    }

    #[test]
    fn percent_floors_and_clamps() {
        assert_eq!(percent(0.0), 0);
        assert_eq!(percent(0.5), 50);
        assert_eq!(percent(0.999), 99);
        assert_eq!(percent(1.0), 100);
        assert_eq!(percent(2.0), 100);
        assert_eq!(percent(-1.0), 0);
        assert_eq!(percent(f64::NAN), 0);
        assert_eq!(percent(f64::INFINITY), 100);
    }

    #[test]
    fn percent_of_every_whole_hundredth_is_that_whole_number() {
        for whole in 0..=100 {
            assert_eq!(percent(f64::from(whole) / 100.0), whole, "{whole}%");
        }
    }

    #[test]
    fn a_huge_count_a_few_units_short_is_never_shown_done() {
        let total = 100_000_000_000_u64;
        for left in [1, 3, 1_000] {
            let fraction = (total - left) as f64 / total as f64;
            assert!(fraction < 1.0);
            assert_eq!(percent(fraction), 99, "{left} left");
            assert_eq!(share(fraction, false), "99%");
            assert_eq!(eighths(&bar(fraction, 10)), 79, "{left} left");
            assert_ne!(bar(fraction, 10), bar(1.0, 10));
        }
        assert_eq!(percent(1.0 - 1e-11), 99);
        assert_eq!(percent(total as f64 / total as f64), 100);
    }

    #[test]
    fn a_fraction_from_a_division_floors_to_its_exact_step() {
        for count in [3_u64, 7, 25, 100, 1_000, 4_096, 1_000_000] {
            for done in 0..=count.min(500) {
                let fraction = done as f64 / count as f64;
                let exact = (done * 100 / count) as u32;
                assert_eq!(percent(fraction), exact, "{done}/{count}");
            }
        }
    }

    #[test]
    fn share_of_a_count_over_a_hundred_prints_the_count() {
        assert_eq!(share(57.0 / 100.0, false), "57%");
        assert_eq!(share(29.0 / 100.0, false), "29%");
        assert_eq!(share(58.0 / 100.0, true), "≈58%");
    }

    fn eighths(drawn: &str) -> usize {
        drawn
            .chars()
            .map(|glyph| {
                LADDER
                    .iter()
                    .position(|&step| step == glyph)
                    .map_or(0, |step| step + 1)
            })
            .sum()
    }

    #[test]
    fn a_bar_at_an_exact_number_of_eighths_draws_exactly_that_many() {
        for cells in 1..=12 {
            let total = cells * 8;
            for steps in 0..=total {
                let fraction = steps as f64 / total as f64;
                let drawn = bar(fraction, cells);
                assert_eq!(eighths(&drawn), steps, "{steps}/{total}");
            }
        }
        assert_eq!(eighths(&bar(15.0 / 88.0, 11)), 15);
    }

    #[test]
    fn share_marks_an_estimate_with_the_about_sign() {
        assert_eq!(share(0.0, false), "0%");
        assert_eq!(share(0.5, false), "50%");
        assert_eq!(share(0.5, true), "≈50%");
        assert_eq!(share(1.0, true), "≈100%");
        assert_eq!(share(3.0, false), "100%");
        assert_eq!(share(f64::NAN, true), "≈0%");
    }

    #[test]
    fn the_spinner_steps_one_frame_per_spin_and_wraps() {
        assert_eq!(spinner(Duration::ZERO), SPINNER[0]);
        assert_eq!(spinner(SPIN - Duration::from_nanos(1)), SPINNER[0]);
        assert_eq!(spinner(SPIN), SPINNER[1]);
        assert_eq!(spinner(SPIN * 9), SPINNER[9]);
        assert_eq!(spinner(SPIN * 10), SPINNER[0]);
        assert_eq!(spinner(SPIN * 23), SPINNER[3]);
        assert!(SPINNER.contains(&spinner(Duration::MAX)));
    }

    #[test]
    fn a_full_spinner_cycle_shows_every_frame_once() {
        let shown: Vec<&str> = (0..10).map(|frame| spinner(SPIN * frame)).collect();
        assert_eq!(shown, SPINNER);
    }
}
