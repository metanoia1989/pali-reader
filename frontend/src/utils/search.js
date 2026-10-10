// Searching, as data.
//
// The four modes, where each one looks, and the two pieces of arithmetic the
// search box needs — which chapter the reader is in, and what order to show
// hits in. Kept out of the components so they can be checked without a browser,
// and kept out of `ReaderSearch.vue` so the field, the mode switch and the
// result list stay three separate pieces: the interaction design coming in
// parallel will want to move them, and a welded block cannot be moved.

// The modes, in the order the switch offers them. Local first: a reader who
// opens search while reading almost always means "somewhere around here", and
// the local answer is the one that costs nothing.
//
// `hint` is what the switch says under each name — including the two things a
// reader would otherwise have to discover by being disappointed: 本章 is the
// chapter the contents has highlighted, and 译文 covers one book, not the canon.
//
// `short` is the same name cut to two characters, for the phone: at 390px the
// switch shares the bar with the field, the settings and the account, and
// 巴利全文 costs 25px that the field needs more.
export const SEARCH_MODES = [
  {
    id: 'chapter',
    label: '本章',
    short: '本章',
    hint: '目录里选中的这一章',
  },
  {
    id: 'title',
    label: '经文名',
    short: '经名',
    hint: '按经名、章名或卷名查找',
  },
  {
    id: 'pali',
    label: '巴利全文',
    short: '全文',
    hint: '整部三藏的巴利原文',
  },
  {
    id: 'refs',
    label: '译文',
    short: '译文',
    hint: '本书的参考译文，中英都查',
  },
]

export const DEFAULT_MODE = 'chapter'

// Two characters, the same rule the corpus search already had. One letter of
// Pāḷi matches most of the canon, and one Chinese character matches a whole
// book's worth of translation; the answer would be a list nobody can use.
export const MIN_QUERY = 2

export function modeById(id) {
  return SEARCH_MODES.find((m) => m.id === id) || SEARCH_MODES[0]
}

// The range of segment numbers the contents says the reader is in.
//
// The chapter is the last contents entry at or before where they are, and it
// ends where the next entry begins — which is why this reads the *entries*, not
// the loaded segments: a chapter of a 3,600-segment book is usually longer than
// the window in the DOM, and a search that only saw the window would answer
// "not found" to a passage two hundred segments above.
//
// `to: 0` means "no upper bound" — the last chapter runs to the end of the
// book, and the server reads 0 as no constraint.
export function chapterRange(entries, seq) {
  let i = -1
  for (let k = 0; k < entries.length; k++) {
    if (entries[k].seq <= seq) i = k
    else break
  }
  if (i < 0) return { from: 0, to: 0, name: '', index: -1 }
  const next = entries[i + 1]
  return {
    from: entries[i].seq,
    to: next ? Math.max(entries[i].seq, next.seq - 1) : 0,
    name: entries[i].name || '',
    index: i,
  }
}

// Hits from the book being read come first.
//
// The corpus-wide search is ordered by book and segment, which is the order the
// canon is bound in and not the order a reader wants: they are inside one book,
// and a match four lines above them is worth more than one in a volume they
// have never opened. The order within each group is left alone — it is reading
// order, which is predictable, and re-ranking by anything else would be a
// guess dressed up as relevance.
export function rankHits(hits, bookId) {
  const here = []
  const there = []
  for (const h of hits || []) (h.bookId === bookId ? here : there).push(h)
  return [...here, ...there]
}

// What the result row says about where a hit is.
export function hitWhere(hit) {
  if (hit.kind === 'heading' || hit.kind === 'book') return hit.bookName || ''
  return [hit.bookName, hit.para ? '§' + hit.para : ''].filter(Boolean).join(' · ')
}
