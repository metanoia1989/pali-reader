<script setup>
// Renders a dictionary string that carries emphasis.
//
// DPD marks the case ending inside a compound split with <b>; the server turns
// those two tags into the sentinel bytes \x01 and \x02 and drops every other
// tag, so what arrives here is plain text plus markers. Splitting on the markers
// and wrapping the middle in <b> means the reader gets the emphasis without any
// v-html anywhere in the application.
import { computed } from 'vue'

const props = defineProps({
  text: { type: String, default: '' },
})

const parts = computed(() => {
  const out = []
  let emphasis = false
  for (const chunk of String(props.text).split(/[\u0001\u0002]/)) {
    if (chunk) out.push({ emphasis, text: chunk })
    emphasis = !emphasis
  }
  return out
})
</script>

<template>
  <template v-for="(p, i) in parts" :key="i">
    <b v-if="p.emphasis" class="em">{{ p.text }}</b>
    <template v-else>{{ p.text }}</template>
  </template>
</template>

<style scoped>
.em {
  font-weight: 500;
  color: var(--accent);
}
</style>
