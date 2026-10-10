<script setup>
// The reading workspace.
//
// Three columns on a wide screen — catalogue, text, table of contents — plus the
// word panel as a fourth when a word is open. On a phone the same pieces become
// a drawer and a bottom sheet; the reading column itself never changes shape,
// because that is the one thing the reader is actually looking at.
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  BookA,
  BookMarked,
  ChevronLeft,
  ChevronRight,
  Library,
  ListTree,
  Loader2,
  PanelRightClose,
  PanelRightOpen,
  X,
} from 'lucide-vue-next'
import { api } from '../api'
import { useAuth } from '../store/auth'
import { useReader } from '../store/reader'
import { useSettings } from '../store/settings'
import { tocEntries } from '../utils/numbering'
import { isEnSheet, placeEnPopup } from '../utils/enpopup'
import { dragTo, placePopup } from '../utils/popups'
import { edgeLoads, pumpEdges } from '../utils/window'
import TopBar from '../components/TopBar.vue'
import CatalogTree from '../components/CatalogTree.vue'
import ReaderSearch from '../components/ReaderSearch.vue'
import SegmentCard from '../components/SegmentCard.vue'
import TocList from '../components/TocList.vue'
import WordLookup from '../components/WordLookup.vue'
import EnWordPopup from '../components/EnWordPopup.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuth()
const S = useSettings()
const R = useReader()

const catalog = ref([])
const railCollapsed = ref(false)
const tocCollapsed = ref(false)
// The contents always carries its translations. It is navigation, not the
// reading text: the 参考译文 setting governs what sits under a sentence the
// reader is reading, and a chapter name with no Chinese in the list is a name
// they cannot use. It is also the one place to learn what a Pāḷi heading means
// before travelling to it.
const showTocRefs = true

// Segments grouped by the paragraph they belong to, delimited by headings.
//
// The heading is part of the key, not decoration. Paragraph numbers RESTART at
// every sutta — `mula_ku_01` has a paragraph 11 in the Maṅgala Sutta and another
// in the Ratana Sutta — so keying on `para` alone merged paragraphs from
// different suttas into one group. Two things went wrong when it did: only the
// first segment of the merged group was treated as its head, so the later
// sutta's paragraphs drew no 中 / 英 button at all; and clicking one that did
// draw would have opened translations belonging to a different sutta.
//
// A heading starts a new section, so counting them as we walk the loaded window
// is enough; no extra field has to come from the API.
const paraGroups = computed(() => {
  const m = new Map()
  let section = 0
  for (const seg of R.segments) {
    if (seg.kind === 'heading') {
      section++
      m.set(`h${seg.seq}`, [seg.seq])
      continue
    }
    const key = `s${section}:${seg.para || 0}`
    const g = m.get(key)
    if (g) g.push(seg.seq)
    else m.set(key, [seg.seq])
  }
  return m
})

const paraOf = (seq) => {
  for (const seqs of paraGroups.value.values()) if (seqs.includes(seq)) return seqs
  return [seq]
}

// What the paragraph button should say. Only a language that some sentence of
// the paragraph actually has is offered, and it counts as on only when every
// sentence that has it is showing it — a half-open paragraph should read as
// closed, so one click opens the rest instead of shutting what is open.
function paraHas(seqs) {
  const has = {}
  for (const l of ['zh', 'en']) {
    has[l] = seqs.some((q) => !!R.refs[q]?.[l])
  }
  return has
}
function paraOn(seqs) {
  const on = {}
  for (const l of ['zh', 'en']) {
    const withLang = seqs.filter((q) => !!R.refs[q]?.[l])
    on[l] = withLang.length > 0 && withLang.every((q) => R.refShown(q, l))
  }
  return on
}
// Which of the two side panels the left column is showing. The dictionary sits
// on the left, as it does in tipitaka-pali-reader: that is the side the eye
// goes to for a tool, and it leaves the right for the contents.
const side = ref('catalog') // catalog | dict
const tocOpen = ref(false)
// One sentinel per end of the loaded window. They are siblings of the segment
// list, so what lies between them is exactly what is in the DOM.
const sentinelTop = ref(null)
const sentinelBottom = ref(null)
const readCol = ref(null)
const lookupError = ref('')

const bookId = computed(() => String(route.params.bookId || ''))

const isMobile = ref(false)
function measure() {
  isMobile.value = window.innerWidth < 1100
  if (window.innerWidth < 760) railCollapsed.value = true
  // A taller viewport can put the far sentinel inside the margin with no
  // scrolling at all, and no scroll event will ever say so.
  requestAnimationFrame(() => pump())
}
onMounted(() => {
  measure()
  window.addEventListener('resize', measure)
})

// The last seq written into the address bar, so a watcher and a deliberate jump
// cannot write it twice — and so the URL the reader arrived with is not
// immediately rewritten to something coarser before they have moved at all.
// Declared here rather than beside writeUrl() below because boot() runs during
// setup, and a `const` further down would still be in its dead zone.
let urlSeq = Number(route.query.seq || 0)

// --- open a book ---------------------------------------------------------
async function boot() {
  const seq = Number(route.query.seq || 0)
  urlSeq = seq
  await R.open(bookId.value, { seq: seq || 0 })
  if (!seq) restorePosition()
  await nextTick()
  pump()
}

