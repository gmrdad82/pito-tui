use std::{borrow::Cow, fmt, sync::Arc};

use crate::list::{Row, Source};

pub trait Build<T> {
    fn row(&self, index: usize, item: &T) -> Row;

    fn selectable(&self, index: usize, item: &T) -> bool {
        self.row(index, item).selectable()
    }
}

impl<T, F: Fn(usize, &T) -> Row> Build<T> for F {
    fn row(&self, index: usize, item: &T) -> Row {
        self(index, item)
    }
}

pub struct Shared<T, B = fn(usize, &T) -> Row> {
    items: Arc<Vec<T>>,
    builder: B,
    marks: u16,
}

impl<T, B> Shared<T, B> {
    pub fn new(items: Arc<Vec<T>>, builder: B) -> Self {
        Shared {
            items,
            builder,
            marks: 0,
        }
    }

    pub fn marks(mut self, width: u16) -> Self {
        self.marks = width;
        self
    }

    pub fn set_marks(&mut self, width: u16) {
        self.marks = width;
    }

    pub fn items(&self) -> &Arc<Vec<T>> {
        &self.items
    }

    pub fn set_items(&mut self, items: Arc<Vec<T>>) {
        self.items = items;
    }

    pub fn builder(&self) -> &B {
        &self.builder
    }

    pub fn builder_mut(&mut self) -> &mut B {
        &mut self.builder
    }
}

impl<T, B: Build<T>> Source for Shared<T, B> {
    fn len(&self) -> usize {
        self.items.len()
    }

    fn row(&self, index: usize) -> Cow<'_, Row> {
        Cow::Owned(self.builder.row(index, &self.items[index]))
    }

    fn selectable(&self, index: usize) -> bool {
        self.builder.selectable(index, &self.items[index])
    }

    fn mark_width(&self) -> u16 {
        self.marks
    }
}

impl<T, B: Clone> Clone for Shared<T, B> {
    fn clone(&self) -> Self {
        Shared {
            items: Arc::clone(&self.items),
            builder: self.builder.clone(),
            marks: self.marks,
        }
    }
}

impl<T, B: Default> Default for Shared<T, B> {
    fn default() -> Self {
        Shared::new(Arc::default(), B::default())
    }
}

impl<T, B> fmt::Debug for Shared<T, B> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Shared")
            .field("len", &self.items.len())
            .field("marks", &self.marks)
            .finish_non_exhaustive()
    }
}
