use std::time::Duration;

pub const SPIN: Duration = Duration::from_millis(80);
pub const SPINNER: [&str; 10] = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];
pub const ABOUT: &str = "≈";

const LADDER: [char; 8] = ['⡀', '⡄', '⡆', '⡇', '⣇', '⣧', '⣷', '⣿'];
const TRACK: char = '⣀';

fn clean(fraction: f64) -> f64 {
    if fraction.is_nan() {
        0.0
    } else {
        fraction.clamp(0.0, 1.0)
    }
}

pub fn bar(fraction: f64, cells: usize) -> String {
    let total = cells * 8;
    let steps = ((clean(fraction) * total as f64).floor() as usize).min(total);
    let (full, part) = (steps / 8, steps % 8);
    let mut text: String = std::iter::repeat_n(LADDER[7], full).collect();
    if part > 0 {
        text.push(LADDER[part - 1]);
    }
    let used = full + usize::from(part > 0);
    text.extend(std::iter::repeat_n(TRACK, cells.saturating_sub(used)));
    text
}

pub fn percent(fraction: f64) -> u32 {
    (clean(fraction) * 100.0).floor() as u32
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
