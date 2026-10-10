import { defineStore } from 'pinia'
import { useSettings } from './settings'
import { nextTick } from 'vue'
import { api } from '../api'
import { PAGE, MAX_SEGMENTS, anchorShift, trimPlan } from '../utils/window'

// Reader state.
//
// The corpus part of this — book, table of contents, segments — is identical
// for every reader and comes straight from a cached endpoint. The marks part
// is the signed-in reader's own and is fetched separately, so a guest can read
// the whole canon without a session.
//
// PAGE (how much one request carries) now lives with the window's other
// arithmetic, in utils/window.js.

// The rule for whether a translation is shown: an explicit per-sentence
// override if the reader made one, otherwise the setting. Defined once, here,
// because the sentence button and the paragraph button must agree.
function settingsDefaultOn() {
  return useSettings().refDefaultOn
}

// The scrolling element the window is anchored to.
//
// A DOM node in a module binding rather than in state: Pinia would make it
// reactive, and a Proxy wrapped around an element is a performance trap and a
// comparison hazard. The reader binds it when it mounts; `scroller()` falls back
// to the class name so a jump still works before the binding happens.
let scrollerEl = null

export const useReader = defineStore('reader', {
  state: () => ({
    bookId: '',
    book: null,
    toc: [],
    segments: [],
    total: 0,
    // Reached the far end of the book. Two flags now, not one: the loaded
    // window has two ends and either can reach the edge of the book
    // independently — after a jump into the middle neither has.
    doneStart: true,
    doneEnd: false,
    // Which way a request is in flight, so the two ends can say so separately.
    loadingPrev: false,
    // The segment a jump is currently paging towards, 0 when none.
    jumpingTo: 0,
    // Which published translations the reader has opened, per segment and per
    // language. Session state on purpose: it answers "what am I reading right
    // now", not "how do I want the page set up" — that is settings.refMode.
    refsOpen: {},
    loading: false,
    loadingMore: false,

    // marks, keyed for O(1) lookup while rendering
    // "seg:word" -> [pick, ...]. A Pāḷi form is ambiguous, so a word carries
    // however many candidate readings the reader has recorded for it.
    picks: {},
    notes: {}, // "seg" -> [note]  and  "seg:word" -> [note]
    translations: {}, // seg -> {text, updatedAt}
    // seg -> {zh: "...", en: "..."} — every published translation of the
    // segment. Which of them is drawn is the reader's 参考译文 setting, applied
    // at render time so switching language needs no request.
    refs: {},
    // The published translation of each heading, keyed by segment then language.
    // Separate from refs because the contents covers the whole book while refs
    // only cover the segment window that has been loaded.
    tocRefs: {},

    // the word the panel is showing
    look: null, // { word, segment, wordIndex }
    panelOpen: false,
    // The left contents drawer on a phone. In the store rather than in the view
    // because the top bar owns the button that opens it.
    railDrawer: false,
    // Whether the top bar is currently slid away. In the store because the
    // scroll that decides it happens in the reading column and the bar that
    // obeys it is a different component.
    barHidden: false,

    // which segment the reader is looking at, for the toc scrollspy
    activeSegment: 1,
    error: '',
  }),

  getters: {
    // The window's two ends, derived from what is actually loaded rather than
    // tracked beside it: a separate counter drifts the first time an action
    // forgets to write it, and the drift shows up as a gap in the text.
    windowFrom: (s) => (s.segments.length ? s.segments[0].seq : 1),
    windowTo: (s) => (s.segments.length ? s.segments[s.segments.length - 1].seq : 0),
    // Whether the end of the book is on screen. Was `done`; the name is kept
    // because the endmark means the end of the book, not the end of anything
    // that happens to be loaded.
    done: (s) => s.doneEnd,
    picksFor: (s) => (seg, word) => s.picks[`${seg}:${word}`] || [],
    // The compound split the reader settled on, if any. It lives among the
    // picks because it is the same kind of assertion about the same word.
    splitFor: (s) => (seg, word) => {
      const p = (s.picks[`${seg}:${word}`] || []).find((x) => x.kind === 'split')
      return p ? p.split : ''
    },
    // The heading's translation, in whichever language the reader is reading.
    // Chinese first: this app's readers are reading in Chinese.
    // A getter, not an action: it derives a value from state, so it takes the
    // state as its first argument. Written as an action the first argument is
    // the call's own argument, and the call returns a function instead of a
    // string — which renders as the function's source text on the page.
    tocRefFor: (s) => (seq) => {
      const byLang = s.tocRefs[String(seq)]
      if (!byLang) return ''
      return byLang.zh || byLang.en || ''
    },
    notesFor: (s) => (seg, word) =>
      [...(s.notes[`${seg}:${word}`] || []), ...(word == null ? s.notes[`${seg}`] || [] : [])],
    segmentNotes: (s) => (seg) => s.notes[`${seg}`] || [],
    translationFor: (s) => (seg) => s.translations[seg] || null,
    segmentBySeq: (s) => (seq) => s.segments.find((x) => x.seq === seq) || null,
  },

  actions: {
    async open(bookId, opts = {}) {
      this.error = ''
      if (this.bookId !== bookId) {
        this.bookId = bookId
        this.book = null
        this.toc = []
        this.segments = []
        this.picks = {}
        this.refsOpen = {}
        this.notes = {}
        this.translations = {}
        this.refs = {}
        this.doneStart = true
        this.doneEnd = false
        this.look = null
        this.panelOpen = false
      }
      this.loading = !this.book
      try {
        if (!this.book) {
          const [b, seg] = await Promise.all([api.book(bookId), api.segments(bookId, 1, PAGE)])
          this.book = b.book
          this.toc = b.toc || []
          this.tocRefs = b.tocRefs || {}
          this.total = seg.total || 0
          this.segments = seg.items || []
          // The first window starts at the first segment, so the near end of the
          // book is behind the reader before they have scrolled at all.
          this.doneStart = true
          this.doneEnd = !!seg.done || this.segments.length === 0
          await this.loadMarks(1, this.segments.length ? this.segments[this.segments.length - 1].seq : PAGE)
        }
      } catch (e) {
        this.error = e.message || '加载失败'
      } finally {
        this.loading = false
      }
      if (opts.seq) this.scrollToSeq(opts.seq)
    },

    // The element the window is anchored to. The reader binds it on mount.
    bindScroller(el) {
      scrollerEl = el || null
    },

    scroller() {
      if (scrollerEl) return scrollerEl
      // A jump can happen before the binding — `open()` is called from a
      // watcher — so fall back to the class the column always carries.
      if (typeof document === 'undefined' || !document.querySelector) return null
      return document.querySelector('.col-read')
    },

    // --- extending the window, in either direction -----------------------
    //
    // The reader is a contiguous run of segments with two ends, and either end
    // can be extended. Before this there was only a forward path: `loadMore`
    // appended, and `loadAt` replaced the window with one starting twenty
    // segments above its target. So after any jump there were exactly twenty
    // segments above the reader and nothing behind them — scrolling up simply
    // ran out, which is what the owner saw on the desktop, and on a phone the
    // same missing path showed as "it stopped loading".

    async loadMore() {
      if (this.loadingMore || this.doneEnd || !this.bookId || !this.segments.length) return false
      this.loadingMore = true
      let ok = false
      try {
        const from = this.windowTo + 1
        const seg = await api.segments(this.bookId, from, PAGE)
        const items = seg.items || []
        if (items.length) {
          this.segments.push(...items)
          await this.loadMarks(from, items[items.length - 1].seq)
        }
        if (seg.total) this.total = seg.total
        this.doneEnd = !!seg.done || items.length === 0
        ok = true
      } catch (e) {
        this.error = e.message || '加载失败'
      } finally {
        this.loadingMore = false
      }
      // Moving forward, so the run being left behind is the head.
      await this.trim('head')
      return ok
    },

    // Extend the window at the front.
    //
    // Everything here exists to keep one promise: the line under the reader's
    // eyes does not move. Adding segments above the viewport pushes all of it
    // down by exactly the height that was added, so the same number of pixels
    // is put back into scrollTop after Vue has painted. Measured, not assumed —
    // a prepend without it throws the reader a whole page up the text.
    async loadPrev() {
      if (this.loadingMore || this.doneStart || !this.bookId || !this.segments.length) return false
      const col = this.scroller()
      const before = col ? col.scrollHeight : 0
      this.loadingMore = true
      this.loadingPrev = true
      let ok = false
      try {
        const head = this.segments[0].seq
        // The page that ends just before the window: the server returns rows
        // with seq >= from, so asking for one fewer than the window start gives
        // exactly the run that is missing, with no overlap to de-duplicate.
        const from = Math.max(1, head - PAGE)
        const count = head - from
        const seg = await api.segments(this.bookId, from, count)
        const items = seg.items || []
        if (items.length) {
          this.segments.unshift(...items)
          await this.loadMarks(items[0].seq, items[items.length - 1].seq)
          if (col) {
            await nextTick()
            col.scrollTop += anchorShift(before, col.scrollHeight)
          }
        }
        if (seg.total) this.total = seg.total
        this.doneStart = from <= 1 || items.length === 0
        ok = true
      } catch (e) {
        this.error = e.message || '加载失败'
      } finally {
        this.loadingMore = false
        this.loadingPrev = false
      }
      // Moving backward, so the run being left behind is the tail.
      await this.trim('tail')
      return ok
    },

    // Drop what has gone out of reach.
    //
    // Without this a reader who scrolls an 18,753-segment book ends up with
    // 18,753 segments in the DOM. The cap is MAX_SEGMENTS; which end is dropped
    // is decided by `prefer` (the end the reader is moving away from) and by
    // `trimPlan`, which will not drop the segment they are looking at nor the
    // context around it. What it costs: turning round and going back re-fetches
    // the run that was dropped, which is one request.
    //
    // A trim from the head moves the text exactly as a prepend does, so it is
    // anchored the same way — scrollHeight shrinks and scrollTop follows it
    // down. A trim from the tail moves nothing and is not touched.
    async trim(prefer) {
      const col = this.scroller()
      for (let round = 0; round < 4; round++) {
        const plan = trimPlan(this.segments.length, this.activeIndex(), MAX_SEGMENTS, prefer)
        if (!plan.head && !plan.tail) return
        const before = col ? col.scrollHeight : 0
        if (plan.head) {
          this.segments.splice(0, plan.head)
          // What was above is no longer loaded, so there is no longer anything
          // known to be behind the window.
          this.doneStart = false
        }
        if (plan.tail) {
          this.segments.splice(this.segments.length - plan.tail, plan.tail)
          this.doneEnd = false
        }
        if (col && plan.head) {
          await nextTick()
          col.scrollTop += anchorShift(before, col.scrollHeight)
        }
      }
    },

    // Where the reader is, as an index into the loaded window. -1 when the
    // window has been replaced under them and they are somewhere else.
    activeIndex() {
      const i = this.segments.findIndex((s) => s.seq === this.activeSegment)
      return i
    },

    // Marks arrive as flat arrays and are indexed by anchor here, once, so the
    // render path never searches.
    // Jump straight to the window holding `seq` instead of paging to it.
    //
    // Paging was the whole cost of a contents click: one round trip per 60
    // segments, so a heading near the end of a long book took hundreds of them
    // and the reader waited a minute. One request answers it.
    //
    // The target is put in the MIDDLE of the window, not twenty segments from
    // its head. Twenty was chosen when the window could only grow forward, so
    // the twenty were all the context a reader could ever get. Now that both
    // ends extend, the useful thing is room on both sides: the reader lands
    // with half a page behind them and half in front, and whichever way they
    // turn, the run they need is already there or one request away.
    async loadAt(seq) {
      if (!this.bookId) return
      const from = Math.max(1, (seq || 1) - Math.floor(PAGE / 2))
      this.loadingMore = true
      try {
        const seg = await api.segments(this.bookId, from, PAGE)
        this.segments = seg.items || []
        if (seg.total) this.total = seg.total
        this.doneStart = from <= 1
        this.doneEnd = !!seg.done || this.segments.length === 0
        if (this.segments.length) {
          await this.loadMarks(this.segments[0].seq, this.segments[this.segments.length - 1].seq)
        }
      } catch (e) {
        this.error = e.message || '加载失败'
      } finally {
        this.loadingMore = false
      }
    },

    async loadMarks(from, to) {
      try {
        const m = await api.marks(this.bookId, from, to)
        for (const p of m.picks || []) {
          const k = `${p.segment}:${p.wordIndex}`
          const list = this.picks[k] || []
          // A window can be loaded twice — a jump pages forward through ranges
          // an earlier load already covered — and a pick appearing twice would
          // draw two chips for one decision.
          if (list.some((x) => x.key === p.key)) continue
          this.picks[k] = [...list, p]
        }
        for (const n of m.notes || []) {
          const key = n.wordIndex >= 0 ? `${n.segment}:${n.wordIndex}` : `${n.segment}`
          if (!this.notes[key]) this.notes[key] = []
          this.notes[key].push(n)
        }
        for (const t of m.translations || []) this.translations[t.segment] = t
        for (const [seg, byLang] of Object.entries(m.refs || {})) {
          this.refs[seg] = { ...(this.refs[seg] || {}), ...byLang }
        }
      } catch {
        // Public reference translations are best-effort; failing to load them
        // must not stop the reader from reading.
      }
    },

    async showWord(word, segment, wordIndex) {
      this.look = { word, segment, wordIndex }
      this.panelOpen = true
    },

    closePanel() {
      this.panelOpen = false
      this.look = null
    },

    // --- writes ---------------------------------------------------------

    // One sentence's translation, toggled against whatever the setting says.
    //
    // The stored value is an OVERRIDE, not the state: `undefined` means "follow
    // the setting", and clicking the button writes an explicit yes or no. That
    // is what lets the same button mean "show this one" when the setting is
    // 点击展开 and "hide this one" when it is 全部展开.
    toggleRefOpen(segment, lang) {
      const now = this.refShown(segment, lang)
      const next = { ...(this.refsOpen[segment] || {}), [lang]: !now }
      this.refsOpen[segment] = next
    },

    refsOpenFor(segment) {
      return this.refsOpen[segment] || {}
    },

    // The override if there is one, otherwise the setting. Lives here rather
    // than in the component because the paragraph button has to ask the same
    // question of every sentence in the paragraph.
    refShown(segment, lang) {
      const o = this.refsOpen[segment]?.[lang]
      return o === undefined ? settingsDefaultOn() : o
    },

    // One paragraph at a time: every sentence of it, in one click. Worth having
    // because a paragraph is now several sentences, and a reader who wants the
    // Chinese for the paragraph does not want to click ten times.
    setRefShown(seqs, lang, on) {
      for (const s of seqs) {
        this.refsOpen[s] = { ...(this.refsOpen[s] || {}), [lang]: on }
      }
    },

    async addPick(payload) {
      const saved = await api.addPick({ bookId: this.bookId, ...payload })
      const k = `${saved.segment}:${saved.wordIndex}`
      const list = this.picks[k] ? [...this.picks[k]] : []
      if (!list.some((x) => x.key === saved.key)) list.push(saved)
      this.picks = { ...this.picks, [k]: list }
      return saved
    },

    // Removing a meaning the reader has decided against is as much a part of
    // reading as adding one, so it is one gesture and not a confirmation.
    async removePick(segment, wordIndex, key) {
      await api.deletePick(this.bookId, segment, wordIndex, key)
      const k = `${segment}:${wordIndex}`
      const list = (this.picks[k] || []).filter((x) => x.key !== key)
      const next = { ...this.picks }
      if (list.length) next[k] = list
      else delete next[k]
      this.picks = next
    },

    async clearPicks(segment, wordIndex) {
      await api.deletePick(this.bookId, segment, wordIndex)
      const next = { ...this.picks }
      delete next[`${segment}:${wordIndex}`]
      this.picks = next
    },

    async saveTranslation(segment, text) {
      const saved = await api.putTranslation({ bookId: this.bookId, segment, text })
      if (saved && saved.deleted) delete this.translations[segment]
      else if (saved) this.translations[segment] = saved
    },

    async saveNote(payload) {
      const saved = await api.putNote({ bookId: this.bookId, ...payload })
      const key = saved.wordIndex >= 0 ? `${saved.segment}:${saved.wordIndex}` : `${saved.segment}`
      if (!this.notes[key]) this.notes[key] = []
      const i = this.notes[key].findIndex((n) => n.id === saved.id)
      if (i >= 0) this.notes[key][i] = saved
      else this.notes[key].push(saved)
      return saved
    },

    async removeNote(note) {
      await api.deleteNote(note.id)
      const key = note.wordIndex >= 0 ? `${note.segment}:${note.wordIndex}` : `${note.segment}`
      const list = this.notes[key]
      if (list) {
        const i = list.findIndex((n) => n.id === note.id)
        if (i >= 0) list.splice(i, 1)
      }
    },

    async saveProgress(segment) {
      this.activeSegment = segment
      try {
        await api.putProgress(this.bookId, segment)
      } catch {
        /* progress is a convenience; losing one tick is not worth an error */
      }
    },

    // Jumping to a section means fetching until it is on the page.
    //
    // The contents list covers the whole book but only the first window of
    // segments is in the DOM, so a plain querySelector found nothing for
    // anything past it and the click did nothing at all — which is what a
    // reader sees as "the table of contents is broken". So this pages forward
    // until the target exists, with a guard so a seq that is not in the book
    // cannot spin forever, and it says so while it works.
    async scrollToSeq(seq) {
      if (!seq) return
      this.jumpingTo = seq
      try {
        // One request, not a walk. If the target is already loaded, this is a
        // no-op; otherwise the window containing it replaces what is shown.
        if (!document.querySelector(`[data-seq="${seq}"]`)) {
          await this.loadAt(seq)
        }
        // The reader asked for this segment, so that is where they are, whatever
        // the scrollspy would infer from the first paint. Setting it here is
        // what keeps the progress readout and the contents highlight in step
        // with a jump that lands mid-window; the scroll handler then takes over.
        this.activeSegment = seq
        // The window grows by whole pages, so the target is usually well above
        // the bottom; wait for Vue to paint it before measuring.
        await nextTick()
        const el = document.querySelector(`[data-seq="${seq}"]`)
        if (el) el.scrollIntoView({ block: 'start', behavior: 'auto' })
      } finally {
        this.jumpingTo = 0
      }
    },
  },
})
