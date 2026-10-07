use std::time::{Duration, Instant};

pub const FRAME: Duration = Duration::from_nanos(8_333_333);

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Pace {
    last: Option<Instant>,
}

impl Pace {
    pub fn ready(&self, now: Instant) -> bool {
        self.last.is_none_or(|last| now >= last + FRAME)
    }

    pub fn next(&self, now: Instant) -> Instant {
        self.last.map_or(now, |last| (last + FRAME).max(now))
    }

    pub fn drew(&mut self, at: Instant) {
        self.last = Some(at);
    }
}

pub fn wait_until(
    now: Instant,
    dirty: bool,
    pace: &Pace,
    deadlines: &[Option<Instant>],
) -> Option<Instant> {
    if dirty {
        return Some(pace.next(now));
    }
    deadlines.iter().flatten().min().copied()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frames_are_paced_by_time_at_120_per_second_at_most() {
        let start = Instant::now();
        let mut pace = Pace::default();
        assert!(pace.ready(start));
        assert_eq!(pace.next(start), start);
        pace.drew(start);
        assert!(!pace.ready(start + Duration::from_millis(5)));
        assert_eq!(
            pace.next(start + Duration::from_millis(5)),
            start + FRAME,
            "a frame that took 5 ms waits only the rest of its 8.3 ms"
        );
        assert!(pace.ready(start + FRAME));
        let late = start + Duration::from_millis(20);
        assert_eq!(pace.next(late), late, "a slow frame is followed at once");
        assert_eq!(FRAME.as_secs_f64().recip().round(), 120.0);
    }

    #[test]
    fn idle_waits_for_a_wake_or_the_next_deadline_alone() {
        let start = Instant::now();
        let mut pace = Pace::default();
        pace.drew(start);
        assert_eq!(wait_until(start, false, &pace, &[None, None]), None);
        let soon = start + Duration::from_secs(3);
        let later = start + Duration::from_secs(9);
        assert_eq!(
            wait_until(start, false, &pace, &[Some(later), Some(soon)]),
            Some(soon)
        );
        assert_eq!(
            wait_until(start, true, &pace, &[Some(later)]),
            Some(start + FRAME),
            "an animation waits one frame"
        );
    }
}
