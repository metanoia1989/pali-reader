<script setup>
// The book's contents, as a list that follows the reader.
//
// There are two of these on screen at different widths — the desktop rail and
// the phone's drawer — and they used to be two separate blocks of markup with
// two separate behaviours: the rail tracked the reader's position and the
// drawer did not, so on a phone opening the contents always started at the top
// of the book, hundreds of entries away from where the reader was. Two copies
// of a list is two chances to fix one and forget the other, which is what
// happened; this is one component so it cannot.
//
// It owns its own scrolling box. On a phone the drawer is created when it
// opens (`v-if`), so the first positioning happens in `onMounted`, not when the
// page was built.
import { onMounted, ref, watch } from 'vue'
import { tocStyle } from '../utils/numbering'
import { followScrollTop } from '../utils/scroll'

const props = defineProps({
  entries: { type: Array, default: () => [] },
  activeIndex: { type: Number, default: -1 },
  // The heading's translation, if the corpus has one. Contents is navigation,
  // not the reading text: a chapter name in a language the reader does not read
  // is a name they cannot use.
  refFor: { type: Function, default: null },
  empty: { type: String, default: '本卷没有分节标题。' },
})
const emit = defineEmits(['goto'])

const box = ref(null)
const activeEl = ref(null)

function follow() {
  const next = followScrollTop(
    activeEl.value?.getBoundingClientRect(),
    box.value?.getBoundingClientRect(),
    box.value?.scrollTop || 0,
  )
  if (next !== null) box.value.scrollTop = next
}

// One frame, so the browser has laid the list out before it is measured. On the
// first render of a freshly opened drawer this is the difference between
// scrolling to the right item and scrolling nowhere.
function soon() {
  requestAnimationFrame(() => follow())
}

watch(() => props.activeIndex, soon)
watch(() => props.entries, soon)
onMounted(soon)
</script>

<template>
  <div ref="box" class="tocscroll">
    <button
      v-for="(t, i) in entries"
      :key="t.seq + ':' + i"
      :ref="(el) => { if (i === activeIndex) activeEl = el }"
      type="button"
      class="toc-item"
      :class="{ 'is-active': i === activeIndex }"
      :style="{
        paddingLeft: 10 + tocStyle(t.level).indent + 'px',
        fontSize: tocStyle(t.level).size + 'px',
      }"
      @click="emit('goto', t.seq)"
    >
      <span class="tn">{{ t.name }}</span>
      <span v-if="refFor && refFor(t.seq)" class="tref">{{ refFor(t.seq) }}</span>
    </button>
    <p v-if="!entries.length" class="caption" style="padding: 10px 0">{{ empty }}</p>
  </div>
</template>

<style scoped>
/* The scroller. `.col-toc` and the drawer each hand it whatever height is left
   under their own header, so the header — and the button that closes the
   drawer — stays where it was while the list moves under it. */
.tocscroll {
  flex: 1;
  min-height: 0;
  padding-bottom: 24px;
  overflow-y: auto;
  overscroll-behavior: contain;
}
/* A heading and its translation, stacked: at this column width a running line
   of Pāḷi followed by Chinese wraps in the middle of a name.
   No indent of its own — the row already carries the level's indentation, and a
   second one pushed the Chinese a whole level to the right of the Pāḷi it
   translates, which read as it being centred under a longer name. */
.toc-item .tref {
  display: block;
  margin-top: 1px;
  font-size: 12px;
  font-weight: 400;
  line-height: 1.4;
  color: var(--meta);
}
.toc-item.is-active .tref {
  color: var(--accent);
  opacity: 0.85;
}
</style>