// Put the reader back where they stopped. Stored per browser as well as on the
// account, so it works signed-out and instantly.
function restorePosition() {
  try {
    const v = Number(localStorage.getItem('pali.pos.' + bookId.value) || 0)
    // Against the book's length, not against what happens to be loaded: the
    // window is bounded now, so a position a third of the way in is regularly
    // outside the first window and used to be discarded as out of range.
    if (v > 1 && v <= R.total) {
      requestAnimationFrame(() => R.scrollToSeq(v))
    }
  } catch {
    /* nothing stored yet */
  }
}

watch(bookId, boot, { immediate: true })

onMounted(async () => {
  // The store anchors prepends to this element, so it has to know which one it
  // is before the first backward load.
  R.bindScroller(readCol.value)
  // The URL says where the reader is — which contents cell — so the browser
  // must not also try to restore a pixel offset of its own. It would be a
  // second, disagreeing answer to the same question, and it is the one that
  // wins: restoration happens after our scroll, so the reader would land
  // somewhere the address bar does not describe. Set here rather than in
  // index.html because it is the reading page's problem, and put back on the
  // way out so the other, ordinary pages keep the browser's behaviour.
  const restore = window.history?.scrollRestoration
  if (restore) window.history.scrollRestoration = 'manual'
  onBeforeUnmount(() => {
    if (restore) window.history.scrollRestoration = restore
  })
  try {
    const c = await api.catalog()
    catalog.value = c.baskets || []
  } catch {
    /* the rail is a convenience; the reader still works without it */
  }
  window.addEventListener('keydown', onKey)
  window.addEventListener('click', onDocClick)
  await nextTick()
  pump()
})
onBeforeUnmount(() => {
  R.bindScroller(null)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('click', onDocClick)
  window.removeEventListener('resize', measure)
})

function onKey(e) {
  if (e.key === 'Escape') {
    // Innermost first: the English glance is the smallest thing on the page,
    // so it is what Escape means while it is up.
    if (enPop.visible) closeEnPop()
    else if (R.panelOpen) R.closePanel()
    else if (tocOpen.value) tocOpen.value = false
  }
}

// --- the window's two edges -------------------------------------------------
//
// The reading column holds a contiguous run of segments and either end of it
// can be extended. What decides is where the two sentinels are on screen, asked
// afresh every time — see utils/window.js for why an IntersectionObserver was
// the wrong instrument: it reports a change of intersection, so a load that
// left the sentinel inside the margin was never followed by another one and the
// page simply stopped growing.
//
// It is asked again after every load, on every scroll (already one per frame),
// after a jump, and when the window is resized — a taller viewport moves the
// bottom sentinel into the margin without any scrolling at all.
let pumping = false

function edges() {
  const col = readCol.value
  const top = sentinelTop.value
  const bottom = sentinelBottom.value
  if (!col || !top || !bottom) return { prev: false, next: false }
  const need = edgeLoads({
    col: col.getBoundingClientRect(),
    top: top.getBoundingClientRect(),
    bottom: bottom.getBoundingClientRect(),
  })
  return { prev: need.prev && !R.doneStart, next: need.next && !R.doneEnd }
}

// One request per round, and the geometry decides whether there is another.
// This is what makes the deadlock impossible: a load that leaves the sentinel
// inside the margin is followed by the next round, which loads again, until
// either the margin is satisfied or the end of the book is reached. A failed
// request stops the burst outright rather than retrying into the same failure.
async function pump() {
  if (pumping) return
  pumping = true
  try {
    await pumpEdges({
      need: edges,
      loadPrev: () => R.loadPrev(),
      loadMore: () => R.loadMore(),
      settle: nextTick,
    })
  } finally {
    pumping = false
  }
}

// --- scrollspy -----------------------------------------------------------
let ticking = false
// The top bar slides away while the reader is going down the page and comes
// back the moment they go up. Three rules keep it from being annoying:
//
//   - at the top of the page it is always shown, so the way back to the
//     catalogue never has to be scrolled for;
//   - a reversal has to accumulate a real distance, not one pixel, or a
//     trackpad that jitters holds the bar in a permanent flicker;
//   - it comes back on focus, so a keyboard user is never typing into a control
//     that has slid off the screen.
//
// The two side rails deliberately do NOT do this. They are columns: hiding one
// changes the width of the text, which reflows every line and moves the line the
// reader is on — the opposite of what this setting is for. The top bar is an
// overlay; hiding it costs the text nothing. The rails have their own collapse
// buttons for a reader who wants the room.
const BAR_REVEAL_AT = 40
const BAR_DIRECTION_SLACK = 24
let lastY = 0
let goingUp = 0

function trackBar(col) {
  const y = col.scrollTop
  const dy = y - lastY
  lastY = y
  if (!S.autoHideBar) {
    R.barHidden = false
    return
  }
  if (y <= BAR_REVEAL_AT) {
    goingUp = 0
    R.barHidden = false
    return
  }
  if (dy > 0) goingUp = 0
  else if (dy < 0) goingUp += -dy
  if (goingUp >= BAR_DIRECTION_SLACK) R.barHidden = false
  else if (dy > BAR_DIRECTION_SLACK) R.barHidden = true
}

