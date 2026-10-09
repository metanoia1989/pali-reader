// Drive the reader store the way the panel and the paragraph rows drive it.
//
// This is the one part of the feature that neither the SSR harness nor the Go
// tests reach: recording several readings against one word, striking one out,
// and doing it twice without ending up with two chips for one decision. The
// anchors and the multi-pick shape are the core of the round, so they are worth
// a test that runs without a browser.
import { createPinia, setActivePinia } from 'pinia'

const store = new Map()
globalThis.localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
}

const fails = []
function check(name, cond, detail = '') {
  if (cond) return console.log('  ok    ' + name)
  console.log('  FAIL  ' + name + (detail ? `  — ${detail}` : ''))
  fails.push(name)
}

// --- a stand-in for the API ------------------------------------------------
// The picks the "server" holds, keyed the way store.PickKey keys them.
const held = new Map()
const k = (...p) => p.join('|')

const SEGMENTS = [{ seq: 7, kind: 'prose', para: 1, level: 0, text: '1. evaṃ me sutaṃ.', tokens: [[3, 4, 1]] }]

let marksCalls = 0
const res = (body) => ({
  ok: true,
  status: 200,
  json: async () => body,
  text: async () => JSON.stringify(body),
})
globalThis.fetch = async (url, init = {}) => {
  const u = String(url)
  const method = init.method || 'GET'
  const body = init.body ? JSON.parse(init.body) : null

  if (u.includes('/api/work/picks')) {
    if (method === 'POST') {
      const key =
        body.kind === 'grammar'
          ? k('grammar', body.lemma, body.pos, body.gender, body.case, body.number, body.grammar)
          : body.kind === 'meaning'
            ? k('meaning', body.meaningSource, body.meaningKey)
            : k('split', body.split)
      if (!held.has(key)) held.set(key, { ...body, key })
      return res(held.get(key))
    }
    if (method === 'DELETE') {
      const q = new URL(u, 'http://x').searchParams
      const key = q.get('key')
      if (key) held.delete(key)
      else for (const kk of [...held.keys()]) held.delete(kk)
      return res({ ok: true })
    }
  }
  if (u.includes('/segments')) {
    return res({ items: SEGMENTS, total: 1, done: true })
  }
  if (u.includes('/marks')) {
    marksCalls++
    return res({ bookId: 'b', picks: [...held.values()], notes: [], translations: [], refs: {} })
  }
  if (u.includes('/api/books/')) {
    return res({ book: { id: 'b', name: 'n', nameZh: '名' }, toc: [] })
  }
  return res({})
}

const { useReader } = await import('../src/store/reader.js')

setActivePinia(createPinia())
const R = useReader()
await R.open('b')

console.log('— 一个词可以同时记下多条 —')
await R.addPick({
  segment: 7, wordIndex: 0, kind: 'grammar', surface: 'evaṃ',
  lemma: 'evaṃ', pos: 'ind', gender: '', case: '', number: '', grammar: 'indeclineable',
})
await R.addPick({
  segment: 7, wordIndex: 0, kind: 'meaning', surface: 'evaṃ',
  lemma: 'evaṃ', meaning: '如此', meaningKey: '1:DPD:如此', meaningSource: 'DPD',
})
let picks = R.picksFor(7, 0)
check('two picks on one word', picks.length === 2, `got ${picks.length}`)
check('both kinds present',
  picks.some((p) => p.kind === 'grammar') && picks.some((p) => p.kind === 'meaning'))

console.log('— 同一条点两次不会变成两条 —')
await R.addPick({
  segment: 7, wordIndex: 0, kind: 'grammar', surface: 'evaṃ',
  lemma: 'evaṃ', pos: 'ind', gender: '', case: '', number: '', grammar: 'indeclineable',
})
check('still two picks', R.picksFor(7, 0).length === 2, `got ${R.picksFor(7, 0).length}`)

console.log('— 一条可以单独划掉 —')
const grammarKey = R.picksFor(7, 0).find((p) => p.kind === 'grammar').key
await R.removePick(7, 0, grammarKey)
picks = R.picksFor(7, 0)
check('one left', picks.length === 1, `got ${picks.length}`)
check('the right one was removed', picks[0].kind === 'meaning', picks[0].kind)
check('the other word is untouched', R.picksFor(7, 1).length === 0)

console.log('— 拆解也是这个词的一条断言 —')
await R.addPick({ segment: 7, wordIndex: 0, kind: 'split', surface: 'evaṃ', split: 'eva + aṃ' })
check('splitFor returns it', R.splitFor(7, 0) === 'eva + aṃ', R.splitFor(7, 0))

console.log('— 全部划掉 —')
await R.clearPicks(7, 0)
check('nothing left for that word', R.picksFor(7, 0).length === 0)
check('splitFor is empty again', R.splitFor(7, 0) === '')

console.log('— 同一段窗口取两次不会出现重复 chip —')
held.clear()
await R.addPick({
  segment: 7, wordIndex: 0, kind: 'meaning', surface: 'evaṃ',
  lemma: 'evaṃ', meaning: '如此', meaningKey: '1:DPD:如此', meaningSource: 'DPD',
})
R.picks = {}
const before = marksCalls
await R.loadMarks(1, 10)
await R.loadMarks(1, 10)
check('loaded twice', marksCalls === before + 2)
check('but one chip, not two', R.picksFor(7, 0).length === 1, `got ${R.picksFor(7, 0).length}`)

console.log()
if (fails.length) {
  console.error(`${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('store checks: ok')
