// Where the English dictionary popup goes.
//
// Kept out of the component for the same reason the panel's popups are
// (utils/popups.js): a popup placed off-screen cannot be read or closed, and a
// popup placed over the word that was tapped hides the thing the reader was
// looking at. Both are cheap to get wrong by a few pixels and impossible to
// notice in a unit test written any other way.
//
// This popup is a glance — one word, one meaning, gone on the next click — so
// it does not stack, does not drag and does not have a chain of its own. It is
// anchored to the word and it stays small.

export const EN_POPUP_W = 336
// The height the placement works against: the popup's real height is its
// content's, up to this. Used only to decide whether there is room below.
export const EN_POPUP_MAX_H = 340
// Below this width the popup becomes a card at the bottom of the screen. The
// same threshold the reader uses to fold its left rail: a phone has no room to
// put a card beside the text, so it goes where the thumb already is.
export const EN_SHEET_BREAKPOINT = 760

export function isEnSheet(viewportWidth) {
  return viewportWidth < EN_SHEET_BREAKPOINT
}

// placeEnPopup returns where to put the popup for a word at `anchor`.
//
// It sits under the word, centred on it, with a caret pointing back up at it —
// the reader's eye should not have to travel, and the caret is what says which
// of the words on the line this definition belongs to. When there is not enough
// room below (and more above), it flips over the word instead: `y` is then the
// popup's bottom edge, because the card is shifted up by its own height.
export function placeEnPopup(anchor, viewport) {
  const { width: w, height: h } = viewport
  const a = anchor || { left: w / 2, right: w / 2, top: 80, bottom: 80 }

  const width = Math.min(EN_POPUP_W, Math.max(160, w - 16))
  const centre = (a.left + a.right) / 2
  const x = clamp(centre - width / 2, 8, Math.max(8, w - width - 8))
  // The caret follows the word, not the popup: at the screen edges the card
  // stops moving and the caret keeps pointing at the word.
  const caretX = clamp(centre - x, 18, width - 18)

  const below = h - a.bottom
  const above = a.top
  const placement = below < EN_POPUP_MAX_H && above > below ? 'above' : 'below'
  const y = placement === 'above' ? a.top - 10 : a.bottom + 10

  return { x, y, w: width, placement, caretX }
}

function clamp(v, lo, hi) {
  return Math.min(Math.max(v, lo), hi)
}