// The bar's height is one variable the whole shell reads, so putting it on the
// document element is what makes the reading column grow into the space as the
// bar leaves — the slide and the reclaim are one movement instead of two.
//
// It is set here rather than derived in CSS because the value has to be
// animatable: --topbar-h is registered with @property so it can be transitioned,
// and a rule that merely toggled it between two literals would snap.
// Removed on the way out so the next view does not inherit a collapsed bar.
watch(
  () => R.barHidden,
  (hidden) => {
    document.documentElement.style.setProperty('--topbar-h', hidden ? '0px' : '56px')
  },
)
onBeforeUnmount(() => {
  document.documentElement.style.removeProperty('--topbar-h')
})

function onScroll() {
  if (ticking) return
  ticking = true
  requestAnimationFrame(() => {
    ticking = false
    const col = readCol.value
    if (!col) return
    trackBar(col)
    const cards = col.querySelectorAll('[data-seq]')
    const top = 90
    let current = 0
    for (const c of cards) {
      if (c.getBoundingClientRect().top <= top) current = Number(c.dataset.seq)
      else break
    }
    if (current && current !== R.activeSegment) {
      R.activeSegment = current
      try {
        localStorage.setItem('pali.pos.' + bookId.value, String(current))
      } catch {
        /* private mode */
      }
    }
    // Last, so the trim that follows a load is planned against the segment the
    // reader has just been found to be on rather than the previous one.
    pump()
  })
}

// --- toc -----------------------------------------------------------------
// Levels 1 and 2 are the piṭaka and the book; the reader is already inside
// them, so a list of the book's contents starts at chapter level.
const toc = computed(() => tocEntries(R.book ? R.toc : []))
const activeTocIndex = computed(() => {
  const t = toc.value
  let idx = -1
  for (let i = 0; i < t.length; i++) {
    if (t[i].seq <= R.activeSegment) idx = i
    else break
  }
  return idx
})
// The heading the reader is in — the unit the address bar follows and the unit
// the contents highlights. One thing, computed once.
const activeHeading = computed(() => toc.value[activeTocIndex.value] || null)

// Everything the reader asks to be taken to goes through here: a contents
// click, a search hit inside this book. One place, so "a deliberate jump leaves
// a history entry and a scroll does not" is a property of the app rather than
// of whichever control happened to be written last.
async function jumpTo(seq) {
  if (!seq) return
  writeUrl(seq, { push: true })
  await R.scrollToSeq(seq)
  // A jump puts the reader wherever the target landed — often near one end of
  // the new window. Ask both ends whether they need more before handing back,
  // so the reader's first scroll in either direction has something in hand.
  await nextTick()
  pump()
}

function goto(seq) {
  tocOpen.value = false
  R.railDrawer = false
  jumpTo(seq)
}

// --- the address bar -------------------------------------------------------
//
// Where the reader is, in the URL, at the granularity of one cell of the
// contents rail: `/read/tika_an_04?seq=1234`. Not the paragraph — a reader does
// not want four hundred history entries for one sutta — and not the pixel,
// which is not a place in a book.
//
// A query parameter rather than a hash, because `?seq=` is already how this app
// says "open here": the search results link to `/read/<book>?seq=<n>` and
// `open()` reads it. A hash would be a second way of saying the same thing, and
// the two would drift — the search page linking with one, the address bar
// showing the other.
//
// Replace when the position changed because the reader scrolled, push when they
// deliberately jumped (a contents click, or a search hit). So Back leaves the
// book, or returns to the section they jumped from, instead of unwinding a
// hundred scroll positions one press at a time.
// Replace when the position changed because the reader scrolled, push when they
// deliberately jumped (a contents click, or a search hit). So Back leaves the
// book, or returns to the section they jumped from, instead of unwinding a
// hundred scroll positions one press at a time.
function writeUrl(seq, { push = false } = {}) {
  if (!seq || seq === urlSeq) return
  urlSeq = seq
  const query = { ...route.query, seq: String(seq) }
  if (push) router.push({ query })
  else router.replace({ query })
}

// The URL follows the paragraph the reader has moved into, one contents cell at
// a time.
watch(activeHeading, (h) => {
  if (h) writeUrl(h.seq)
})

// The other direction: Back and Forward, or a link into the same book. Without
// this the address bar would change under a Back press and the text would not
// move, which is worse than not touching the URL at all.
watch(
  () => route.query.seq,
  (v) => {
    const seq = Number(v || 0)
    if (!seq || seq === urlSeq) return
    urlSeq = seq
    R.scrollToSeq(seq)
  },
)

const crumbs = computed(() => {
  const b = R.book
  if (!b) return []
  const basket = catalog.value.find((x) => x.code === b.basket)
  const out = []
  if (basket) out.push({ label: basket.name, to: '/catalog' })
  out.push({ label: S.primaryName(b.nameZh, b.name) })
  return out
})

// --- words ---------------------------------------------------------------
const openKey = computed(() => (R.look ? `${R.look.segment}:${R.look.wordIndex}` : ''))

async function pick(p) {
  lookupError.value = ''
  side.value = 'dict'
  // A new word in the text is a new question. The popups were answers to the
  // old one, so they go — that is the gesture the reader already makes to move
  // on, and it saves closing them one at a time.
  popups.value = []
  // The same goes for the English glance: the reader has moved to a Pāḷi word.
  closeEnPop()
  await R.showWord(p.word, p.segment, p.wordIndex)
}

