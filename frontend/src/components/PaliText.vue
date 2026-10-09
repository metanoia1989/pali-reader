<script setup>
// The Pāḷi text of one segment, tokenised on the server.
//
// Tokens arrive as [offset, length, flags] triples into `text`. Rendering them
// means walking the gaps between them, so punctuation, spacing and the like are
// emitted verbatim and only real words become interactive.
//
// The compound handling is the point of this component: when the reader has
// settled on a split for a word, the parts are drawn washed together with a
// separator, and each part is still individually tappable.
import { computed } from 'vue'
import { useSettings } from '../store/settings'

const props = defineProps({
  seg: { type: Object, required: true },
  picksFor: { type: Function, required: true },
  splitFor: { type: Function, required: true },
  openKey: { type: String, default: '' },
  // A heading is the same words in a different face. It used to be printed as
  // plain text, so none of it could be tapped — and a heading is exactly where
  // a reader meets a word they do not know.
  heading: { type: Boolean, default: false },
})
const emit = defineEmits(['pick'])

const S = useSettings()
const FLAG_KNOWN = 1

// pieces is a flat list of {kind: 'text' | 'word' | 'part', ...}. A split word
// expands into a 'compound' run of parts so the wrapper can be drawn once.
const pieces = computed(() => {
  const seg = props.seg
  const text = seg.text || ''
  const toks = seg.tokens || []
  const out = []
  let at = 0
  let compound = null

  // The canon's own paragraph number, which the source writes inline and which
  // scholars checked by hand. It is shown in bold and is not a token — the
  // tokeniser skips runs of digits — so it is emitted as its own piece here.
  const lead = /^(\s*\d+(?:\s*[–-]\s*\d+)?\s*\.)/.exec(text)
  if (lead) {
    out.push({ kind: 'num', text: lead[1] })
    at = lead[1].length
  }

  const flushCompound = () => {
    if (compound) {
      out.push(compound)
      compound = null
    }
  }

  // Variant readings were cut out of the text at import time and stored with
  // the position they stood at. Putting them back is a second cursor walking
  // the same offsets, so they land between the right words.
  const variants = S.variants ? (seg.variants || []) : []
  let vi = 0
  const flushVariants = (upto) => {
    while (vi < variants.length && variants[vi][0] <= upto) {
      out.push({ kind: 'variant', text: variants[vi][1] })
      vi++
    }
  }

  // The edition bolds the word a commentary gloss is about, and that emphasis
  // is the only thing marking it as a headword. The markup was stripped at
  // import; the runs were recorded, so they go back the same way variants do.
  const bold = (seg.bold || []).map(([o, l]) => [o, o + l])

  // A gap in the token stream is not necessarily plain: it can carry the
  // edition's own line break (a verse's lines are joined with \n and would
  // otherwise collapse into one run-on paragraph) and it can be part of a bold
  // run. Both are resolved here, on the same slice of text.
  const emitGap = (from, to) => {
    if (to <= from) return
    let cursor = from
    // Break the gap wherever a line break or a bold boundary falls.
    const bounds = new Set([from, to])
    for (let i = from; i < to; i++) {
      if (text[i] === '\n') {
        bounds.add(i)
        bounds.add(i + 1)
      }
    }
    for (const [bs, be] of bold) {
      if (bs > from && bs < to) bounds.add(bs)
      if (be > from && be < to) bounds.add(be)
    }
    const marks = [...bounds].filter((x) => x >= from && x <= to).sort((a, b) => a - b)
    for (let k = 0; k < marks.length - 1; k++) {
      const a = marks[k]
      const b = marks[k + 1]
      if (b <= a) continue
      const chunk = text.slice(a, b)
      if (chunk === '\n') {
        out.push({ kind: 'br' })
        cursor = b
        continue
      }
      if (chunk === '') continue
      const isBold = bold.some(([bs, be]) => a >= bs && b <= be)
      out.push({ kind: isBold ? 'bold' : 'text', text: chunk })
      cursor = b
    }
    if (cursor < to) out.push({ kind: 'text', text: text.slice(cursor, to) })
  }

  toks.forEach((t, i) => {
    const [off, len] = t
    if (off > at) {
      flushCompound()
      emitGap(at, off)
    }
    flushVariants(off)
    const surface = text.slice(off, off + len)
    const known = (t[2] & FLAG_KNOWN) !== 0
    const runs = splitBold(surface, off, bold)

    // The reader's own decision about this occurrence, if they made one. A word
    // the dictionary does not know can still carry readings the reader recorded
    // themselves, so this is looked up regardless of `known`.
    const split = props.splitFor(seg.seq, i)
    const parts = split ? split.split('+').map((p) => p.trim()).filter(Boolean) : null
    const marked = props.picksFor(seg.seq, i).length > 0

    if (parts && parts.length > 1) {
      if (!compound) compound = { kind: 'compound', parts: [], ids: [] }
      compound.parts.push({ text: surface, key: `${seg.seq}:${i}`, index: i, known, emphasised, marked })
      compound.ids.push(i)
      // A multi-word split cannot be drawn from one surface form, so the split
      // is shown as a chip on the panel instead; here the word renders plain.
      if (parts.length !== 1 && compound.parts.length === 1) {
        compound.splitLabel = parts.join(' + ')
      }
    } else {
      flushCompound()
      out.push({
        kind: 'word',
        text: surface,
        key: `${seg.seq}:${i}`,
        index: i,
        known,
        runs,
        marked,
      })
    }
    at = off + len
  })
  flushCompound()
  flushVariants(text.length)
  emitGap(at, text.length)
  return out
})

