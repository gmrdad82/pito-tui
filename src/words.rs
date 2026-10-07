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
            .field("too_small", &self.too_small)
            .field("waiting", &self.waiting)
            .finish_non_exhaustive()
    }
}