// --- the English dictionary popup ----------------------------------------
// One word of the English 参考译文 and what it means. It is not the panel:
// nothing is recorded, nothing is picked, and there is only ever one of it —
// tapping the next word replaces its contents instead of opening a second card.
// That is what makes it safe to leave un-draggable: anything the reader does
// next (tap another word, tap anywhere else, press Esc, look up a Pāḷi word)
// puts it away, so it cannot end up parked over the sentence being read.
const enPop = reactive({
  visible: false,
  word: '',
  head: '',
  phonetic: '',
  via: '',
  senses: [],
  available: true,
  notFound: false,
  loading: false,
  error: '',
  sheet: false,
  placement: 'below',
  x: 0,
  y: 0,
  w: 0,
  caretX: 0,
})

// Every lookup carries the number it was started with; a late answer for a word
// the reader has already moved on from is dropped rather than drawn.
let enSeq = 0

function closeEnPop() {
  enPop.visible = false
  enSeq++
}

async function onEnWord({ word, el }) {
  if (!word) return
  const seq = ++enSeq

  const rect = el?.getBoundingClientRect?.()
  const vp = { width: window.innerWidth, height: window.innerHeight }
  enPop.sheet = isEnSheet(vp.width)
  const at = placeEnPopup(
    rect
      ? { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom }
      : { left: vp.width / 2, right: vp.width / 2, top: 80, bottom: 80 },
    vp,
  )
  Object.assign(enPop, {
    word,
    head: '',
    phonetic: '',
    via: '',
    senses: [],
    available: true,
    notFound: false,
    error: '',
    loading: true,
    visible: true,
    ...at,
  })

  try {
    const data = await api.enLookup(word)
    if (seq !== enSeq) return
    enPop.head = data.word || ''
    enPop.phonetic = data.phonetic || ''
    enPop.via = data.via || ''
    enPop.senses = data.senses || []
    enPop.available = data.available !== false
    enPop.notFound = !data.found
  } catch (e) {
    if (seq !== enSeq) return
    enPop.error = e.message
  } finally {
    if (seq === enSeq) enPop.loading = false
  }
}

// A tap anywhere that is not the popup and not another word of the English row
// puts it away. The word itself is exempt because tapping one word after
// another is the reader working down a sentence, and closing in between would
// flash the card off and on.
function onDocClick(e) {
  if (!enPop.visible) return
  if (e.target?.closest?.('.en-pop') || e.target?.closest?.('.enw')) return
  closeEnPop()
}

function closePanel() {
  R.closePanel()
  side.value = 'catalog'
}

// --- the words visited in the panel, so ‹ › can walk back -----------------
const trail = ref([])
const at = ref(-1)
let stepping = false

watch(
  () => R.look && `${R.look.segment}:${R.look.wordIndex}:${R.look.word}`,
  () => {
    if (!R.look) return
    if (stepping) {
      stepping = false
      return
    }
    // The WORD identifies a step, not the anchor. Two different words looked up
    // from the same place — which is exactly what a search from the panel does —
    // share an anchor, so keying on it alone made the second one overwrite the
    // first and ‹ had nowhere to go back to.
    const key = `${R.look.segment}:${R.look.wordIndex}:${R.look.word}`
    if (trail.value[at.value]?.key === key) return
    trail.value = [...trail.value.slice(0, at.value + 1), { key, ...R.look }].slice(-50)
    at.value = trail.value.length - 1
  },
)

function step(delta) {
  const next = at.value + delta
  if (next < 0 || next >= trail.value.length) return
  at.value = next
  stepping = true
  const t = trail.value[next]
  R.showWord(t.word, t.segment, t.wordIndex)
}

// --- popups ---------------------------------------------------------------
// A click inside the panel opens beside it rather than replacing it. On a wide
// screen there is room for both, and losing the word you were reading in order
// to look up a word inside its own entry is the wrong trade. Popups stack and
// none of them closes on its own — the reader closes what they are done with.
const popups = ref([])
let popupSeq = 0

// Placement is computed by utils/popups.js so it can be checked without a
// browser. A popup opens beside the thing that was clicked, so the eye does not
// have to travel; opening from another popup steps down from it.
function openPopup(word, el) {
  if (!word) return
  const r = el?.getBoundingClientRect?.()
  const anchor = r
    ? { left: r.left, right: r.right, top: r.top }
    : { left: window.innerWidth - 400, right: window.innerWidth - 20, top: 76 }
  const depth = el?.closest?.('.popup') ? 1 : 0
  const at = placePopup(anchor, { width: window.innerWidth, height: window.innerHeight }, depth)
  popups.value = [...popups.value, { id: ++popupSeq, word, ...at }]
}

function closePopup(id) {
  popups.value = popups.value.filter((p) => p.id !== id)
}

// Popups are dragged by their body: a chain of look-ups is something the reader
// arranges, and a panel that always covers the same part of the screen is a
// panel they end up closing to see past.
let drag = null

function bringToFront(id) {
  const p = popups.value.find((q) => q.id === id)
  if (!p || popups.value[popups.value.length - 1]?.id === id) return
  popups.value = [...popups.value.filter((q) => q.id !== id), p]
}

