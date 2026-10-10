import { defineStore } from 'pinia'
import { useSettings } from './settings'
import { nextTick } from 'vue'
import { api } from '../api'

// Reader state.
//
// The corpus part of this — book, table of contents, segments — is identical
// for every reader and comes straight from a cached endpoint. The marks part
// is the signed-in reader's own and is fetched separately, so a guest can read
// the whole canon without a session.

const PAGE = 60

// The rule for whether a translation is shown: an explicit per-sentence
// override if the reader made one, otherwise the setting. Defined once, here,
// because the sentence button and the paragraph button must agree.
function settingsDefaultOn() {
  return useSettings().refDefaultOn
}

export const useReader = defineStore('reader', {
  state: () => ({
    bookId: '',
    book: null,
    toc: [],
    segments: [],
    total: 0,
    done: false,
    // The seq the loaded window starts at. Not always 1: a jump into the middle
    // of a long book loads that window directly rather than paging to it, so
    // "where the next page starts" is this plus what is already loaded.
    windowFrom: 1,
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
        this.done = false
        this.windowFrom = 1
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
          this.done = !!seg.done
          await this.loadMarks(1, this.segments.length ? this.segments[this.segments.length - 1].seq : PAGE)
        }
      } catch (e) {
        this.error = e.message || '加载失败'
      } finally {
        this.loading = false
      }
      if (opts.seq) this.scrollToSeq(opts.seq)
    },

    async loadMore() {
      if (this.loadingMore || this.done || !this.bookId) return
      this.loadingMore = true
      try {
        const from = this.windowFrom + this.segments.length
        const seg = await api.segments(this.bookId, from, PAGE)
        const items = seg.items || []
        if (items.length) {
          this.segments.push(...items)
          await this.loadMarks(from, items[items.length - 1].seq)
        }
        this.done = !!seg.done || items.length === 0
      } catch (e) {
        this.error = e.message || '加载失败'
      } finally {
        this.loadingMore = false
      }
    },

    // Marks arrive as flat arrays and are indexed by anchor here, once, so the
    // render path never searches.
    // Jump straight to the window holding `seq` instead of paging to it.
    //
    // Paging was the whole cost of a contents click: one round trip per 60
    // segments, so a heading near the end of a long book took hundreds of them
    // and the reader waited a minute. One request answers it. The window starts
    // a little above the target so the reader arrives with a little context and
    // can scroll up.
    async loadAt(seq) {
      if (!this.bookId) return
      const from = Math.max(1, (seq || 1) - 20)
      this.loadingMore = true
      try {
        const seg = await api.segments(this.bookId, from, PAGE)
        this.segments = seg.items || []
        this.windowFrom = from
        this.done = !!seg.done || this.segments.length === 0
        if (this.segments.length) {
          await this.loadMarks(from, this.segments[this.segments.length - 1].seq)
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
