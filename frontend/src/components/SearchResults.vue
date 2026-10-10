<script setup>
// The result list, on its own.
//
// The third of the three pieces. It knows how to draw a hit of any of the four
// modes and what to say when there is nothing — and nothing else: no request,
// no debounce, no knowledge of which mode produced what it is showing beyond
// the label it prints. That is what makes it movable when the interaction
// design arrives.
//
// Why inline at all: the reader is in the middle of a page of text, and every
// mode here is answered by "a place in that text". Sending them to a separate
// results page means leaving the book to find out whether they needed to leave
// the book — and coming back to a scroll position the browser, not the app,
// decided on.
import { Loader2 } from 'lucide-vue-next'

defineProps({
  hits: { type: Array, default: () => [] },
  mode: { type: String, default: 'chapter' },
  loading: { type: Boolean, default: false },
  // The query the list is showing, echoed in the empty state so an answer about
  // a stale query is never mistaken for an answer about the current one.
  query: { type: String, default: '' },
  error: { type: String, default: '' },
  // True when the server stopped at the limit and there are more.
  limited: { type: Boolean, default: false },
  // Shown instead of a bare "no results": what was searched, so a mode with a
  // narrower scope than the reader assumed says so.
  scope: { type: String, default: '' },
})
const emit = defineEmits(['pick', 'more'])

// The server wraps the match in <mark> and escapes the rest, so this is markup
// it produced, not text from the corpus. Same contract the search page uses.
</script>

<template>
  <div class="sres" role="listbox">
    <div v-if="loading" class="sres-state">
      <Loader2 :size="15" class="spin" />
      <span>正在搜索…</span>
    </div>

    <p v-else-if="error" class="sres-state is-error">{{ error }}</p>

    <template v-else-if="hits.length">
      <button
        v-for="(h, i) in hits"
        :key="i"
        type="button"
        role="option"
        :aria-selected="false"
        class="sres-hit"
        @click="emit('pick', h)"
      >
        <span class="sres-where">
          <span class="sres-book">{{ h.bookName }}</span>
          <span v-if="h.kind === 'heading'" class="sres-tag">章</span>
          <span v-else-if="h.kind === 'book'" class="sres-tag">卷</span>
          <span v-else-if="h.para" class="sres-num num">§{{ h.para }}</span>
          <span v-else-if="h.segment" class="sres-num num">#{{ h.segment }}</span>
          <!-- A hit inside a translation is somebody else's sentence. Saying
               which language it was found in is the difference between "the
               canon says this" and "a translator says this". -->
          <span v-if="h.lang" class="sres-tag is-ref">{{ h.lang === 'zh' ? '中' : '英' }}</span>
        </span>
        <span v-if="h.kind === 'book' || h.kind === 'heading'" class="sres-name">
          <span class="sres-pali pi">{{ h.name }}</span>
          <span v-if="h.ref" class="sres-ref">{{ h.ref }}</span>
        </span>
        <span v-else class="sres-text pi" v-html="h.snippet" />
      </button>

      <button v-if="limited" type="button" class="sres-more" @click="emit('more')">
        结果不止这些 —— 在检索页查看全部
      </button>
    </template>

    <p v-else-if="query.length >= 2" class="sres-state">
      没有找到「{{ query }}」。<span v-if="scope" class="sres-scope">{{ scope }}</span>
    </p>
  </div>
</template>

<style scoped>
.sres {
  max-height: min(58vh, 460px);
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 4px;
}
.sres-state {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 12px;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--muted);
}
.sres-state.is-error {
  color: var(--danger);
}
.sres-scope {
  color: var(--meta);
}
.sres-hit {
  display: block;
  width: 100%;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  text-align: left;
}
.sres-hit:hover,
.sres-hit:focus-visible {
  background: var(--surface-warm);
}
.sres-where {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-bottom: 3px;
  font-size: 11px;
  color: var(--meta);
}
.sres-book {
  color: var(--accent);
  letter-spacing: 0.2px;
}
.sres-tag {
  padding: 0 4px;
  border: 1px solid var(--border-strong);
  border-radius: 3px;
  font-size: 10px;
  line-height: 14px;
}
.sres-tag.is-ref {
  border-color: var(--dict-split-rule);
  color: var(--accent);
}
.sres-num {
  font-size: 10.5px;
}
.sres-text {
  display: block;
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--fg-2);
}
/* The one place a mark is drawn in this list, in the same ink as the search
   page's, so a match looks the same wherever it is found. */
.sres-text :deep(mark) {
  padding: 0 2px;
  border-radius: 2px;
  background: var(--tag-soft);
  color: var(--accent);
}
.sres-name {
  display: block;
}
.sres-pali {
  font-size: 14px;
}
.sres-ref {
  display: block;
  margin-top: 1px;
  font-size: 12px;
  color: var(--muted);
}
.sres-more {
  display: block;
  width: 100%;
  padding: 9px 10px;
  border-top: 1px solid var(--border-soft);
  color: var(--accent);
  font-size: 12.5px;
  text-align: left;
}
.sres-more:hover {
  background: var(--surface-warm);
}
.spin {
  animation: spin 700ms linear infinite;
}
</style>