function startDrag(e, p) {
  // Anything interactive keeps its own behaviour; only bare surface drags.
  if (e.target.closest('button, a, input, textarea, select, label')) return
  bringToFront(p.id)
  const from = { x: e.clientX, y: e.clientY, ox: p.x, oy: p.y }
  drag = p.id
  const move = (ev) => {
    const at = dragTo(
      { x: from.ox, y: from.oy },
      { dx: ev.clientX - from.x, dy: ev.clientY - from.y },
      { width: window.innerWidth, height: window.innerHeight },
    )
    popups.value = popups.value.map((q) => (q.id === p.id ? { ...q, ...at } : q))
  }
  const up = () => {
    drag = null
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

// A link inside the panel. A wide screen has room for the entry and what the
// entry refers to at the same time, so it opens beside; a phone does not, so
// there the panel follows the reader and the trail brings them back.
// A word typed into the panel's search. It goes through showWord, so the trail
// records it exactly as if it had been tapped in the text, and ‹ › walks it.
// Choosing a panel opens the rail if it is folded.
//
// The rail collapses to 52px, which is wide enough for the two icons and
// nothing else. Picking 词典 while folded used to render the whole dictionary
// into that strip — a panel asking for the space, in a column that does not
// have it. Asking for a panel is asking for the space it needs, so the rail
// comes back.
function showPanel(which) {
  side.value = which
  if (railCollapsed.value) railCollapsed.value = false
}

function onSearch(word) {
  R.showWord(word, R.look?.segment || 0, R.look?.wordIndex ?? -1)
}

function onLink({ word, el }) {
  if (!word) return
  if (isMobile.value) {
    R.showWord(word, R.look?.segment || 0, R.look?.wordIndex ?? -1)
    return
  }
  openPopup(word, el)
}

// The picks already recorded for the word the panel is showing, so the panel can
// mark the rows the reader has taken and offer to take them back.
const existingPicks = computed(() => {
  if (!R.look) return []
  return R.picksFor(R.look.segment, R.look.wordIndex)
})

async function addPick(payload) {
  try {
    await R.addPick(payload)
  } catch (e) {
    lookupError.value = e.message
  }
}

async function removePick(payload) {
  try {
    await R.removePick(payload.segment, payload.wordIndex, payload.key)
  } catch (e) {
    lookupError.value = e.message
  }
}


function needAuth() {
  router.push({ name: 'login', query: { next: route.fullPath } })
}

async function reopen(anchor) {
  const seg = R.segmentBySeq(anchor.segment)
  const tok = seg?.tokens?.[anchor.wordIndex]
  const surface = tok && seg ? seg.text.slice(tok[0], tok[0] + tok[1]) : ''
  await R.showWord(surface, anchor.segment, anchor.wordIndex)
}

async function saveTranslation(payload) {
  try {
    await R.saveTranslation(payload.segment, payload.text)
  } catch (e) {
    lookupError.value = e.message
  }
}

async function addNote(payload) {
  try {
    await R.saveNote(payload)
  } catch (e) {
    lookupError.value = e.message
  }
}

async function deleteNote(n) {
  try {
    await R.removeNote(n)
  } catch (e) {
    lookupError.value = e.message
  }
}

const progress = computed(() => {
  if (!R.total) return 0
  return Math.min(100, Math.round((R.activeSegment / R.total) * 100))
})
</script>

<template>
  <div class="shell">
    <TopBar :crumbs="crumbs" :show-nav="false">
      <!-- The bar's own field leads to the search page; here it is replaced by
           a search over the book that is open, with the results under the
           field. It is a tool inside the reader, so it is bounded by the width
           of the field: it never takes over the bar or the reading column. -->
      <template #search>
        <ReaderSearch @goto="jumpTo" />
      </template>
      <template #actions>
        <!-- On a phone the two directories live here, where a reader looks for
             them, rather than in a bar at the foot of the screen next to the
             progress readout. On a desktop both are already columns. -->
        <template v-if="isMobile">
          <!-- Only the title contents here. The catalogue drawer has its own
               button in the top bar's left corner, and two controls for one
               panel — one on each side of the bar — is one too many. -->
          <button
            class="iconbtn"
            title="标题目录"
            aria-label="标题目录"
            :aria-pressed="tocOpen"
            @click="tocOpen = !tocOpen"
          >
            <ListTree :size="18" />
          </button>
        </template>
      </template>
    </TopBar>

    <div
      class="workspace"
      :class="{
        'rail-collapsed': railCollapsed && !isMobile,
        'toc-collapsed': tocCollapsed && !isMobile,
      }"
    >
      <!-- left: catalogue or the dictionary -->
      <aside
        v-if="!isMobile"
        class="col col-rail"
        :class="{ 'is-dict': side === 'dict', 'is-collapsed': railCollapsed }"
      >
        <div class="side-tabs">
          <button :aria-pressed="side === 'catalog'" title="典籍目录" @click="showPanel('catalog')">
            <Library :size="14" :stroke-width="1.8" aria-hidden="true" /><span class="tab-label"
              >目录</span
            >
          </button>
          <!-- Always available: the panel has its own search field, so a reader
               who knows the word they want should not have to find it in the
               text and tap it first. -->
          <button :aria-pressed="side === 'dict'" title="词典" @click="showPanel('dict')">
            <BookA :size="14" :stroke-width="1.8" aria-hidden="true" /><span class="tab-label"
              >词典</span
            >
          </button>
        </div>

        <CatalogTree
          v-if="side === 'catalog'"
          :catalog="catalog"
          :current="bookId"
          :toc="R.toc"
          :toc-ref-for="R.tocRefFor"
          :active-seq="R.activeSegment"
          :collapsed="railCollapsed"
          @collapse="railCollapsed = !railCollapsed"
          @goto="goto"
        />

        <div v-else class="dict-wrap">
          <WordLookup
            :key="openKey"
            :word="R.look?.word || ''"
            :segment="R.look?.segment || 0"
            :word-index="R.look?.wordIndex ?? -1"
            :book-id="bookId"
            :picks="existingPicks"
            :signed-in="auth.signedIn"
            @add="addPick"
            @remove="removePick"
            @link="onLink"
            @search="onSearch"
            @step="step"
            @close="closePanel"
            @need-auth="needAuth"
          />
        </div>
      </aside>

      <div v-if="isMobile && R.railDrawer" class="scrim" @click="R.railDrawer = false" />
      <aside v-if="isMobile && R.railDrawer" class="drawer">
        <CatalogTree
          :catalog="catalog"
          :current="bookId"
          :toc="R.toc"
          :toc-ref-for="R.tocRefFor"
          :active-seq="R.activeSegment"
          @collapse="R.railDrawer = false"
          @goto="(seq) => { R.railDrawer = false; goto(seq) }"
        />
      </aside>

      <!-- centre: the text -->
      <main ref="readCol" class="col col-read" @scroll.passive="onScroll">
        <div class="measure">
          <div v-if="R.loading" style="display: grid; place-items: center; padding: 80px">
            <Loader2 :size="22" class="spin" style="color: var(--meta)" />
          </div>

          <div v-else-if="R.error" class="card" style="padding: 20px">
            <p style="color: var(--danger)">{{ R.error }}</p>
            <button class="btn btn-secondary" style="margin-top: 12px" @click="boot">重试</button>
          </div>

          <template v-else>
            <div class="bookhead">
              <p class="eyebrow">{{ R.book?.name }}</p>
              <h1 class="booktitle">{{ R.book?.nameZh || R.book?.name }}</h1>
              <div class="bookmeta num">
                <span>{{ R.total }} 段</span>
                <span>·</span>
                <span>已读 {{ progress }}%</span>
              </div>
            </div>

            <!-- The head of the window. A fixed 1px box: whatever it draws is
                 absolutely positioned inside it, so a load in progress cannot
                 change the height of the content and put the anchoring
                 arithmetic out by the height of a pill. -->
            <div ref="sentinelTop" class="edge-sentinel">
              <span v-if="R.loadingPrev" class="edge-note">
                <Loader2 :size="12" class="spin" />正在载入前面的内容…
              </span>
            </div>

            <SegmentCard
              v-for="seg in R.segments"
              :key="seg.seq"
              :seg="seg"
              :picks-for="R.picksFor"
              :split-for="R.splitFor"
              :notes-for="R.notesFor"
              :translation="R.translationFor(seg.seq)"
              :references="R.refs[seg.seq] || {}"
              :refs-here="R.refsOpenFor(seg.seq)"
              :is-para-head="paraOf(seg.seq)[0] === seg.seq"
              :para-seqs="paraOf(seg.seq)"
              :para-has="paraHas(paraOf(seg.seq))"
              :para-on="paraOn(paraOf(seg.seq))"
              :open-key="openKey"
              :signed-in="auth.signedIn"
              :active="false"
              @pick="pick"
              @unpick="removePick"
              @en-word="onEnWord"
              @toggle-ref="R.toggleRefOpen($event.segment, $event.lang)"
              @toggle-para-ref="R.setRefShown($event.seqs, $event.lang, $event.on)"
              @save-translation="saveTranslation"
              @add-note="addNote"
              @delete-note="deleteNote"
              @need-auth="needAuth"
            />

            <div ref="sentinelBottom" class="edge-sentinel">
              <span v-if="R.loadingMore && !R.loadingPrev" class="edge-note is-below">
                <Loader2 :size="12" class="spin" />正在载入后面的内容…
              </span>
            </div>
            <div v-if="R.done && R.segments.length" class="endmark">
              <span class="eyebrow">— 此卷终 —</span>
              <div style="display: flex; gap: 8px; margin-top: 14px; justify-content: center">
                <router-link v-if="R.book?.prev" class="btn btn-secondary" :to="`/read/${R.book.prev.id}`">
                  <ChevronLeft :size="15" />{{ R.book.prev.nameZh || R.book.prev.name }}
                </router-link>
                <router-link v-if="R.book?.next" class="btn btn-secondary" :to="`/read/${R.book.next.id}`">
                  {{ R.book.next.nameZh || R.book.next.name }}<ChevronRight :size="15" />
                </router-link>
              </div>
            </div>
          </template>
        </div>
      </main>

      <!-- right: contents. Collapsible like the rail on the left — a reader
           who is following one passage does not need three hundred headings
           taking a fifth of the screen. -->
      <aside v-if="!isMobile" class="col col-toc">
        <button
          v-if="tocCollapsed"
          class="iconbtn"
          style="width: 28px; height: 28px; margin: 6px auto 0"
          title="展开标题目录"
          aria-label="展开标题目录"
          @click="tocCollapsed = false"
        >
          <PanelRightOpen :size="15" />
        </button>
        <template v-else>
          <div class="sechead" style="margin-bottom: 10px">
            <span class="eyebrow">标题目录</span>
            <span style="display: flex; align-items: center; gap: 6px">
              <span class="secmeta num">{{ toc.length }}</span>
              <button
                class="iconbtn"
                style="width: 26px; height: 26px"
                title="收起标题目录"
                aria-label="收起标题目录"
                @click="tocCollapsed = true"
              >
                <PanelRightClose :size="15" />
              </button>
            </span>
          </div>
          <TocList
            :entries="toc"
            :active-index="activeTocIndex"
            :ref-for="showTocRefs ? R.tocRefFor : null"
            @goto="goto"
          />
        </template>
      </aside>

    </div>

    <!-- Popups opened from inside the panel. They stack, none of them closes on
         its own, and they are placed clear of the text column so the sentence
         being read is never covered. -->
    <div
      v-for="p in popups"
      :key="p.id"
      class="popup"
      :class="{ 'is-dragging': drag === p.id }"
      :style="{ left: p.x + 'px', top: p.y + 'px', width: p.w + 'px' }"
      @pointerdown="startDrag($event, p)"
    >
      <WordLookup
        :key="p.id + ':' + p.word"
        :word="p.word"
        :segment="R.look?.segment || 0"
        :word-index="R.look?.wordIndex ?? -1"
        :book-id="bookId"
        :picks="[]"
        :signed-in="auth.signedIn"
        compact
        @link="({ word, el }) => openPopup(word, el)"
        @search="openPopup"
        @close="closePopup(p.id)"
        @need-auth="needAuth"
      />
    </div>

    <!-- The English dictionary popup: one word of the 参考译文, its meanings,
         and nothing else. It is not part of the stack above — there is only one
         of it, replacing its contents is what tapping the next word does, and
         any click elsewhere, Escape or a Pāḷi lookup puts it away. A glance
         does not need a window manager. -->
    <EnWordPopup :popup="enPop" @close="closeEnPop" />

    <!-- mobile: contents drawer -->
    <template v-if="isMobile">
      <div v-if="tocOpen" class="scrim" @click="tocOpen = false" />
      <aside v-if="tocOpen" class="drawer right">
        <div class="sechead" style="padding: 14px 14px 10px; margin: 0">
          <span class="eyebrow">标题目录</span>
          <button class="iconbtn" style="width: 28px; height: 28px" @click="tocOpen = false">
            <X :size="16" />
          </button>
        </div>
        <!-- Same list as the desktop rail, so it opens already scrolled to the
             chapter being read. It did not before: the drawer was written as a
             second copy of the markup, and the copy that follows the reader was
             the other one. -->
        <TocList
          style="padding: 0 10px 30px"
          :entries="toc"
          :active-index="activeTocIndex"
          :ref-for="R.tocRefFor"
          @goto="goto"
        />
      </aside>

      <!-- mobile: the word panel as a bottom sheet -->
      <template v-if="R.panelOpen">
        <div class="scrim" @click="R.closePanel()" />
        <div class="sheet">
          <div class="sheet-grip" @click="closePanel"><i /></div>
          <div class="sheet-body">
            <WordLookup
              :key="openKey"
              :word="R.look?.word || ''"
              :segment="R.look?.segment || 0"
              :word-index="R.look?.wordIndex ?? -1"
              :book-id="bookId"
              :picks="existingPicks"
              :signed-in="auth.signedIn"
              @add="addPick"
              @remove="removePick"
              @link="onLink"
              @search="onSearch"
              @close="closePanel"
              @need-auth="needAuth"
            />
          </div>
        </div>
      </template>

      <!-- A readout, not a control strip: the two directory buttons are in the
           top bar where a reader looks for them. -->
      <div class="mobilebar num" aria-hidden="true">
        <span class="mb-pos">{{ R.activeSegment }} / {{ R.total }}</span>
        <span class="mb-track"><i :style="{ width: progress + '%' }" /></span>
        <span class="mb-pct">{{ progress }}%</span>
      </div>
    </template>

    <div v-if="lookupError" class="toast" @click="lookupError = ''">{{ lookupError }}</div>
  </div>
</template>

<style scoped>
/* 目录 / 词典 — two panels share one column, so which one is showing has to be
   visible and switchable without hunting. */
/* Collapsed to 52px there is no room for the words, and squeezing them in made
   the strip look broken. The icons alone still say which panel is which, and
   the title attribute carries the name for anyone who needs it. */
.col-rail.is-collapsed .side-tabs {
  flex-direction: column;
  /* No inset at all. The expanded bar's two tabs fill their halves and meet with
     a hairline between them; the collapsed bar is the same control turned on its
     side, so its two tabs fill their bands the same way. Padding on the
     container left a strip of bare rail above the first button. */
  padding: 0;
}
.col-rail.is-collapsed .tab-label {
  display: none;
}
/* Tall enough that the tint reads as a band of the rail rather than as a
   highlight behind an icon. The two together fill the strip with only the
   hairline between them. */
.col-rail.is-collapsed .side-tabs button {
  padding: 14px 0;
}
/* No side padding: the pressed tab's tint runs to the edge of its half, which
   reads as two halves of one control rather than two buttons in a box. */
.side-tabs {
  flex: none;
  display: flex;
  gap: 1px;
  padding: 0;
  border-bottom: 1px solid var(--border);
}
.side-tabs button {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  /* Square, and a little taller than a chip. The tint now runs to the edges of
     its half — two halves of one control — so rounding it would round a shape
     whose whole job is to look like half of a bar. */
  padding: 9px 0;
  border-radius: 0;
  font-size: 12.5px;
  color: var(--muted);
  transition: background var(--fast), color var(--fast);
}
.side-tabs button:hover {
  background: var(--surface-warm);
  color: var(--fg);
}
.side-tabs button[aria-pressed='true'] {
  background: var(--accent);
  color: var(--surface);
}
/* A heading and its translation, stacked: at this column width a running line
   of Pāḷi followed by Chinese wraps in the middle of a name.
   No indent of its own — the row already carries the level's indentation, and a
   second one pushed the Chinese a whole level to the right of the Pāḷi it
   translates, which read as it being centred under a longer name. */
.toc-item .tref {
  display: block;
  margin-top: 1px;
  font-size: 12px;
  font-weight: 400;
  line-height: 1.4;
  color: var(--meta);
}
.toc-item.is-active .tref {
  color: var(--accent);
  opacity: 0.85;
}
.col-rail.is-dict {
  padding: 0;
}
.dict-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
}
.dict-wrap .panel {
  border: 0;
  border-radius: 0;
  box-shadow: none;
  flex: 1;
  min-height: 0;
}
/* A popup opened from inside the dictionary panel. Fixed, so it never disturbs
   the column, and stacked by z-index in the order it was opened. */
