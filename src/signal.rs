type Ring = Box<dyn Fn(i32) + Send>;

pub(crate) struct Live;

pub(crate) fn live(ring: impl Fn(i32) + Send + 'static) -> Live {
    imp::live(Box::new(ring));
    Live
}

impl Live {
    pub(crate) fn end(self) -> Option<i32> {
        imp::quiet()
    }
}

impl Drop for Live {
    fn drop(&mut self) {
        imp::quiet();
    }
}

pub(crate) fn die(signal: i32) -> ! {
    imp::die(signal)
}

#[cfg(unix)]
mod imp {
    use std::process;
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
    use std::sync::{Arc, Mutex, MutexGuard, OnceLock};
    use std::thread;

    use signal_hook::consts::{SIGHUP, SIGINT, SIGTERM};
    use signal_hook::iterator::Signals;
    use signal_hook::{flag, low_level};

    use super::Ring;

    const ENDS: [i32; 3] = [SIGHUP, SIGTERM, SIGINT];

    struct Watch {
        idle: Arc<AtomicBool>,
        caught: Arc<AtomicUsize>,
        ring: Mutex<Option<Ring>>,
    }

    static WATCH: OnceLock<Option<Watch>> = OnceLock::new();

    fn watch() -> Option<&'static Watch> {
        WATCH.get_or_init(install).as_ref()
    }

    fn install() -> Option<Watch> {
        let idle = Arc::new(AtomicBool::new(true));
        let caught = Arc::new(AtomicUsize::new(0));
        for signal in ENDS {
            flag::register_conditional_default(signal, Arc::clone(&idle)).ok()?;
            flag::register_usize(signal, Arc::clone(&caught), signal as usize).ok()?;
        }
        let mut signals = Signals::new(ENDS).ok()?;
        thread::spawn(move || {
            for signal in signals.forever() {
                if let Some(watch) = watch()
                    && let Some(ring) = lock(&watch.ring).as_ref()
                {
                    ring(signal);
                }
            }
        });
        Some(Watch {
            idle,
            caught,
            ring: Mutex::new(None),
        })
    }

    fn lock<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
        mutex
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
    }

    pub(super) fn live(ring: Ring) {
        let Some(watch) = watch() else {
            return;
        };
        *lock(&watch.ring) = Some(ring);
        watch.caught.store(0, Ordering::SeqCst);
        watch.idle.store(false, Ordering::SeqCst);
    }

    pub(super) fn quiet() -> Option<i32> {
        let watch = WATCH.get()?.as_ref()?;
        watch.idle.store(true, Ordering::SeqCst);
        *lock(&watch.ring) = None;
        match watch.caught.swap(0, Ordering::SeqCst) {
            0 => None,
            signal => i32::try_from(signal).ok(),
        }
    }

    pub(super) fn die(signal: i32) -> ! {
        let _ = low_level::emulate_default_handler(signal);
        process::exit(128 + signal)
    }
}

#[cfg(not(unix))]
mod imp {
    use super::Ring;

    pub(super) fn live(_ring: Ring) {}

    pub(super) fn quiet() -> Option<i32> {
        None
    }

    pub(super) fn die(signal: i32) -> ! {
        std::process::exit(128 + signal)
    }
}
