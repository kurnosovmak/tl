// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use crate::basictl::{Error, Result};
use std::{
    fmt,
    ops::{Deref, DerefMut},
    slice,
};

/// A sequence that keeps initialized slots for the next binary read.
/// Iteration, indexing and serialization see only the current message.
pub struct RetainedVec<T> {
    slots: Vec<T>,
    spare: Vec<T>,
    len: usize,
}

impl<T> Default for RetainedVec<T> {
    fn default() -> Self {
        Self {
            slots: Vec::new(),
            spare: Vec::new(),
            len: 0,
        }
    }
}

impl<T> RetainedVec<T> {
    pub fn clear(&mut self) {
        self.len = 0;
    }

    /// Drop the inactive elements and their nested allocations.
    pub fn discard_spare(&mut self) {
        self.slots.truncate(self.len);
        self.spare = Vec::new();
    }

    pub fn push(&mut self, item: T) {
        if let Some(slot) = self.slots.get_mut(self.len) {
            *slot = item;
        } else {
            self.slots.push(item);
        }
        self.len += 1;
    }

    // A transactional record borrows the inactive bank, then returns the old or
    // partially decoded bank. Its receiver always owns the live message.
    pub(crate) fn exchange_spare(&mut self, pending: &mut Self) {
        std::mem::swap(&mut self.spare, &mut pending.slots);
        pending.len = 0;
    }

    pub(crate) fn slots(&self) -> &[T] {
        &self.slots
    }

    #[inline]
    pub(crate) fn read_into(
        &mut self,
        count: usize,
        new: impl Fn() -> T,
        mut read: impl FnMut(&mut T) -> Result<()>,
    ) -> Result<()> {
        if let Some(slots) = self.slots.get_mut(..count) {
            for slot in slots {
                read(slot)?;
            }
        } else {
            for slot in &mut self.slots {
                read(slot)?;
            }
            // A TL1 count alone is insufficient evidence to allocate this many slots.
            for _ in self.slots.len()..count {
                let mut slot = new();
                read(&mut slot)?;
                self.slots
                    .try_reserve(1)
                    .map_err(|_| Error::InvalidLength)?;
                self.slots.push(slot);
            }
        }
        self.len = count;
        Ok(())
    }
}

impl<T> Deref for RetainedVec<T> {
    type Target = [T];
    fn deref(&self) -> &[T] {
        &self.slots[..self.len]
    }
}

impl<T> DerefMut for RetainedVec<T> {
    fn deref_mut(&mut self) -> &mut [T] {
        &mut self.slots[..self.len]
    }
}

impl<T: fmt::Debug> fmt::Debug for RetainedVec<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.deref().fmt(f)
    }
}

impl<T> From<Vec<T>> for RetainedVec<T> {
    fn from(slots: Vec<T>) -> Self {
        Self {
            spare: Vec::new(),
            len: slots.len(),
            slots,
        }
    }
}

impl<T> From<RetainedVec<T>> for Vec<T> {
    fn from(mut value: RetainedVec<T>) -> Self {
        value.discard_spare();
        value.slots
    }
}

impl<T> FromIterator<T> for RetainedVec<T> {
    fn from_iter<I: IntoIterator<Item = T>>(iter: I) -> Self {
        Vec::from_iter(iter).into()
    }
}

impl<'a, T> IntoIterator for &'a RetainedVec<T> {
    type Item = &'a T;
    type IntoIter = slice::Iter<'a, T>;
    fn into_iter(self) -> Self::IntoIter {
        self.iter()
    }
}

impl<'a, T> IntoIterator for &'a mut RetainedVec<T> {
    type Item = &'a mut T;
    type IntoIter = slice::IterMut<'a, T>;
    fn into_iter(self) -> Self::IntoIter {
        self.iter_mut()
    }
}

impl<T> IntoIterator for RetainedVec<T> {
    type Item = T;
    type IntoIter = std::vec::IntoIter<T>;
    fn into_iter(self) -> Self::IntoIter {
        Vec::from(self).into_iter()
    }
}