// splitBold cuts a word into the runs the edition emphasises and the runs it
// does not.
//
// The emphasis can end inside a word: the commentary writes
// <span class="bld">kāmayamānassā</span>ti, bolding the headword and leaving the
// quotation particle. Bolding only words wholly inside a run meant such a word
// was never emphasised at all, which is the common case in the commentaries.
// The word must stay one clickable unit, so the emphasis is drawn inside it.
function splitBold(surface, off, bold) {
  const runs = bold
    .map(([bs, be]) => [Math.max(bs, off) - off, Math.min(be, off + surface.length) - off])
    .filter(([a, b]) => b > a)
    .sort((x, y) => x[0] - y[0])
  if (!runs.length) return [{ text: surface, b: false }]

  const out = []
  let at = 0
  for (const [a, b] of runs) {
    if (a > at) out.push({ text: surface.slice(at, a), b: false })
    out.push({ text: surface.slice(a, b), b: true })
    at = b
  }
  if (at < surface.length) out.push({ text: surface.slice(at), b: false })
  return out
}

function tap(piece) {
  emit('pick', { word: piece.text, segment: props.seg.seq, wordIndex: piece.index })
}
</script>

<template>
  <div :class="heading ? 'seg-heading-body' : 'seg-pali'">
    <template v-for="(p, i) in pieces" :key="i">
      <template v-if="p.kind === 'text'">{{ p.text }}</template>
      <br v-else-if="p.kind === 'br'" />
      <b v-else-if="p.kind === 'num'" class="pn">{{ p.text }}</b>
      <b v-else-if="p.kind === 'bold'" class="ed">{{ p.text }}</b>
      <span v-else-if="p.kind === 'variant'" class="v" :title="'异读：' + p.text">{{ p.text }}</span>
      <span v-else-if="p.kind === 'compound'" class="cmp">
        <template v-for="(part, j) in p.parts" :key="j">
          <span v-if="j" class="plus">+</span>
          <span
            class="w"
            role="button"
            tabindex="0"
            :class="{ 'is-open': openKey === part.key, marked: part.marked }"
            @click="tap(part)"
            @keydown.enter.prevent="tap(part)"
            @keydown.space.prevent="tap(part)"
            ><template v-for="(q, qi) in part.runs" :key="qi"
              ><b v-if="q.b" class="ed">{{ q.text }}</b><template v-else>{{ q.text }}</template></template
            ></span
          >
        </template>
        <span class="splitmark">拆</span>
      </span>
      <span
        v-else
        class="w"
        role="button"
        tabindex="0"
        :class="{ 'is-open': openKey === p.key, marked: p.marked }"
        @click="tap(p)"
        @keydown.enter.prevent="tap(p)"
        @keydown.space.prevent="tap(p)"
        ><template v-for="(q, qi) in p.runs" :key="qi"
          ><b v-if="q.b" class="ed">{{ q.text }}</b><template v-else>{{ q.text }}</template></template
        ></span
      >
    </template>
  </div>
</template>
