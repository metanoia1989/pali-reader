import { defineStore } from 'pinia'
import { api, getToken } from '../api'

// Reading settings.
//
// Everything here changes what the reader *sees* — how big the Pāḷi is, which
// translations and glosses are shown, whether the catalogue leads with Chinese
// or with Pāḷi. All of it is applied by writing CSS variables and store state,
// never by re-fetching: the data for both languages and both dictionaries is
// already in the response, so switching is instant.
//
// The settings live in localStorage so they work signed out and apply before
// the first paint. When the reader is signed in they are also pushed to the
// account, and the account's copy wins on a new device.

const KEY = 'pali.settings'

// The sizes 字号 offers. The labels describe the Pāḷi, not a percentage: what a
// reader wants to know is whether the text will be comfortable, not that they
// picked 115%.
export const SCALE_STEPS = [
  { value: 0.9, label: '小' },
  { value: 1, label: '标准' },
  { value: 1.15, label: '大' },
  { value: 1.3, label: '特大' },
]

export const DEFAULTS = {
  // Text scale, as a multiplier on the reading sizes.
  scale: 1,
  // 参考译文: the state a translation starts in.
  //
  //   off   nothing is shown until the reader asks
  //   all   every sentence shows what it has
  //
  // Only the STARTING state. The 中 / 英 buttons are drawn in both modes and
  // always toggle — one at the end of a sentence for that sentence, one at the
  // head of a paragraph for the whole paragraph. So 全部展开 is not a mode the
  // reader is locked into, and 隐藏 is not a mode with no way back: it is what
  // a reader who wants a clean page gets, and the buttons are how they open
  // one sentence at a time while reading closely.
  //
  // There were three values once, with a 点击展开 in between. It was wrong:
  // that is not a state, it is what the button does, and having it as a setting
  // made the other two mean something different from what they say.
  refMode: 'off', // off | all
  // 词典: which dictionaries the lookup panel offers. DPD is a choice in its
  // own right — it is the most complete of them, and a reader who wants one
  // authority rather than seven wants that one.
  dictLang: 'all', // all | dpd | zh | en
  // 目录与书名: which script leads in a list. 'both' puts Pāḷi under the Chinese.
  nameLang: 'both', // zh | pi | both
  // 显示异读: whether the variant readings recorded in the notes are drawn.
  variants: false,
  // 正文字体: the face the canon's own text is set in.
  bodyFont: 'serif', // serif | sans
}

// merge fills in anything a stored copy is missing, and drops anything whose
// type has changed since it was written. A settings blob is the one thing that
// outlives a deploy, so it has to survive one.
function merge(stored) {
  const out = { ...DEFAULTS }
  if (!stored || typeof stored !== 'object') return out
  for (const k of Object.keys(DEFAULTS)) {
    const v = stored[k]
    if (v === undefined || v === null) continue
    if (typeof v !== typeof DEFAULTS[k]) continue
    out[k] = v
  }
  return out
}

function read() {
  try {
    return merge(JSON.parse(localStorage.getItem(KEY) || 'null'))
  } catch {
    return merge(null)
  }
}

export const useSettings = defineStore('settings', {
  state: () => read(),

  getters: {
    // Whether every sentence shows its translations without being asked. The
    // per-sentence overrides in the reader store sit on top of this.
    refDefaultOn: (s) => s.refMode === 'all',

    // Whether a dictionary in this language should be offered, given 词典.
    allowsLang: (s) => (lang) => {
      if (s.dictLang === 'all') return true
      if (s.dictLang === 'dpd') return lang === 'dpd'
      return lang === s.dictLang
    },

    // The two lines of a name in a list, in the order 目录与书名 asks for. A
    // book with no Chinese name falls back to its Pāḷi one rather than showing
    // a blank, and the second line is dropped when it would repeat the first.
    primaryName: (s) => (zh, pi) => {
      if (s.nameLang === 'pi') return pi || zh || ''
      return zh || pi || ''
    },
    secondaryName: (s) => (zh, pi) => {
      if (s.nameLang !== 'both') return ''
      return zh ? pi || '' : ''
    },
  },

  actions: {
    set(k, v) {
      this[k] = v
      this.persist()
      this.apply()
    },

    reset() {
      Object.assign(this, DEFAULTS)
      this.persist()
      this.apply()
    },

    // Writes the preferences into the document, where the stylesheet reads
    // them. Kept out of the components so a reload and a change take the same
    // path.
    apply() {
      const el = document.documentElement
      el.style.setProperty('--scale', String(this.scale))
      // The reading face, not the interface one: 正文字体 is about the canon's
      // own text. --pali is the serif drawn for long stretches of Pāḷi; --ui is
      // the interface sans, which is what the rest of the page already uses.
      el.style.setProperty('--body-font', this.bodyFont === 'sans' ? 'var(--ui)' : 'var(--pali)')
      el.dataset.variants = this.variants ? 'on' : 'off'
    },

    persist() {
      try {
        localStorage.setItem(KEY, JSON.stringify(this.$state))
      } catch {
        // A full or disabled store is not a reason to break the page.
      }
      // Mirrored to the account when there is one, so the preferences follow
      // the reader to another device. Signed out this is a no-op, and a failure
      // must not disturb the local copy that already took effect.
      if (getToken()) {
        api.putSettings(this.$state).catch(() => {})
      }
    },

    // Pull the account's copy down when there is one. It wins on a new device.
    async loadFromAccount() {
      if (!getToken()) return
      try {
        const remote = await api.settings()
        if (remote && typeof remote === 'object') {
          Object.assign(this, merge(remote))
          this.persist()
          this.apply()
        }
      } catch {
        // Signed out, or offline. The local copy stands.
      }
    },
  },
})
