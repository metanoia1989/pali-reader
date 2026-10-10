// The one search field the top bar has, whoever rendered it.
//
// The bar owns the shortcut that focuses the field, but on the reading page the
// field is supplied by the view (it is a search over the book that is open, not
// over the canon). Rather than make the bar reach into the DOM for it, the
// component that draws it registers how to focus it, and the bar calls that.
// One field exists at a time, so one slot is enough.
let focusFn = null

export function registerSearchField(fn) {
  focusFn = typeof fn === 'function' ? fn : null
}

export function focusSearchField() {
  if (!focusFn) return false
  focusFn()
  return true
}
