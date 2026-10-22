use std::borrow::Cow;
use std::time::{Duration, Instant};

const MINUTE: u64 = 60;
const HOUR: u64 = 60 * MINUTE;
const DAY: u64 = 24 * HOUR;
const YEAR: u64 = 365 * DAY;

type Word = Cow<'static, str>;

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Units {
    pub second: Word,
    pub minute: Word,
    pub hour: Word,
    pub day: Word,
    pub year: Word,
    pub under: Word,
    pub now: Word,
}

impl Units {
    pub fn new(
        second: impl Into<Word>,
        minute: impl Into<Word>,
        hour: impl Into<Word>,
        day: impl Into<Word>,
    ) -> Self {
        Units {
            second: second.into(),
            minute: minute.into(),
            hour: hour.into(),
            day: day.into(),
            year: Word::Borrowed(""),
            under: Word::Borrowed(""),
            now: Word::Borrowed(""),
        }
    }

    pub fn year(mut self, word: impl Into<Word>) -> Self {
        self.year = word.into();
        self
    }

    pub fn under(mut self, words: impl Into<Word>) -> Self {
        self.under = words.into();
        self
    }

    pub fn now(mut self, words: impl Into<Word>) -> Self {
        self.now = words.into();
        self
    }
}

pub fn span(took: Duration, units: &Units) -> String {
    spanned(took, units, true)
}

pub fn span_hours(took: Duration, units: &Units) -> String {
    spanned(took, units, false)
}

fn spanned(took: Duration, units: &Units, days: bool) -> String {
    let seconds = took.as_secs();
    if seconds == 0 && !units.under.is_empty() {
        return units.under.to_string();
    }
    if seconds < MINUTE {
        return format!("{seconds}{}", units.second);
    }
    let (big, big_unit, small, small_unit) = if seconds < HOUR {
        (
            seconds / MINUTE,
            &units.minute,
            seconds % MINUTE,
            &units.second,
        )
    } else if seconds < DAY || !days {
        (
            seconds / HOUR,
            &units.hour,
            seconds % HOUR / MINUTE,
            &units.minute,
        )
    } else {
        (seconds / DAY, &units.day, seconds % DAY / HOUR, &units.hour)
    };
    format!("{big}{big_unit} {small}{small_unit}")
}

pub fn age(gone: Duration, units: &Units) -> String {
    let seconds = gone.as_secs();
    if seconds < MINUTE && !units.now.is_empty() {
        units.now.to_string()
    } else if seconds < MINUTE {
        format!("{seconds}{}", units.second)
    } else if seconds < HOUR {
        format!("{}{}", seconds / MINUTE, units.minute)
    } else if seconds < 2 * DAY {
        format!("{}{}", seconds / HOUR, units.hour)
    } else if seconds < YEAR || units.year.is_empty() {
        format!("{}{}", seconds / DAY, units.day)
    } else {
        format!("{}{}", seconds / YEAR, units.year)
    }
}

fn next(since: Instant, now: Instant, step: u64) -> Instant {
    let gone = now.saturating_duration_since(since).as_secs();
    since + Duration::from_secs((gone / step + 1) * step)
}

pub fn next_span(since: Instant, now: Instant) -> Instant {
    let gone = now.saturating_duration_since(since).as_secs();
    let step = if gone < HOUR { 1 } else { MINUTE };
    next(since, now, step)
}

