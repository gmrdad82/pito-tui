use std::borrow::Cow;
use std::fmt;
use std::sync::Arc;

type Text = Cow<'static, str>;

#[derive(Clone)]
#[non_exhaustive]
pub struct Words {
    pub help: Text,
    pub again: Text,
    pub yes: Text,
    pub no: Text,
    pub choose: Text,
    pub leave: Text,
    pub too_small: Text,
    pub waiting: Text,
    pub busy: Arc<dyn Fn(usize) -> String + Send + Sync>,
}

impl Words {
    pub fn new() -> Self {
        Words {
            help: Cow::Borrowed(""),
            again: Cow::Borrowed(""),
            yes: Cow::Borrowed(""),
            no: Cow::Borrowed(""),
            choose: Cow::Borrowed(""),
            leave: Cow::Borrowed(""),
            too_small: Cow::Borrowed(""),
            waiting: Cow::Borrowed(""),
            busy: Arc::new(|_| String::new()),
        }
    }

    pub fn help(mut self, text: impl Into<Text>) -> Self {
        self.help = text.into();
        self
    }

    pub fn again(mut self, text: impl Into<Text>) -> Self {
        self.again = text.into();
        self
    }

    pub fn yes(mut self, text: impl Into<Text>) -> Self {
        self.yes = text.into();
        self
    }

    pub fn no(mut self, text: impl Into<Text>) -> Self {
        self.no = text.into();
        self
    }

    pub fn choose(mut self, text: impl Into<Text>) -> Self {
        self.choose = text.into();
        self
    }

    pub fn leave(mut self, text: impl Into<Text>) -> Self {
        self.leave = text.into();
        self
    }

    pub fn too_small(mut self, text: impl Into<Text>) -> Self {
        self.too_small = text.into();
        self
    }

    pub fn waiting(mut self, text: impl Into<Text>) -> Self {
        self.waiting = text.into();
        self
    }

    pub fn busy(mut self, words: impl Fn(usize) -> String + Send + Sync + 'static) -> Self {
        self.busy = Arc::new(words);
        self
    }
}

impl Default for Words {
    fn default() -> Self {
        Words::new()
    }
}

impl fmt::Debug for Words {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Words")
            .field("help", &self.help)
            .field("again", &self.again)
            .field("yes", &self.yes)
            .field("no", &self.no)
            .field("choose", &self.choose)
            .field("leave", &self.leave)
            .field("too_small", &self.too_small)
            .field("waiting", &self.waiting)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fixed(words: &Words) -> [&str; 8] {
        [
            &words.help,
            &words.again,
            &words.yes,
            &words.no,
            &words.choose,
            &words.leave,
            &words.too_small,
            &words.waiting,
        ]
    }

    #[test]
    fn the_shell_has_no_words_of_its_own() {
        for words in [Words::new(), Words::default()] {
            assert_eq!(fixed(&words), [""; 8]);
            for busy in [0, 1, 7, usize::MAX] {
                assert_eq!((words.busy)(busy), "");
            }
        }
    }

    #[test]
    fn each_override_sets_its_own_word_alone() {
        let set = [
            Words::new().help("help"),
            Words::new().again("again"),
            Words::new().yes("yes"),
            Words::new().no("no"),
            Words::new().choose("choose"),
            Words::new().leave("leave"),
            Words::new().too_small("too small"),
            Words::new().waiting("waiting"),
        ];
        let names = [
            "help",
            "again",
            "yes",
            "no",
            "choose",
            "leave",
            "too small",
            "waiting",
        ];
        for (index, words) in set.iter().enumerate() {
            let shown = fixed(words);
            for (at, word) in shown.iter().enumerate() {
                let expected = if at == index { names[index] } else { "" };
                assert_eq!(*word, expected, "override {index}, word {at}");
            }
        }
    }

    #[test]
    fn words_take_borrowed_and_owned_text_and_the_last_override_wins() {
        let words = Words::new()
            .yes("Y")
            .yes(String::from("Sí"))
            .no(Cow::Borrowed("Nu"))
            .too_small(format!("{}×{}", 40, 12));
        assert_eq!(words.yes, "Sí");
        assert!(matches!(words.yes, Cow::Owned(_)));
        assert_eq!(words.no, "Nu");
        assert_eq!(words.too_small, "40×12");
    }

    #[test]
    fn busy_words_are_built_from_the_count_of_running_jobs() {
        let words = Words::new().busy(|jobs| match jobs {
            0 => String::new(),
            1 => "1 job".into(),
            many => format!("{many} jobs"),
        });
        assert_eq!((words.busy)(0), "");
        assert_eq!((words.busy)(1), "1 job");
        assert_eq!((words.busy)(12), "12 jobs");
        let copy = words.clone();
        assert_eq!((copy.busy)(3), "3 jobs");
        assert!(Arc::ptr_eq(&words.busy, &copy.busy));
    }

    #[test]
    fn debug_shows_the_words_and_hides_the_busy_closure() {
        let shown = format!("{:?}", Words::new().help("Help").waiting("Hold on"));
        assert!(shown.starts_with("Words {"), "{shown}");
        assert!(shown.contains("help: \"Help\""), "{shown}");
        assert!(shown.contains("waiting: \"Hold on\""), "{shown}");
        assert!(shown.contains("too_small: \"\""), "{shown}");
        assert!(!shown.contains("busy"), "{shown}");
        assert!(shown.ends_with(".. }"), "{shown}");
    }
}
