// Server-render the reading components and print the HTML.
//
// There is no browser available to this session, and "it compiles" says nothing
// about whether a row renders, whether an icon button leaked its label as text,
// or whether a verse came out as one run-on line. Rendering the same components
// through vue/server-renderer answers those questions against real markup.
import { createSSRApp, h } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { renderToString } from 'vue/server-renderer'
import SegmentCard from '../src/components/SegmentCard.vue'
import WordLookup from '../src/components/WordLookup.vue'
import CatalogTree from '../src/components/CatalogTree.vue'
import { useSettings } from '../src/store/settings'

// --- the browser bits the components reach for ----------------------------
const store = new Map()
globalThis.localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
}
globalThis.document = { documentElement: { style: { setProperty() {} }, setAttribute() {} } }

// --- fixture text, tokenised the way the server tokenises ------------------
const WORD = /[\p{L}\p{M}']+/gu
function tokenize(text) {
  const out = []
  let m
  WORD.lastIndex = 0
  while ((m = WORD.exec(text))) out.push([m.index, m[0].length, 1])
  return out
}
function seg(seq, kind, text, extra = {}) {
  return { seq, kind, para: 0, level: 4, text, tokens: tokenize(text), ...extra }
}

const prose = seg(7, 'prose', '1. evaṃ me sutaṃ – ekaṃ samayaṃ bhagavā sāvatthiyaṃ viharati.')
const verse = seg(8, 'verse', 'manopubbaṅgamā dhammā,\nmanasā ce paduṭṭhena,\ntato naṃ dukkhamanveti.')
const heading = seg(6, 'heading', 'dhammapadapāḷi', { level: 2 })

// A segment carrying both things the edition marks and the CST apparatus adds:
// a bold run (the commentary's headword) and a variant reading that was cut out
// of the text at import and has to be put back at the right offset.
// Offsets are derived from the text rather than written by hand: a fixture with
// a hand-counted offset tests the fixture, not the renderer.
const MARKED_TEXT = 'atha kāmayamānassāti vuttaṃ. anubandhā honti bhikkhusaṅghañca.'
const BOLD_WORD = 'kāmayamānassā'
const VARIANT_AT = MARKED_TEXT.indexOf('anubandhā')
const marked = seg(9, 'prose', MARKED_TEXT, {
  // The edition writes <span class="bld">kāmayamānassā</span>ti: the emphasis
  // covers the headword, not the quotation particle after it.
  bold: [[MARKED_TEXT.indexOf(BOLD_WORD), BOLD_WORD.length]],
  variants: [[VARIANT_AT, '[anubaddhā (ka. sī. pī.)]']],
})

const picksFor = (s, w) =>
  s === 7 && w === 4
    ? [
        { key: 'g|samaya|noun|masc|nom|sg', kind: 'grammar', pos: 'noun', gender: 'masc', case: 'nom', number: 'sg' },
        { key: 'm|DPD|1:一会儿', kind: 'meaning', meaning: '一时；某个时候', meaningSource: 'DPD' },
      ]
    : []

const notesFor = (s) =>
  s === 7 ? [{ id: 1, body: '这一句是结集时的套语。', updatedAt: new Date().toISOString() }] : []

const refs = { zh: '如是我闻。一时，世尊住在舍卫城。', en: 'Thus have I heard. At one time the Blessed One was dwelling at Sāvatthī.' }

// The mode each card fixture is rendered in is pinned where the pinia is set
// up, further down: this harness asks what a card looks like when nothing has
// been asked for, and the default is 全部, which render-page.mjs covers.

// --- what a card in each state looks like ---------------------------------
const App = {
  setup() {
    return () =>
      h('div', { class: 'page' }, [
        h(SegmentCard, {
          seg: heading, picksFor, splitFor: () => '', notesFor: () => [], references: {},
          openKey: '', signedIn: true, active: false,
        }),
        h(SegmentCard, {
          seg: prose, picksFor, splitFor: () => '', notesFor,
          translation: { text: '如是我闻：一时，世尊住在舍卫城。', updatedAt: new Date().toISOString() },
          references: refs, openKey: '', signedIn: true, active: false,
        }),
        h(SegmentCard, {
          seg: verse, picksFor: () => [], splitFor: () => '', notesFor: () => [],
          translation: null, references: refs, openKey: '', signedIn: true, active: false,
        }),
        h(SegmentCard, {
          seg: marked, picksFor: () => [], splitFor: () => '', notesFor: () => [],
          translation: null, references: {}, openKey: '', signedIn: true, active: false,
        }),
      ])
  },
}

// --- and the panel, with a canned lookup ----------------------------------
const LOOKUP = {
  found: true,
  key: 'buddha',
  freq: 41230,
  headwords: [
    {
      id: 1, lemma: 'buddha', homonym: '1', pos: 'masc', phonetic: 'bud̪d̪ʰa',
      meaning1: 'the Buddha; Awakened One; enlightened', meaningLit: 'awakened',
      grammar: 'masc, from bujjhati', rootKey: '√budh', familyRoot: '√budh',
      construction: '√budh + ta', sanskrit: 'buddha', synonym: 'sambuddha', antonym: '',
      compoundConstruction: '', compoundType: '',
      declension: {
        stem: 'buddh',
        rows: [
          { case: 'nom', cells: [['o'], ['ā']] },
          { case: 'acc', cells: [['aṃ'], ['e']] },
        ],
        columns: ['masc sg', 'masc pl'],
        hit: { row: 1, column: 0, suffix: 'aṃ', form: 'buddhaṃ' },
      },
    },
  ],
  analyses: [
    { pos: 'masc', gender: 'masc', case: 'nom', number: 'sg', lemma: 'buddha', grammar: 'masc nom sg' },
    { pos: 'masc', gender: 'masc', case: 'acc', number: 'sg', lemma: 'buddha', grammar: 'masc acc sg' },
  ],
  meanings: [{ source: 'zh_bahh', name: '巴漢詞典', lang: 'zh', text: '佛陀；覺者' }],
  splits: [{ parts: ['buddha', 'aṃ'], resolved: true }],
  roots: [{ root: '√budh', sign: '', meaning: 'to know, to awaken' }],
}
globalThis.fetch = async (url) => {
  const body = String(url).includes('/dict/lookup') ? LOOKUP : {}
  return { ok: true, status: 200, json: async () => body, text: async () => JSON.stringify(body) }
}

const PanelApp = {
  setup() {
    return () =>
      h(WordLookup, {
        word: 'buddhaṃ', segment: 7, wordIndex: 4, bookId: 'mula_ku_02',
        picks: [], signedIn: true,
      })
  },
}

// The catalogue rail is loaded by the page on mount, which SSR does not run, so
// it is rendered here on its own with the same payload the API returns.
const CATALOG = {
  baskets: [
    {
      code: 'mula', name: '根本三藏', namePi: 'Mūlasāsana', bookCount: 61,
      categories: [
        {
          id: 'vi', name: 'vinayapiṭaka (khuddakanikāya)', nameZh: '律藏', namePi: 'Vinayapiṭaka',
          pitaka: 'vinaya', books: [],
        },
        {
          id: 'sa', name: 'suttantapiṭaka (saṃyuttanikāya)', nameZh: '经藏 · 相应部', namePi: 'Saṃyuttanikāya',
          pitaka: 'sutta',
          books: [{ id: 'mula_sa_01', name: 'sagāthāvaggasaṃyuttapāḷi', nameZh: '有偈品相应', basket: 'mula' }],
        },
      ],
    },
  ],
}
const TOC = [
  { name: '1. devatāsaṃyuttaṃ', level: 3, seq: 4, para: 0 },
  { name: '1. naḷavaggo', level: 3, seq: 5, para: 0 },
  { name: '1. oghataraṇasuttaṃ', level: 4, seq: 6, para: 0 },
]
const TOC_REFS = { 4: '1. 天子相应', 5: '1. 芦苇品', 6: '1. 渡流经' }

const TreeApp = {
  setup() {
    return () =>
      h(CatalogTree, {
        catalog: CATALOG.baskets, current: 'mula_sa_01', toc: TOC,
        tocRefFor: (seq) => TOC_REFS[seq] || '',
        activeSeq: 5, collapsed: false,
      })
  },
}

const pinia = createPinia()
setActivePinia(pinia)
const which = process.argv[2] || 'cards'
if (which === 'cards') {
  useSettings().variants = true
  // This harness asks what a card looks like when the reader has asked for
  // nothing, so it pins 点击: 全部 — the default — is render-page.mjs's job.
  useSettings().refMode = 'click'
}
const app = createSSRApp(which === 'panel' ? PanelApp : which === 'tree' ? TreeApp : App)
app.use(pinia)

// The catalogue rail navigates with router-link. Without a router installed the
// link is an unresolved component and renders as nothing at all — which looks
// exactly like a row that failed to render.
if (which === 'tree' || which === 'cards') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:p(.*)*', component: { render: () => null } }],
  })
  await router.push('/read/mula_sa_01')
  await router.isReady()
  app.use(router)
}

const html = await renderToString(app)
process.stdout.write(html)