pub fn next_age(since: Instant, now: Instant) -> Instant {
    let gone = now.saturating_duration_since(since).as_secs();
    let step = if gone < MINUTE {
        1
    } else if gone < HOUR {
        MINUTE
    } else if gone < 2 * DAY {
        HOUR
    } else {
        DAY
    };
    next(since, now, step)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn spans_and_ages_take_the_apps_words_at_each_end() {
        let units = Units::new("s", "m", "h", "d")
            .year(" y")
            .under("under 1s")
            .now("just now");
        let secs = Duration::from_secs;
        assert_eq!(span(Duration::from_millis(400), &units), "under 1s");
        assert_eq!(span(secs(62), &units), "1m 2s");
        assert_eq!(span_hours(secs(49 * 3600 + 180), &units), "49h 3m");
        assert_eq!(span(secs(49 * 3600 + 180), &units), "2d 1h");
        assert_eq!(age(secs(59), &units), "just now");
        assert_eq!(age(secs(3 * 86_400), &units), "3d");
        assert_eq!(age(secs(800 * 86_400), &units), "2 y");
        let plain = Units::new("s", "m", "h", "d");
        assert_eq!(span(Duration::ZERO, &plain), "0s");
        assert_eq!(age(secs(800 * 86_400), &plain), "800d");
    }

    fn secs(seconds: u64) -> Duration {
        Duration::from_secs(seconds)
    }

    fn plain() -> Units {
        Units::new("s", "m", "h", "d")
    }

    fn worded() -> Units {
        plain().year("y").under("<1s").now("now")
    }

    #[test]
    fn units_start_without_year_under_or_now_words() {
        let units = plain();
        assert_eq!(units.second, "s");
        assert_eq!(units.minute, "m");
        assert_eq!(units.hour, "h");
        assert_eq!(units.day, "d");
        assert_eq!(units.year, "");
        assert_eq!(units.under, "");
        assert_eq!(units.now, "");
        let owned = Units::new(String::from(" sec"), " min", " hr", " day");
        assert_eq!(span(secs(61), &owned), "1 min 1 sec");
    }

    #[test]
    fn spans_switch_units_exactly_at_each_boundary() {
        let units = plain();
        let cases = [
            (0, "0s"),
            (1, "1s"),
            (59, "59s"),
            (60, "1m 0s"),
            (61, "1m 1s"),
            (3_599, "59m 59s"),
            (3_600, "1h 0m"),
            (3_659, "1h 0m"),
            (3_660, "1h 1m"),
            (86_399, "23h 59m"),
            (86_400, "1d 0h"),
            (90_000, "1d 1h"),
            (172_800, "2d 0h"),
            (400 * 86_400, "400d 0h"),
        ];
        for (seconds, label) in cases {
            assert_eq!(span(secs(seconds), &units), label, "{seconds}s");
        }
    }

    #[test]
    fn span_hours_never_rolls_into_days() {
        let units = plain();
        assert_eq!(span_hours(secs(59), &units), "59s");
        assert_eq!(span_hours(secs(3_600), &units), "1h 0m");
        assert_eq!(span_hours(secs(86_399), &units), "23h 59m");
        assert_eq!(span_hours(secs(86_400), &units), "24h 0m");
        assert_eq!(span_hours(secs(10 * 86_400 + 60), &units), "240h 1m");
    }

    #[test]
    fn spans_drop_fractions_of_a_second_and_name_the_first_second_with_under() {
        let units = worded();
        assert_eq!(span(Duration::ZERO, &units), "<1s");
        assert_eq!(span(Duration::from_millis(999), &units), "<1s");
        assert_eq!(span(Duration::from_millis(1_000), &units), "1s");
        assert_eq!(span(Duration::from_millis(59_999), &units), "59s");
        assert_eq!(span_hours(Duration::from_millis(1), &units), "<1s");
        assert_eq!(span(Duration::from_millis(999), &plain()), "0s");
    }

    #[test]
    fn the_longest_span_still_renders() {
        let label = span(Duration::MAX, &plain());
        assert!(label.ends_with('h'), "{label}");
        let label = span_hours(Duration::MAX, &plain());
        assert!(label.ends_with('m'), "{label}");
        let label = age(Duration::MAX, &worded());
        assert!(label.ends_with('y'), "{label}");
    }

    #[test]
    fn ages_switch_units_exactly_at_each_boundary() {
        let units = plain();
        let cases = [
            (0, "0s"),
            (59, "59s"),
            (60, "1m"),
            (119, "1m"),
            (3_599, "59m"),
            (3_600, "1h"),
            (86_400, "24h"),
            (172_799, "47h"),
            (172_800, "2d"),
            (364 * 86_400, "364d"),
            (365 * 86_400, "365d"),
            (800 * 86_400, "800d"),
        ];
        for (seconds, label) in cases {
            assert_eq!(age(secs(seconds), &units), label, "{seconds}s");
        }
    }

    #[test]
    fn ages_under_a_minute_read_as_now_and_years_count_once_named() {
        let units = worded();
        assert_eq!(age(Duration::ZERO, &units), "now");
        assert_eq!(age(secs(59), &units), "now");
        assert_eq!(age(Duration::from_millis(59_999), &units), "now");
        assert_eq!(age(secs(60), &units), "1m");
        assert_eq!(age(secs(365 * 86_400 - 1), &units), "364d");
        assert_eq!(age(secs(365 * 86_400), &units), "1y");
        assert_eq!(age(secs(730 * 86_400 - 1), &units), "1y");
        assert_eq!(age(secs(730 * 86_400), &units), "2y");
    }

    #[test]
    fn the_next_span_redraw_comes_on_the_next_whole_second_then_minute() {
        let since = Instant::now();
        let at = |gone: Duration| next_span(since, since + gone) - since;
        assert_eq!(at(Duration::ZERO), secs(1));
        assert_eq!(at(Duration::from_millis(500)), secs(1));
        assert_eq!(at(secs(1)), secs(2));
        assert_eq!(at(secs(59)), secs(60));
        assert_eq!(at(secs(3_599)), secs(3_600));
        assert_eq!(at(secs(3_600)), secs(3_660));
        assert_eq!(at(secs(3_659)), secs(3_660));
        assert_eq!(at(secs(90_000)), secs(90_060));
    }

    #[test]
    fn the_next_age_redraw_steps_by_second_minute_hour_then_day() {
        let since = Instant::now();
        let at = |gone: Duration| next_age(since, since + gone) - since;
        assert_eq!(at(Duration::ZERO), secs(1));
        assert_eq!(at(secs(59)), secs(60));
        assert_eq!(at(secs(60)), secs(120));
        assert_eq!(at(secs(3_599)), secs(3_600));
        assert_eq!(at(secs(3_600)), secs(7_200));
        assert_eq!(at(secs(172_799)), secs(172_800));
        assert_eq!(at(secs(172_800)), secs(259_200));
        assert_eq!(at(secs(259_199)), secs(259_200));
    }

    #[test]
    fn a_clock_behind_its_start_waits_for_the_first_second() {
        let now = Instant::now();
        let since = now + secs(10);
        assert_eq!(next_span(since, now), since + secs(1));
        assert_eq!(next_age(since, now), since + secs(1));
    }

    #[test]
    fn the_label_holds_until_the_next_redraw_and_changes_at_it() {
        let since = Instant::now();
        let units = plain();
        let just_before = Duration::from_millis(1);
        for gone in [0, 1, 59, 60, 61, 3_599, 3_600, 3_601, 86_399, 172_799, 172_800, 300_000] {
            let now = since + secs(gone);
            let shown = age(secs(gone), &units);
            let next = next_age(since, now) - since;
            assert_eq!(age(next - just_before, &units), shown, "age at {gone}s");
            assert_ne!(age(next, &units), shown, "age at {gone}s");
            if gone >= DAY {
                continue;
            }
            let shown = span(secs(gone), &units);
            let next = next_span(since, now) - since;
            assert_eq!(span(next - just_before, &units), shown, "span at {gone}s");
            assert_ne!(span(next, &units), shown, "span at {gone}s");
        }
    }
}
