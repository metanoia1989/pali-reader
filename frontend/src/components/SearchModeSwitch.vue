<script setup>
// Which search is being run.
//
// This is what sits at the right of the field, where the ⌘K hint used to be.
// The hint was a reminder of a shortcut nobody had asked about, occupying the
// one place in the bar where the reader's own choice belongs; the shortcut
// still works, it is just no longer advertised at the cost of a control.
//
// A button and a menu rather than four inline tabs. Four Chinese labels would
// be wider than the field on a phone, and the labels are not self-explanatory —
// 本章 says nothing about "the chapter the contents has highlighted", and 译文
// says nothing about "one book, not the canon". A menu has room for both lines,
// and it is the same control at 390px and at 1440px.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'

const props = defineProps({
  modes: { type: Array, required: true },
  modelValue: { type: String, required: true },
})
const emit = defineEmits(['update:modelValue'])

const open = ref(false)
const wrap = ref(null)
const current = computed(() => props.modes.find((m) => m.id === props.modelValue) || props.modes[0])

function pick(id) {
  open.value = false
  if (id !== props.modelValue) emit('update:modelValue', id)
}

// A click anywhere else, or Escape, closes it. The reader is mid-read; a menu
// that stays open over the text until its own button is found again is a menu
// they will learn to avoid.
function onDocPointer(e) {
  if (!open.value) return
  if (wrap.value && !wrap.value.contains(e.target)) open.value = false
}
function onDocKey(e) {
  if (e.key === 'Escape') open.value = false
}
onMounted(() => {
  document.addEventListener('pointerdown', onDocPointer, true)
  window.addEventListener('keydown', onDocKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointer, true)
  window.removeEventListener('keydown', onDocKey)
})
watch(open, (v) => {
  if (v) emit('opened')
})
</script>

<template>
  <div ref="wrap" class="smode">
    <button
      type="button"
      class="smode-btn"
      :aria-expanded="open"
      aria-haspopup="menu"
      :title="'搜索范围：' + current.label + '（' + current.hint + '）'"
      @click="open = !open"
    >
      <span class="smode-label">{{ current.label }}</span>
      <span class="smode-short">{{ current.short || current.label }}</span>
      <ChevronDown :size="13" aria-hidden="true" />
    </button>

    <div v-if="open" class="smode-menu" role="menu">
      <button
        v-for="m in modes"
        :key="m.id"
        type="button"
        role="menuitemradio"
        :aria-checked="m.id === modelValue"
        class="smode-item"
        @click="pick(m.id)"
      >
        <span class="smode-tick"><Check v-if="m.id === modelValue" :size="13" /></span>
        <span>
          <span class="smode-name">{{ m.label }}</span>
          <span class="smode-hint">{{ m.hint }}</span>
        </span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.smode {
  position: relative;
  flex: none;
}
.smode-btn {
  display: flex;
  align-items: center;
  gap: 3px;
  height: 26px;
  padding: 0 6px 0 9px;
  border-radius: var(--radius-sm);
  color: var(--accent);
  font-size: 12.5px;
  font-weight: 500;
  white-space: nowrap;
}
.smode-btn:hover {
  background: var(--surface-warm);
}
.smode-short {
  display: none;
}
.smode-menu {
  position: absolute;
  top: 34px;
  right: 0;
  z-index: 70;
  width: 232px;
  padding: 5px;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--surface);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.1);
}
.smode-item {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  width: 100%;
  padding: 7px 8px;
  border-radius: var(--radius-sm);
  text-align: left;
}
.smode-item:hover {
  background: var(--surface-warm);
}
.smode-tick {
  flex: none;
  width: 14px;
  height: 18px;
  display: grid;
  place-items: center;
  color: var(--accent);
}
.smode-name {
  display: block;
  font-size: 13px;
  color: var(--fg);
}
.smode-hint {
  display: block;
  margin-top: 1px;
  font-size: 11.5px;
  line-height: 1.35;
  color: var(--meta);
}
@media (max-width: 860px) {
  .smode-menu {
    right: auto;
    left: 0;
    width: min(78vw, 260px);
  }
}
/* On a phone the bar is 390px wide and three other controls share it. The
   switch keeps its name — an icon alone would not say which of four searches is
   running — but the name is cut to two characters and the control to its
   essentials, and the field gets the rest. */
@media (max-width: 520px) {
  .smode-label {
    display: none;
  }
  .smode-short {
    display: inline;
  }
  .smode-btn {
    gap: 2px;
    padding: 0 4px 0 6px;
    font-size: 12px;
  }
}
</style>
