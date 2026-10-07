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
