use std::time::{Duration, Instant};

use crossterm::event::{Event, KeyCode, KeyEvent, KeyEventKind, KeyboardEnhancementFlags};

pub(crate) const WHEEL_PAIR: Duration = Duration::from_millis(4);

pub(crate) const KEY_FLAGS: KeyboardEnhancementFlags =
    KeyboardEnhancementFlags::DISAMBIGUATE_ESCAPE_CODES
        .union(KeyboardEnhancementFlags::REPORT_EVENT_TYPES);

#[derive(Default)]
pub(crate) struct WheelFilter {
    held: Option<(KeyEvent, Instant)>,
}

impl WheelFilter {
    pub(crate) fn feed(&mut self, ev: Event, at: Instant, out: &mut Vec<Event>) {
        self.take(ev, Some(at), out);
    }

    pub(crate) fn pass(&mut self, ev: Event, out: &mut Vec<Event>) {
        self.take(ev, None, out);
    }

    fn take(&mut self, ev: Event, at: Option<Instant>, out: &mut Vec<Event>) {
        let Event::Key(key) = ev else {
            self.flush(out);
            out.push(ev);
            return;
        };
        match key.kind {
            KeyEventKind::Release => {
                if let Some((held, since)) = self.held.take()
                    && !(held.code == key.code
                        && at.is_some_and(|at| at.saturating_duration_since(since) < WHEEL_PAIR))
                {
                    out.push(Event::Key(held));
                }
            }
            KeyEventKind::Press if at.is_some() && scrolls(key.code) => {
                self.flush(out);
                self.held = at.map(|at| (key, at));
            }
            KeyEventKind::Repeat => {
                self.flush(out);
                out.push(Event::Key(KeyEvent {
                    kind: KeyEventKind::Press,
                    ..key
                }));
            }
            KeyEventKind::Press => {
                self.flush(out);
                out.push(ev);
            }
        }
    }

    pub(crate) fn deadline(&self) -> Option<Instant> {
        self.held.map(|(_, since)| since + WHEEL_PAIR)
    }

    pub(crate) fn expire(&mut self, now: Instant, out: &mut Vec<Event>) {
        if self.deadline().is_some_and(|due| now >= due) {
            self.flush(out);
        }
    }

    fn flush(&mut self, out: &mut Vec<Event>) {
        if let Some((held, _)) = self.held.take() {
            out.push(Event::Key(held));
        }
    }
}

fn scrolls(code: KeyCode) -> bool {
    matches!(code, KeyCode::Up | KeyCode::Down)
}

#[cfg(test)]
mod tests {
    use crossterm::event::{KeyEventState, KeyModifiers};

    use super::*;

    fn key(code: KeyCode, kind: KeyEventKind) -> Event {
        Event::Key(KeyEvent::new_with_kind_and_state(
            code,
            KeyModifiers::NONE,
            kind,
            KeyEventState::empty(),
        ))
    }

    fn run(events: &[(u64, Event)], end: u64) -> Vec<Event> {
        let start = Instant::now();
        let at = |micros: u64| start + Duration::from_micros(micros);
        let mut filter = WheelFilter::default();
        let mut out = Vec::new();
        for (micros, ev) in events {
            filter.expire(at(*micros), &mut out);
            filter.feed(ev.clone(), at(*micros), &mut out);
        }
        filter.expire(at(end), &mut out);
        assert!(filter.held.is_none(), "{events:?}");
        out
    }

    #[test]
    fn a_wheel_burst_gives_no_key() {
        let mut events = Vec::new();
        for step in 0..5 {
            events.push((step * 200, key(KeyCode::Up, KeyEventKind::Press)));
            events.push((step * 200 + 50, key(KeyCode::Up, KeyEventKind::Release)));
        }
        assert!(run(&events, 100_000).is_empty());
    }

    #[test]
    fn a_real_up_gives_one_up() {
        let events = [
            (0, key(KeyCode::Up, KeyEventKind::Press)),
            (80_000, key(KeyCode::Up, KeyEventKind::Release)),
        ];
        assert_eq!(
            run(&events, 100_000),
            vec![key(KeyCode::Up, KeyEventKind::Press)]
        );
    }

