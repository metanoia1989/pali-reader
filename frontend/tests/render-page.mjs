// Server-render the whole reading page.
//
// The component harness in render.mjs checks one card at a time. This one
// mounts the page itself — the workspace, the catalogue rail, the contents
// rail, the segments — because the wiring between them is where the mistakes
// have actually been: an event bound to a name that does not exist, a prop that
// stopped being passed, a panel that renders nothing when it should.
import { createSSRApp, h } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { renderToString } from 'vue/server-renderer'
import { createRouter, createMemoryHistory } from 'vue-router'
import ReaderView from '../src/views/ReaderView.vue'
import { useReader } from '../src/store/reader'
import { useSettings } from '../src/store/settings'

const store = new Map()
globalThis.localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
}
globalThis.document = {
  documentElement: { style: { setProperty() {}, setAttribute() {} }, setAttribute() {} },
  querySelector: () => null,
  addEventListener() {},
  removeEventListener() {},
}
globalThis.window = { innerWidth: 1600, innerHeight: 1000, addEventListener() {}, removeEventListener() {} }
globalThis.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
globalThis.requestAnimationFrame = (fn) => setTimeout(fn, 0)

const WORD = /[\p{L}\p{M}']+/gu
const tok = (t) => {
  const out = []
  let m
  WORD.lastIndex = 0
  while ((m = WORD.exec(t))) out.push([m.index, m[0].length, 1])
  return out
}
const seg = (seq, kind, text, extra = {}) => ({ seq, kind, para: 0, level: 4, text, tokens: tok(text), ...extra })

const SEGMENTS = [
  seg(1, 'heading', 'saṃyuttanikāyo', { level: 1 }),
  seg(2, 'heading', 'sagāthāvaggasaṃyuttapāḷi', { level: 2 }),
  seg(3, 'center', 'namo tassa bhagavato arahato sammāsambuddhassa'),
  seg(4, 'heading', '1. devatāsaṃyuttaṃ', { level: 3 }),
  seg(5, 'heading', '1. naḷavaggo', { level: 3 }),
  seg(6, 'heading', '1. oghataraṇasuttaṃ', { level: 3 }),
  seg(7, 'prose', '1. evaṃ me sutaṃ – ekaṃ samayaṃ bhagavā sāvatthiyaṃ viharati.'),
  seg(8, 'verse', '"cirassaṃ vata passāmi,\nbrāhmaṇaṃ parinibbutaṃ."'),
]

const BOOK = {
  book: { id: 'mula_sa_01', name: 'sagāthāvaggasaṃyuttapāḷi', nameZh: '有偈品相应', bookNameZh: '有偈品相应' },
  // Headings carry their own translations: the contents covers the whole book
  // while the segment window only covers what is loaded.
  tocRefs: {
    4: { zh: '1. 天子相应', en: '1. The Devatā Saṃyutta' },
    5: { zh: '1. 芦苇品' },
    6: { zh: '1. 渡流经' },
  },
  toc: [
    { name: '1. devatāsaṃyuttaṃ', level: 3, seq: 4, para: 0 },
    { name: '1. naḷavaggo', level: 3, seq: 5, para: 0 },
    { name: '1. oghataraṇasuttaṃ', level: 3, seq: 6, para: 0 },
  ],
}
const CATALOG = {
  baskets: [
    {
      code: 'mula', name: '根本三藏', namePi: 'Mūlasāsana', bookCount: 61,
      categories: [
        {
          id: 'sa', name: 'suttantapiṭaka (saṃyuttanikāya)', nameZh: '经藏 · 相应部', namePi: 'Saṃyuttanikāya',
          pitaka: 'sutta', books: [{ id: 'mula_sa_01', name: 'sagāthāvaggasaṃyuttapāḷi', nameZh: '有偈品相应', basket: 'mula' }],
        },
      ],
    },
  ],
}
const MARKS = {
  bookId: 'mula_sa_01',
  picks: [
    { key: 'g|buddha|masc|masc|nom|sg', segment: 7, wordIndex: 4, kind: 'grammar', pos: 'masc', gender: 'masc', case: 'nom', number: 'sg' },
    { key: 'm|DPD|1:一时', segment: 7, wordIndex: 4, kind: 'meaning', meaning: '一时', meaningSource: 'DPD' },
  ],
  notes: [{ id: 1, segment: 7, wordIndex: -1, kind: 'segment', body: '结集时的套语。', updatedAt: new Date().toISOString() }],
  translations: [{ segment: 7, text: '如是我闻：一时，世尊住在舍卫城。', updatedAt: new Date().toISOString() }],
  refs: {
    6: { zh: '1. 渡流经', en: '1. The Flood' },
    7: {
      zh: '如是我闻。一时，世尊住在舍卫城。',
      en: 'Thus have I heard. At one time the Blessed One was dwelling at Sāvatthī.',
    },
  },
}

globalThis.fetch = async (url) => {
  const u = String(url)
  let body = {}
  if (u.includes('/api/catalog')) body = CATALOG
  else if (u.includes('/segments')) body = { items: SEGMENTS, total: 8, done: true }
  else if (u.includes('/marks')) body = MARKS
  else if (u.includes('/api/books/')) body = BOOK
  else if (u.includes('/api/me')) body = { user: null }
  return { ok: true, status: 200, json: async () => body, text: async () => JSON.stringify(body) }
}

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', component: { render: () => null } },
    { path: '/read/:bookId', name: 'read', component: ReaderView },
    { path: '/:p(.*)*', component: { render: () => null } },
  ],
})
await router.push('/read/mula_sa_01')
await router.isReady()

const pinia = createPinia()
setActivePinia(pinia)

// The page's own load is asynchronous, and SSR does not wait for a store
// action. Running it first is what makes the render below show real segments
// rather than an empty shell — and the segments are the part worth checking.
const reader = useReader()
await reader.open('mula_sa_01')
// Which of the two ways of showing 参考译文 to render. The mode is passed in so
// the same bundle proves both: in click mode opening one paragraph leaves every
// other one alone, and in 全部 mode every paragraph that has a translation shows
// it without being asked — which is the default, and what the page would look
// like for a reader who has never touched the setting.
const MODE = process.argv[2] === 'click' ? 'click' : 'all'
const settings = useSettings()
settings.refMode = MODE
if (MODE === 'click') {
  reader.toggleRefOpen(7, 'zh')
  reader.toggleRefOpen(7, 'en')
}

const app = createSSRApp({ render: () => h(ReaderView) })
app.use(pinia)
app.use(router)
process.stdout.write(await renderToString(app))
