// The rules behind a tap on a row of the lookup panel.
//
// A tap that matches a held reading must strike it out; a tap that matches
// nothing must record one. Getting either direction wrong leaves the reader
// unable to take back a reading they have decided against — which is the whole
// reason a word can hold several.
import { createPinia, setActivePinia } from 'pinia'

const fails = []
function check(name, cond, detail = '') {
  if (cond) return console.log('  ok    ' + name)
  console.log('  FAIL  ' + name + (detail ? `  — ${detail}` : ''))
  fails.push(name)
}

setActivePinia(createPinia())
const { grammarPayload, heldGrammar, heldMeaning, meaningPayload, sensesOf } =
  await import('../src/utils/picks.js')

const ctx = { segment: 7, wordIndex: 4, word: 'dhammā', lemma: 'dhamma', lemmaId: 12 }

console.log('— 一条语法读法的身份 —')
const nom = { lemma: 'dhamma', pos: 'masc', gender: 'masc', case: 'nom', number: 'pl', grammar: 'masc nom pl' }
const acc = { ...nom, case: 'acc', grammar: 'masc acc pl' }
const held = [{ key: 'k1', kind: 'grammar', ...grammarPayload(nom, ctx) }]
check('the same reading is recognised', !!heldGrammar(nom, held))
// Case is the whole point of recording a reading: collapsing it would make the
// two impossible to hold at once, and impossible to tell apart afterwards.
check('a different case is a different reading', !heldGrammar(acc, held))
check('a different number is a different reading',
  !heldGrammar({ ...nom, number: 'sg' }, held))
check('a different lemma is a different reading',
  !heldGrammar({ ...nom, lemma: 'dhamman' }, held))
check('a meaning pick never matches a grammar row',
  !heldGrammar(nom, [{ key: 'k2', kind: 'meaning', meaning: 'x' }]))

console.log('— 记录一条语法读法 —')
const gp = grammarPayload(nom, ctx)
check('anchored on the occurrence',
  gp.segment === 7 && gp.wordIndex === 4 && gp.surface === 'dhammā', JSON.stringify(gp))
check('carries the reading', gp.kind === 'grammar' && gp.case === 'nom' && gp.number === 'pl')
check('falls back to the entry lemma when the row has none',
  grammarPayload({ ...nom, lemma: '' }, ctx).lemma === 'dhamma')

console.log('— 一条释义的身份 —')
const picks = [{ key: 'm1', kind: 'meaning', meaning: '法', meaningSource: 'DPD' }]
check('same words, same dictionary', !!heldMeaning('法', 'DPD', picks))
// The same gloss from two dictionaries is two different things to have
// recorded — they are different claims about the word.
check('same words, another dictionary', !heldMeaning('法', '巴漢詞典', picks))
check('another gloss', !heldMeaning('教说', 'DPD', picks))
check('a grammar pick never matches a gloss',
  !heldMeaning('法', 'DPD', [{ key: 'g', kind: 'grammar', case: 'nom' }]))

console.log('— 记录一条释义 —')
const mp = meaningPayload('法', { ...ctx, sourceKey: 'dpd', sourceName: 'DPD' })
check('carries the gloss and its source',
  mp.kind === 'meaning' && mp.meaning === '法' && mp.meaningSource === 'DPD')
check('the key identifies the entry, the dictionary and the gloss',
  mp.meaningKey === '12:dpd:法', mp.meaningKey)

console.log('— 一条词条拆成可分别记录的义项 —')
check('splits on the semicolon DPD uses',
  sensesOf('the Buddha; Awakened One; enlightened').length === 3)
check('trims and drops empties', sensesOf(' a ; ; b ').join('|') === 'a|b')
check('a gloss with no separator is one sense', sensesOf('法').length === 1)
check('nothing in, nothing out', sensesOf('').length === 0 && sensesOf(null).length === 0)

console.log()
if (fails.length) {
  console.error(`${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('pick checks: ok')
