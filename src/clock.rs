use std::borrow::Cow;
use std::time::{Duration, Instant};

const MINUTE: u64 = 60;
const HOUR: u64 = 60 * MINUTE;
const DAY: u64 = 24 * HOUR;

type Word = Cow<'static, str>;

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Units {
    pub second: Word,
    pub minute: Word,
    pub hour: Word,
    pub day: Word,
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
        }
    }
}

pub fn span(took: Duration, units: &Units) -> String {
    let seconds = took.as_secs();
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
    } else if seconds < DAY {
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
    if seconds < MINUTE {
        format!("{seconds}{}", units.second)
    } else if seconds < HOUR {
        format!("{}{}", seconds / MINUTE, units.minute)
    } else if seconds < 2 * DAY {
        format!("{}{}", seconds / HOUR, units.hour)
    } else {
        format!("{}{}", seconds / DAY, units.day)
    }
}

fn next(since: Instant, now: Instant, step: u64) -> Instant {
    let gone = now.saturating_duration_since(since).as_secs();
    since + Duration::from_secs((gone / step + 1) * step)
}

pub fn next_span(since: Instant, now: Instant) -> Instant {
    let gone = now.saturating_duration_since(since).as_secs();
    let step = if gone < HOUR {
        1
    } else if gone < DAY {
        MINUTE
    } else {
        HOUR
    };
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