.popup {
  position: fixed;
  z-index: 60;
  max-height: min(70vh, 560px);
  display: flex;
  cursor: grab;
  touch-action: none;
}
.popup.is-dragging {
  cursor: grabbing;
}
.popup.is-dragging :deep(.panel) {
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.14);
}
.popup :deep(.panel-body),
.popup :deep(textarea) {
  cursor: auto;
}
.popup :deep(.panel) {
  flex: 1;
  min-height: 0;
  /* Not `height: 100%`: the popup's height is auto up to a max, so a percentage
     resolves against nothing and the panel grows past the cap. Bounding it by
     the container instead lets .panel-body scroll. */
  height: auto;
  max-height: 100%;
}
.popup :deep(.panel-body) {
  overflow-y: auto;
  overscroll-behavior: contain;
}
.jumpnote {
  position: sticky;
  top: 10px;
  z-index: 12;
  display: flex;
  align-items: center;
  gap: 7px;
  width: fit-content;
  margin: 0 auto -34px;
  padding: 6px 13px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface);
  box-shadow: var(--whisper);
  font-size: 12px;
  color: var(--muted);
}
/* The two ends of the loaded window.
   1px tall whatever is happening, and the note that appears while a load is in
   flight is taken out of the flow and hung outside it. This is not decoration:
   a prepend (and a trim from the head) is anchored by measuring the scroller
   before and after, so an indicator that changed the height of the content
   would be subtracted from the reader's place in the text and then given back a
   moment later — the text would jump by the height of a pill, twice. The old
   bottom spinner was 78px of ordinary flow, which is exactly that bug. */
