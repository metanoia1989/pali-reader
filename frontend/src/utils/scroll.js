// Keeping a list's active row in view.
//
// One function, because this has to happen in two lists that were written
// separately once already — the desktop contents rail and the phone's contents
// drawer — and only one of them did it. Anything with a list that follows the
// reader's position calls this, so "which of them scrolls" stops being a
// question.

// The scrollTop a list needs so `item` is comfortably visible, or null when it
// already is and the list should not move at all.
//
// The check is against the *visible box*, from bounding rectangles, not against
// `offsetTop`: offsetTop is measured from the nearest positioned ancestor,
// which is usually not the scrolling element, and comparing it with scrollTop
// mixes two coordinate systems — it looks right in a simple list and drifts in
// a padded one.
//
// An item that is off the top or the bottom lands a third of the way down
// rather than at the very edge: the point of following the reader is that they
// can see what comes next as well as where they are.
export function followScrollTop(item, box, current, { safeTop = 40, safeBottom = 60, lead = 1 / 3 } = {}) {
  if (!item || !box || !box.height) return null
  const top = item.top - box.top
  const bottom = item.bottom - box.top
  if (top >= safeTop && bottom <= box.height - safeBottom) return null
  const next = current + top - box.height * lead
  return next < 0 ? 0 : Math.round(next)
}
