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

    #[test]
    fn a_frame_is_one_hundred_and_twentieth_of_a_second_rounded_down() {
        assert!(FRAME * 120 <= Duration::from_secs(1));
        assert!(FRAME * 121 > Duration::from_secs(1));
    }

    #[test]
    fn the_next_frame_is_due_exactly_one_frame_after_the_last_draw() {
        let start = Instant::now();
        let mut pace = Pace::default();
        pace.drew(start);
        let one_short = start + FRAME - Duration::from_nanos(1);
        assert!(!pace.ready(one_short));
        assert_eq!(pace.next(one_short), start + FRAME);
        assert!(pace.ready(start + FRAME));
        assert_eq!(pace.next(start + FRAME), start + FRAME);
        assert_eq!(pace.next(start), start + FRAME);
    }

    #[test]
    fn drawing_as_soon_as_ready_keeps_frames_exactly_one_frame_apart() {
        let start = Instant::now();
        let mut pace = Pace::default();
        let mut now = start;
        let mut frames = 0u32;
        while now < start + Duration::from_secs(1) {
            assert!(pace.ready(now));
            assert_eq!(now - start, FRAME * frames);
            pace.drew(now);
            frames += 1;
            now = pace.next(now);
        }
        assert_eq!(frames, 121);
    }

    #[test]
    fn a_late_draw_restarts_the_pace_from_that_draw() {
        let start = Instant::now();
        let mut pace = Pace::default();
        pace.drew(start);
        let late = start + Duration::from_millis(50);
        pace.drew(late);
        assert!(!pace.ready(late + Duration::from_millis(1)));
        assert_eq!(pace.next(late), late + FRAME);
    }

    #[test]
    fn a_fresh_pace_is_ready_at_once() {
        let now = Instant::now();
        let pace = Pace::default();
        assert!(pace.ready(now));
        assert_eq!(pace.next(now), now);
        assert_eq!(wait_until(now, true, &pace, &[]), Some(now));
        assert_eq!(pace, Pace::default());
    }

    #[test]
    fn a_deadline_already_past_is_returned_so_the_loop_does_not_block() {
        let start = Instant::now();
        let now = start + Duration::from_secs(10);
        let mut pace = Pace::default();
        pace.drew(now);
        let past = start + Duration::from_secs(2);
        let woke = wait_until(now, false, &pace, &[Some(now + Duration::from_secs(5)), Some(past)]);
        assert_eq!(woke, Some(past));
        assert!(woke.is_some_and(|at| at <= now));
        assert_eq!(wait_until(now, false, &pace, &[Some(now)]), Some(now));
    }

    #[test]
    fn idle_with_no_deadlines_blocks_on_the_wake_channel_alone() {
        let now = Instant::now();
        let pace = Pace::default();
        assert_eq!(wait_until(now, false, &pace, &[]), None);
        assert_eq!(wait_until(now, false, &pace, &[None]), None);
    }

    #[test]
    fn a_dirty_screen_waits_for_its_frame_whatever_the_deadlines() {
        let start = Instant::now();
        let mut pace = Pace::default();
        pace.drew(start);
        let now = start + Duration::from_millis(2);
        assert_eq!(
            wait_until(now, true, &pace, &[Some(start), None]),
            Some(start + FRAME)
        );
        assert_eq!(wait_until(now, true, &pace, &[]), Some(start + FRAME));
    }
}
