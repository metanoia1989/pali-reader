<script setup>
// One line of the English 参考译文, with its words tappable.
//
// The Chinese line is deliberately not handled here: the dictionary is an
// English one, and a Chinese translation has nothing to look up in it.
//
// The Pāḷi text is tokenised on the server and the offsets are frozen in the
// database (铁律 3). That is because a Pāḷi word is a hard problem — compounds,
// sandhi, diacritics that are letters — and it has to be the same answer every
// time. An English sentence has none of that: a word is a run of letters, and a
// second implementation of "where does a word begin" is not being written here,
// simply because there is nothing to decide. What is left is the shape of the
// text: everything that is not a word must be printed exactly as it stands, so
// punctuation and spacing cannot move.
import { computed } from 'vue'

const props = defineProps({
  text: { type: String, default: '' },
})
const emit = defineEmits(['word'])

// A word: letters (with diacritics — the translations carry sāvatthī, jhāna)
// joined by an apostrophe or a hyphen, which are part of the word and are
// looked through by the server. Digits are not words: a number that opens a
// popup saying "no entry" is worse than a number that does nothing, the same
// rule the Pāḷi tokeniser follows.
const WORD = /[\p{L}\p{M}]+(?:['’\-][\p{L}\p{M}]+)*/gu

const pieces = computed(() => {
  const text = props.text || ''
  const out = []
  let at = 0
  let m
  WORD.lastIndex = 0
  while ((m = WORD.exec(text))) {
    if (m.index > at) out.push({ text: text.slice(at, m.index), word: false })
    out.push({ text: m[0], word: true })
    at = m.index + m[0].length
  }
  if (at < text.length) out.push({ text: text.slice(at), word: false })
  return out
})

function tap(piece, event) {
  emit('word', { word: piece.text, el: event.currentTarget })
}
</script>

<template>
  <template v-for="(p, i) in pieces" :key="i"
    ><span
      v-if="p.word"
      class="enw"
      role="button"
      tabindex="0"
      @click="tap(p, $event)"
      @keydown.enter.prevent="tap(p, $event)"
      @keydown.space.prevent="tap(p, $event)"
      >{{ p.text }}</span
    ><template v-else>{{ p.text }}</template></template
  >
</template>
