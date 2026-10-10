<script setup>
// The search input, on its own.
//
// One of the three pieces the reader's search is built from — the field, the
// mode switch beside it, and the result list under it. They are separate
// components rather than one block because the field is the only part that
// survives every mode: the switch is a control that changes what the other two
// do, and the list is the only part that is ever empty, busy or long.
//
// It renders no border of its own. The box around it belongs to whatever
// arranges it with the switch — see `.rsearch-row` in base.css — so the same
// field can sit inside a plain bordered box (the top bar elsewhere) or beside a
// control without either arrangement reaching into this file.
import { computed, ref } from 'vue'
import { Search, X } from 'lucide-vue-next'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '搜索经文…' },
  label: { type: String, default: '搜索经文' },
  busy: { type: Boolean, default: false },
  // The switch that sits at the right of the field. A slot rather than a prop:
  // what is drawn there is a different piece of the feature, and this component
  // must not know which mode is selected or how many there are.
  clearable: { type: Boolean, default: true },
})
const emit = defineEmits(['update:modelValue', 'submit', 'escape', 'focus', 'blur'])

const el = ref(null)
const value = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

function onKeydown(e) {
  if (e.key === 'Enter') {
    e.preventDefault()
    emit('submit', props.modelValue)
  } else if (e.key === 'Escape') {
    // Escape inside a search field means "put it away", not "clear it and keep
    // the panel" — the reader wants their text back.
    e.stopPropagation()
    emit('escape')
  }
}

function clear() {
  emit('update:modelValue', '')
  el.value?.focus()
}

defineExpose({
  focus: () => el.value?.focus(),
  blur: () => el.value?.blur(),
  el,
})
</script>

<template>
  <div class="sfield">
    <Search :size="15" aria-hidden="true" />
    <input
      ref="el"
      v-model="value"
      type="search"
      :placeholder="placeholder"
      :aria-label="label"
      autocomplete="off"
      spellcheck="false"
      @keydown="onKeydown"
      @focus="emit('focus')"
      @blur="emit('blur')"
    />
    <button
      v-if="clearable && modelValue"
      type="button"
      class="sfield-clear"
      title="清除"
      aria-label="清除搜索"
      @click="clear"
    >
      <X :size="13" />
    </button>
  </div>
</template>

<style scoped>
.sfield {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  color: var(--meta);
}
.sfield input {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  outline: none;
  font-size: 13px;
  color: var(--fg);
}
/* Chrome draws its own ✕ inside type=search; two clear buttons is one too
   many, and the native one cannot be given the icon the rest of the app uses. */
.sfield input::-webkit-search-cancel-button {
  display: none;
}
.sfield-clear {
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  flex: none;
  border-radius: 999px;
  color: var(--meta);
}
.sfield-clear:hover {
  background: var(--surface-warm);
  color: var(--fg);
}
</style>
