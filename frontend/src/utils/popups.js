// Where a popup opened from inside the dictionary panel goes, and how dragging
// moves it.
//
// Kept out of the component so it can be checked without a browser: a popup
// placed off-screen is unreachable, and one placed over the text column covers
// the sentence the reader is following the thread from.

export const POPUP_W = 368
const MARGIN = 16
const STEP_Y = 26

// placePopup returns the top-left corner for a popup opened from `anchor`.
//
// Beside the thing that was clicked — to its right when there is room, to its
// left when there is not — and vertically lined up with it. Pinning them all to
// the top-right corner sent the eye to the wrong place: the reader clicked a
// word in the panel and the answer appeared at the far edge of the screen with
// nothing connecting the two.
//
// A popup opened from another popup steps down from its parent instead, so a
// chain reads as a chain rather than as a pile.
export function placePopup(anchor, viewport, depth = 0) {
  const { width: w, height: h } = viewport
  const a = anchor || { left: w - POPUP_W - MARGIN, right: w - MARGIN, top: 76 }

  // To the right of the click if it fits, otherwise to its left. Either way it
  // stays on screen.
  let x = a.right + 12
  if (x + POPUP_W > w - 8) x = a.left - POPUP_W - 12
  x = clamp(x, 8, Math.max(8, w - POPUP_W - 8))

  // Line up with the click, then step down for each popup opened from one.
  let y = (a.top ?? 76) + depth * STEP_Y
  // Keep it fully visible: slide up rather than letting it hang off the bottom.
  y = clamp(y, 8, Math.max(8, h - 320))
  return { x, y, w: POPUP_W }
}

// dragTo applies a pointer delta to a popup's origin, keeping at least a corner
// of it on screen so a popup can always be grabbed again.
export function dragTo(from, delta, viewport) {
  return {
    x: clamp(from.x + delta.dx, 4, viewport.width - 120),
    y: clamp(from.y + delta.dy, 4, viewport.height - 48),
  }
}

function clamp(v, lo, hi) {
  return Math.min(Math.max(v, lo), hi)
}
