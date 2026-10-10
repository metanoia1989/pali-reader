// Server-render the three pieces of the reader's search, and the contents list
// that follows the reader.
//
// The point is separability as much as markup: the field, the mode switch and
// the result list are three components, and this renders each one on its own so
// that stays true after the interaction design lands. It also covers the states
// that are easy to get wrong and invisible in a screenshot of the happy path —
// nothing found, a request that failed, a query too short to run, more results
// than were shown.
import { createSSRApp, h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import SearchField from '../src/components/SearchField.vue'
import SearchModeSwitch from '../src/components/SearchModeSwitch.vue'
import SearchResults from '../src/components/SearchResults.vue'
import TocList from '../src/components/TocList.vue'
import { SEARCH_MODES } from '../src/utils/search.js'

globalThis.document = { documentElement: { style: { setProperty() {} } }, addEventListener() {} }
globalThis.window = { addEventListener() {}, removeEventListener() {}, innerWidth: 1440 }

const which = process.argv[2] || 'field'

const HITS = [
  {
    bookId: 'tika_an_04', bookName: '增支部注', segment: 1204, para: 88, kind: 'prose',
    snippet: '… sutte <mark>dukkha</mark>nirodho vutto …',
  },
  {
    bookId: 'tika_an_04', bookName: '增支部注', segment: 1590, para: 121, kind: 'verse',
    snippet: '… <mark>dukkha</mark>ṃ aniccaṃ …',
  },
]
const TITLE_HITS = [
  {
    bookId: 'mula_an_09', bookName: '增支部', segment: 1722, kind: 'heading', level: 3,
    name: '6. sandiṭṭhikanibbānasuttaṃ', ref: '6. 现见涅槃经',
  },
  { bookId: 'mula_an_04', bookName: '增支部', kind: 'book', name: 'aṅguttaranikāyo', ref: '增支部' },
]
const REF_HITS = [
  {
    bookId: 'tika_an_04', bookName: '增支部注', segment: 900, para: 12, kind: 'prose', lang: 'zh',
    snippet: '… 此中<b>苦</b>者，<mark>涅槃</mark>为灭 …',
  },
  {
    bookId: 'tika_an_04', bookName: '增支部注', segment: 901, para: 13, kind: 'prose', lang: 'en',
    snippet: '… the cessation of <mark>suffering</mark> …',
  },
]
const TOC = [
  { name: '1. naḷavaggo', level: 3, seq: 5 },
  { name: '1. oghataraṇasuttaṃ', level: 3, seq: 6 },
  { name: '2. arahatasuttaṃ', level: 3, seq: 30 },
]
const REFS = { 5: '1. 芦苇品', 6: '1. 渡流经' }

const cases = {
  field: () =>
    h(SearchField, { modelValue: 'dukkha', placeholder: '搜索本章…', label: '搜索经文' }),
  'field-empty': () => h(SearchField, { modelValue: '', placeholder: '搜索本章…' }),
  switch: () => h(SearchModeSwitch, { modes: SEARCH_MODES, modelValue: 'chapter' }),
  'switch-refs': () => h(SearchModeSwitch, { modes: SEARCH_MODES, modelValue: 'refs' }),
  chapter: () =>
    h(SearchResults, { hits: HITS, mode: 'chapter', query: 'dukkha', scope: '只搜索「1. 渡流经」这一章。' }),
  titles: () => h(SearchResults, { hits: TITLE_HITS, mode: 'title', query: '涅槃' }),
  refs: () =>
    h(SearchResults, {
      hits: REF_HITS, mode: 'refs', query: '涅槃',
      scope: '只搜索本书的中英参考译文；不搜索巴利原文，也不搜索其他书。',
    }),
  empty: () =>
    h(SearchResults, {
      hits: [], mode: 'refs', query: '涅槃',
      scope: '只搜索本书的中英参考译文；不搜索巴利原文，也不搜索其他书。',
    }),
  short: () => h(SearchResults, { hits: [], mode: 'chapter', query: 'd' }),
  error: () => h(SearchResults, { hits: [], mode: 'pali', query: 'dukkha', error: '无法连接服务器，请检查网络' }),
  loading: () => h(SearchResults, { hits: [], mode: 'pali', query: 'dukkha', loading: true }),
  limited: () => h(SearchResults, { hits: HITS, mode: 'pali', query: 'dukkha', limited: true }),
  toc: () => h(TocList, { entries: TOC, activeIndex: 1, refFor: (seq) => REFS[seq] || '' }),
  'toc-empty': () => h(TocList, { entries: [], activeIndex: -1 }),
}

const app = createSSRApp({ render: cases[which] || cases.field })
process.stdout.write(await renderToString(app))
