<script setup>
// The English dictionary popup: one word of a reference translation and what it
// means. Nothing else.
//
// It is deliberately NOT the Pāḷi panel (WordLookup.vue) and not the floating
// popups that panel opens. Those are a tool for working: they record readings,
// they stack, they are dragged into an arrangement and kept until the reader
// closes them. This is a glance at a word they did not write and are not
// deciding anything about. There is no 选 button, no history, no chip written
// back under the sentence — one word, its meanings, and the next click closes
// it. Keeping the two apart is what keeps the panel's record trustworthy.
//
// Look and behaviour are taken from the sibling EnglishReading project's
// WordPopup.vue (the same pos/definition rows, the same caret and above/below
// flip, the same bottom card on a phone); the palette is this project's.
import { computed } from 'vue'
import { Loader2, SearchX, TriangleAlert, X } from 'lucide-vue-next'
import { expandSenses } from '../utils/ensenses'

const props = defineProps({
  popup: { type: Object, required: true },
})
const emit = defineEmits(['close'])

const rows = computed(() => expandSenses(props.popup.senses))

// `above` is drawn with the popup's bottom edge at y, so its height — which is
// its content's — does not leave a gap under the word.
const style = computed(() => {
  if (props.popup.sheet) return {}
  const base = { left: `${props.popup.x}px`, top: `${props.popup.y}px`, width: `${props.popup.w}px` }
  return props.popup.placement === 'above'
    ? { ...base, transform: 'translateY(-100%)' }
    : base
})

const headword = computed(() => props.popup.head || props.popup.word)
</script>

<template>
  <Transition name="pop">
    <div
      v-if="popup.visible"
      class="en-pop"
      :class="[popup.sheet ? 'en-pop--sheet' : `en-pop--${popup.placement}`]"
      :style="style"
      role="dialog"
      aria-label="英文词典"
      @click.stop
    >
      <span
        v-if="!popup.sheet"
        class="pop-caret"
        :style="{ left: `${popup.caretX}px` }"
        aria-hidden="true"
      />
      <div class="pop-clip">
        <header class="en-head">
          <div class="en-word">
            <span class="hw" lang="en">{{ headword }}</span>
            <span v-if="popup.phonetic" class="ph">{{ popup.phonetic }}</span>
          </div>
          <button class="iconbtn" type="button" title="关闭" @click="emit('close')">
            <X :size="16" :stroke-width="1.7" aria-hidden="true" />
            <span class="sr-only">关闭英文词典</span>
          </button>
        </header>

        <!-- The word the reader tapped, when the entry is filed under another
             form. Without it "uses" answered with "use" looks like a mistake. -->
        <p v-if="popup.via" class="en-via">词条 {{ popup.via }}</p>

        <div v-if="popup.loading" class="en-state">
          <Loader2 class="spinner" :size="16" :stroke-width="1.8" aria-hidden="true" />
          <span>正在查 {{ popup.word }}…</span>
        </div>

        <div v-else-if="popup.error" class="en-state">
          <TriangleAlert :size="18" :stroke-width="1.6" aria-hidden="true" />
          <span>{{ popup.error }}</span>
        </div>

        <!-- Not an error and not a missing word: the add-on dictionary has not
             been imported on this deployment. Saying "no entry" here would be a
             lie about the word. -->
        <div v-else-if="popup.available === false" class="en-state">
          <SearchX :size="18" :stroke-width="1.6" aria-hidden="true" />
          <span>英文词典尚未导入</span>
        </div>

        <div v-else-if="popup.notFound" class="en-state">
          <SearchX :size="18" :stroke-width="1.6" aria-hidden="true" />
          <span>词典暂无收录：{{ popup.word }}</span>
        </div>

        <div v-else class="en-senses">
          <div v-for="(r, i) in rows" :key="i" class="en-sense">
            <span class="en-pos">{{ r.pos }}</span>
            <span class="en-def">{{ r.def }}</span>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
/* The caret: a square turned 45°, keeping only the two edges that face the
   word, so the card looks pinned to the word that was tapped rather than
   floating near it. */
.pop-caret {
  position: absolute;
  width: 12px;
  height: 12px;
  margin-left: -6px;
  background: var(--surface);
  transform: rotate(45deg);
}
.en-pop--below .pop-caret {
  top: -6px;
  border-top: 1px solid var(--border);
  border-left: 1px solid var(--border);
}
.en-pop--above .pop-caret {
  bottom: -6px;
  border-right: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
}

.en-pop {
  position: fixed;
  z-index: 55;
  display: flex;
  flex-direction: column;
  max-height: min(340px, calc(100vh - 24px));
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  box-shadow: var(--whisper);
  /* Not `overflow: hidden`: the caret pokes out of the card. The rounding is
     clipped by the inner box instead. */
  overflow: visible;
}

.pop-clip {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  border-radius: inherit;
  overflow: hidden;
}

/* A phone: a small card at the bottom, where the thumb already is. No scrim —
   the popup is dismissed by the next tap anywhere, and a scrim would make
   reading past it a two-step operation. */
.en-pop--sheet {
  left: 6px;
  right: 6px;
  bottom: 6px;
  z-index: 45;
  max-height: 48vh;
  padding-bottom: env(safe-area-inset-bottom, 0);
}

.en-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 8px 9px 14px;
  border-bottom: 1px solid var(--border-soft);
}
.en-word {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}
/* Not the reading surface's .w: this word is not tappable, and the dotted
   underline that says "you can tap this" would be a lie here. */
.en-word .hw {
  font-size: 17px;
  font-weight: 500;
  letter-spacing: -0.01em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.en-word .ph {
  font-family: var(--mono);
  font-size: 11.5px;
  color: var(--meta);
  white-space: nowrap;
}

.en-via {
  padding: 6px 14px 0;
  font-size: 11.5px;
  color: var(--meta);
}

.en-senses {
  overflow-y: auto;
  padding: 6px;
  overscroll-behavior: contain;
}
/* One meaning per row: the tag in its own column so the meanings line up, and
   the definition free to run on. */
.en-sense {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  column-gap: 8px;
  align-items: start;
  padding: 6px 8px;
  border-radius: var(--radius-sm);
  font-size: 13.5px;
  line-height: 1.65;
  color: var(--fg);
}
.en-pos {
  font-family: var(--mono);
  font-size: 11px;
  line-height: 1.9;
  color: var(--meta);
}
.en-def {
  min-width: 0;
  /* The definitions are multi-line text, and the line breaks are content:
     ECDICT puts the general gloss first and the technical ones after it. */
  white-space: pre-line;
  overflow-wrap: anywhere;
}

.en-state {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 16px 18px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--muted);
}

@media (max-width: 640px) {
  .en-sense {
    font-size: 14px;
    padding: 8px;
  }
}

.pop-enter-active,
.pop-leave-active {
  transition: opacity var(--fast), transform var(--fast);
}
.pop-enter-from,
.pop-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