    #[test]
    fn a_held_down_gives_the_press_and_every_repeat_as_a_press() {
        let mut events = vec![(0, key(KeyCode::Down, KeyEventKind::Press))];
        for step in 1..=4 {
            events.push((
                300_000 + step * 30_000,
                key(KeyCode::Down, KeyEventKind::Repeat),
            ));
        }
        events.push((500_000, key(KeyCode::Down, KeyEventKind::Release)));
        let want = vec![key(KeyCode::Down, KeyEventKind::Press); 5];
        assert_eq!(run(&events, 600_000), want);
    }

    #[test]
    fn a_press_followed_by_another_key_forwards_both_in_order() {
        let events = [
            (0, key(KeyCode::Down, KeyEventKind::Press)),
            (1_000, key(KeyCode::Char('j'), KeyEventKind::Press)),
        ];
        assert_eq!(
            run(&events, 100_000),
            vec![
                key(KeyCode::Down, KeyEventKind::Press),
                key(KeyCode::Char('j'), KeyEventKind::Press),
            ]
        );
        let events = [
            (0, key(KeyCode::Up, KeyEventKind::Press)),
            (1_000, key(KeyCode::Down, KeyEventKind::Press)),
            (2_000, key(KeyCode::Up, KeyEventKind::Release)),
        ];
        assert_eq!(
            run(&events, 100_000),
            vec![
                key(KeyCode::Up, KeyEventKind::Press),
                key(KeyCode::Down, KeyEventKind::Press),
            ]
        );
    }

    #[test]
    fn a_lone_release_gives_nothing() {
        for code in [KeyCode::Up, KeyCode::Down, KeyCode::Char('q'), KeyCode::Esc] {
            assert!(run(&[(0, key(code, KeyEventKind::Release))], 100_000).is_empty());
        }
    }

    #[test]
    fn a_press_waits_at_most_the_pair_window() {
        let start = Instant::now();
        let mut filter = WheelFilter::default();
        let mut out = Vec::new();
        filter.feed(key(KeyCode::Up, KeyEventKind::Press), start, &mut out);
        assert!(out.is_empty());
        assert_eq!(filter.deadline(), Some(start + WHEEL_PAIR));
        filter.expire(start + WHEEL_PAIR / 2, &mut out);
        assert!(out.is_empty());
        filter.expire(start + WHEEL_PAIR, &mut out);
        assert_eq!(out, vec![key(KeyCode::Up, KeyEventKind::Press)]);
        assert_eq!(filter.deadline(), None);
    }

    #[test]
    fn other_keys_and_events_pass_at_once() {
        let events = [
            (0, key(KeyCode::Char('j'), KeyEventKind::Press)),
            (10, Event::Resize(80, 24)),
            (20, key(KeyCode::Left, KeyEventKind::Press)),
        ];
        let start = Instant::now();
        let mut filter = WheelFilter::default();
        let mut out = Vec::new();
        for (micros, ev) in events.iter().cloned() {
            filter.feed(ev, start + Duration::from_micros(micros), &mut out);
        }
        assert_eq!(filter.deadline(), None);
        assert_eq!(out, events.map(|(_, ev)| ev).to_vec());
    }

    #[test]
    fn without_the_flags_every_press_passes_at_once_and_releases_drop() {
        let start = Instant::now();
        let mut filter = WheelFilter::default();
        let mut out = Vec::new();
        filter.feed(key(KeyCode::Down, KeyEventKind::Press), start, &mut out);
        for ev in [
            key(KeyCode::Up, KeyEventKind::Press),
            key(KeyCode::Up, KeyEventKind::Release),
            key(KeyCode::Down, KeyEventKind::Repeat),
            key(KeyCode::Char('q'), KeyEventKind::Release),
        ] {
            filter.pass(ev, &mut out);
        }
        assert_eq!(filter.deadline(), None);
        assert_eq!(
            out,
            vec![
                key(KeyCode::Down, KeyEventKind::Press),
                key(KeyCode::Up, KeyEventKind::Press),
                key(KeyCode::Down, KeyEventKind::Press),
            ]
        );
    }
}
