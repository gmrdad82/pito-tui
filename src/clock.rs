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
}
