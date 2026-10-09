// What the lookup panel records when a row is tapped.
//
// Kept out of the component because it is the part with rules in it: which two
// taps mean the same thing, and what payload the server gets. A tap on a
// grammar row that already matches a held pick has to *remove* it, and a tap
// that matches nothing has to add one; getting the identity comparison wrong in
// either direction leaves the reader unable to strike out a reading they have
// decided against.

// sameGrammar reports whether a recorded pick and an analysis row are the same
// reading. Every field that distinguishes one row of DPD's table from another
// takes part; the lemma alone would collapse nominative and accusative.
export function sameGrammar(a, p) {
  if (!p || p.kind !== 'grammar') return false
  return (
    (p.lemma || '') === (a.lemma || '') &&
    (p.pos || '') === (a.pos || '') &&
    (p.gender || '') === (a.gender || '') &&
    (p.case || '') === (a.case || '') &&
    (p.number || '') === (a.number || '')
  )
}

// heldGrammar finds the recorded pick a tap on this row would strike out.
export function heldGrammar(a, picks) {
  return (picks || []).find((p) => sameGrammar(a, p)) || null
}

// heldMeaning finds the recorded pick a tap on this gloss would strike out.
// The dictionary is part of the identity: the same words from DPD and from a
// Chinese dictionary are two different things to have recorded.
export function heldMeaning(sense, source, picks) {
  return (
    (picks || []).find(
      (p) => p.kind === 'meaning' && p.meaning === sense && p.meaningSource === source,
    ) || null
  )
}

// grammarPayload is what the server is asked to record for an analysis row.
export function grammarPayload(a, ctx) {
  return {
    segment: ctx.segment,
    wordIndex: ctx.wordIndex,
    kind: 'grammar',
    surface: ctx.word,
    lemma: a.lemma || ctx.lemma || '',
    lemmaId: ctx.lemmaId || 0,
    pos: a.pos || '',
    gender: a.gender || '',
    case: a.case || '',
    number: a.number || '',
    grammar: a.grammar || '',
  }
}

// meaningPayload is the same for one gloss line.
export function meaningPayload(sense, ctx) {
  return {
    segment: ctx.segment,
    wordIndex: ctx.wordIndex,
    kind: 'meaning',
    surface: ctx.word,
    lemma: ctx.lemma || '',
    lemmaId: ctx.lemmaId || 0,
    meaning: sense,
    // The key stands in for the gloss when the server builds the row's
    // identity, so a long gloss need not be part of a unique index.
    meaningKey: `${ctx.lemmaId || 0}:${ctx.sourceKey}:${sense}`,
    meaningSource: ctx.sourceName,
  }
}

// sensesOf splits one entry's gloss into separately recordable senses. DPD
// joins them with ";" and the reader needs them apart: recording "teaching"
// should not also record "law".
export function sensesOf(text) {
  return String(text || '')
    .split(/;\s*/)
    .map((x) => x.trim())
    .filter(Boolean)
}
