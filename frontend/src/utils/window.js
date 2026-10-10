// The loaded window's arithmetic.
//
// Kept out of the store and out of the component because this is the part that
// is wrong in a way nobody sees until they are three thousand segments into a
// book: which end needs more, how much may be dropped, and how far the scroller
// has to be moved to keep the text under the reader's eyes. All three are pure
// functions of numbers, so they are checked without a browser or a server.

// How many segments one request carries. Also the step every extension moves by.
export const PAGE = 60

// The cap on what is in the DOM at once.
//
// The bug this replaces had no cap at all, so a reader who scrolled a long book
// accumulated every segment they passed — `annya_vi_12` is 18,753 of them. 400
// is a little under seven requests: enough that a reader who scrolls steadily
// in one direction is never waiting on the network (they are always more than
// 300 segments from the end they are moving towards), small enough that the DOM
// stays a few thousand nodes. Trimming costs a request when the reader turns
// round and comes back, which is the price; what it buys is that the page does
// not degrade over a long read.
export const MAX_SEGMENTS = 400

// How close to the end of the loaded run the viewport has to come before more
// is fetched. 600px is about a screen and a half of text — the reader should
// never see the bottom of what is loaded.
export const EDGE_MARGIN = 600

// The context kept behind whichever end is being trimmed. Dropping the whole
// head when the reader is at the bottom would leave the top sentinel touching
// the viewport, which asks for the head back immediately — one request per
// frame, forever. Keeping a run behind means the sentinel ends up far above the
// viewport and stays quiet.
const KEEP_CONTEXT = 60

// Which ends need more, given where the two sentinels are on screen.
//
// Level-triggered, deliberately. The IntersectionObserver this replaces fired
// on a *change* of intersection, so a load that left the sentinel inside the
// margin — a short page, or a sentinel that was already inside when the
// observer was attached — was never followed by another one, and the page
// simply stopped growing. These are comparisons between two rectangles, so the
// same question can be asked again after every load, and the answer is the
// truth about the current layout rather than about the last transition.
//
// `col`, `top` and `bottom` are DOMRect-shaped: {top, bottom}. The top sentinel
// sits above the first loaded segment, the bottom one below the last, so the
// distance from the viewport to each is the amount of text still in hand on
// that side.
export function edgeLoads({ col, top, bottom, margin = EDGE_MARGIN } = {}) {
  if (!col || !top || !bottom) return { prev: false, next: false }
  const above = col.top - top.top
  const below = bottom.bottom - col.bottom
  return { prev: above < margin, next: below < margin }
}

// How many segments to drop from each end so the window is back under the cap.
//
// `prefer` is the end the reader is moving away from — the caller passes 'head'
// after a forward load and 'tail' after a backward one — so the run being
// dropped is the one they have already read past rather than the one they are
// about to read. The segment they are looking at is never dropped, and neither
// is KEEP_CONTEXT around it.
export function trimPlan(count, activeIndex, max = MAX_SEGMENTS, prefer = 'head') {
  const over = count - max
  if (over <= 0 || count <= 0) return { head: 0, tail: 0 }
  const i = activeIndex < 0 ? count - 1 : activeIndex
  const headRoom = Math.max(0, i - KEEP_CONTEXT)
  const tailRoom = Math.max(0, count - 1 - i - KEEP_CONTEXT)

  let left = over
  let head = 0
  let tail = 0
  const take = (room) => {
    const n = Math.min(room, left)
    left -= n
    return n
  }
  if (prefer === 'tail') {
    tail = take(tailRoom)
    head = take(headRoom)
  } else {
    head = take(headRoom)
    tail = take(tailRoom)
  }
  return { head, tail }
}

// How much to add to scrollTop so the same text stays under the reader.
//
// Only the head matters. Appending below the viewport does not move anything,
// but adding above it pushes every line down by exactly the height that was
// added, and removing from above pulls it up by the height that went — so in
// both cases the correction is the change in scrollHeight, and in both cases it
// is applied to scrollTop in the same direction. The browser clamps the result
// to the scrollable range, which is the right answer when the correction would
// take the reader above the start of the window.
export function anchorShift(beforeHeight, afterHeight) {
  const d = afterHeight - beforeHeight
  return Number.isFinite(d) ? d : 0
}

// Keep loading while an end of the window is still inside the margin.
//
// This is the whole of the fix for the stall: the question "does this end need
// more?" is asked again after every load, so a load that leaves the sentinel
// where it was is followed by another one instead of by silence. `need` is
// re-read each round rather than captured, because loading changes the answer;
// `loadPrev`/`loadMore` resolve true when they made progress, and a false — an
// end of the book, or a failed request — stops the burst rather than retrying
// into it. `settle` waits for the resulting paint, since the geometry the next
// round reads is the geometry after the DOM changed.
//
// The cap is a backstop, not the termination argument: each round either
// consumes a page or arrives at an end of the book, and both make `need` false.
export async function pumpEdges({ need, loadPrev, loadMore, settle, max = 12 }) {
  for (let i = 0; i < max; i++) {
    const n = need()
    if (!n.prev && !n.next) return i
    const ok = n.prev ? await loadPrev() : await loadMore()
    if (!ok) return i
    if (settle) await settle()
  }
  return max
}