.edge-sentinel {
  position: relative;
  height: 1px;
}
.edge-note {
  position: absolute;
  bottom: 5px;
  left: 50%;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  transform: translateX(-50%);
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface);
  box-shadow: var(--whisper);
  font-size: 12px;
  color: var(--muted);
  white-space: nowrap;
}
.edge-note.is-below {
  top: 5px;
  bottom: auto;
}
.bookhead {
  padding: 8px 4px 22px;
}
.booktitle {
  margin-top: 6px;
  font-size: 28px;
  font-weight: 500;
  line-height: 1.25;
  letter-spacing: 0.2px;
}
.bookmeta {
  display: flex;
  gap: 6px;
  margin-top: 8px;
  font-size: 12px;
  color: var(--meta);
}
.endmark {
  padding: 60px 0 30px;
  text-align: center;
}
.drawer {
  position: fixed;
  top: var(--topbar-h);
  bottom: 0;
  left: 0;
  z-index: 41;
  width: min(84vw, 320px);
  display: flex;
  flex-direction: column;
  background: var(--surface);
  border-right: 1px solid var(--border);
  animation: slide var(--base);
}
.drawer.right {
  left: auto;
  right: 0;
  border-right: 0;
  border-left: 1px solid var(--border);
  animation: slideR var(--base);
}
@keyframes slide {
  from {
    transform: translateX(-12px);
    opacity: 0.6;
  }
}
@keyframes slideR {
  from {
    transform: translateX(12px);
    opacity: 0.6;
  }
}
.mobilebar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px calc(8px + env(safe-area-inset-bottom, 0px));
  background: var(--surface);
  border-top: 1px solid var(--border);
  font-size: 11.5px;
  color: var(--meta);
}
.mb-track {
  flex: 1;
  height: 3px;
  border-radius: 999px;
  background: var(--border);
  overflow: hidden;
}
.mb-track i {
  display: block;
  height: 100%;
  background: var(--accent);
  transition: width var(--base);
}
.toast {
  position: fixed;
  left: 50%;
  bottom: 78px;
  z-index: 60;
  transform: translateX(-50%);
  max-width: 86vw;
  padding: 9px 14px;
  border-radius: var(--radius-md);
  background: var(--fg);
  color: var(--surface);
  font-size: 12.5px;
  box-shadow: var(--whisper);
}
.spin {
  animation: spin 700ms linear infinite;
}
@media (max-width: 1100px) {
  .col-read {
    padding: 14px 12px 120px;
  }
  .booktitle {
    font-size: 23px;
  }
}
</style>
