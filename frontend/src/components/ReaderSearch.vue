<script setup>
// Search inside the book being read.
//
// The composer. It owns the query, the mode, the request and the jump, and it
// arranges three components that know nothing about each other: the field, the
// mode switch, and the result list. Anything the interaction design wants to
// move can be moved here without touching what the pieces do.
//
// Where each mode looks, and why:
//
//   本章      the chapter the contents has highlighted. Asked of the server as
//             a range of segment numbers, not searched in the DOM: the window
//             in the DOM is bounded to a few hundred segments, so a local
//             search would answer "not found" to a passage two hundred segments
//             above the reader — a search that silently returns nothing is
//             worse than one that says what it covers.
//   经文名    volumes and the suttas/chapters inside them, in Pāḷi and in their
//             published translations. Inline: a name is a destination, the
//             list is short, and clicking one replaces the reading surface.
//   巴利全文  the corpus. Inline, capped, and the cap says so and hands off to
//             the search page — a corpus-wide query can return hundreds of
//             hits, and a dropdown that long is not a list anyone reads. Hits
//             from the book being read are shown first, because that is where
//             the reader is.
//   译文      the published translations of THIS book, both languages. Book
//             scoped rather than corpus-wide: see the note on handleRefSearch —
//             2.2 million rows, a full scan at ~1.4s, and a FULLTEXT index we
//             have not added. It is also the honest scope for a search the
//             reader opens while reading one book, and it does not depend on
//             参考译文 being set to 全部展开, which is what a client-side
//             implementation would have been limited by.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { useReader } from '../store/reader'
import { chapterRange, modeById, rankHits, SEARCH_MODES, MIN_QUERY } from '../utils/search'
import { registerSearchField } from '../utils/searchFocus'
import { tocEntries } from '../utils/numbering'
import SearchField from './SearchField.vue'
import SearchModeSwitch from './SearchModeSwitch.vue'
import SearchResults from './SearchResults.vue'

const router = useRouter()
const R = useReader()
// A hit inside the book being read is the view's to act on: the jump is also a
// history entry, and the address bar belongs to the view.
const emit = defineEmits(['goto'])

const mode = ref('chapter')
const q = ref('')
const hits = ref([])
const limited = ref(false)
const loading = ref(false)
const error = ref('')
const open = ref(false)

// Which request the visible list belongs to. Typing runs a query per pause, and
// two requests can come back in the wrong order; without this the list can end
// up showing the answer to a query the reader has already replaced.
let asked = 0

const toc = computed(() => tocEntries(R.toc))
const chapter = computed(() => chapterRange(toc.value, R.activeSegment))

// What the empty state says about the scope of the mode that found nothing.
// A mode with a narrower reach than the reader assumed has to be the one to say
// so — "没有找到" on its own reads as "the canon does not say this".
const scope = computed(() => {
  switch (mode.value) {
    case 'chapter':
      return chapter.value.name
        ? `只搜索目录里选中的「${chapter.value.name}」这一章，不是整本书。`
        : '只搜索目录里选中的这一章，不是整本书。'
    case 'title':
      return '按卷名、经名与章名查找；正文请用另外两种模式。'
    case 'pali':
      return '搜索整部三藏的巴利原文，不搜索译文。'
    default:
      return '只搜索本书的中英参考译文；不搜索巴利原文，也不搜索其他书。'
  }
})

async function run() {
  const needle = q.value.trim()
  if (needle.length < MIN_QUERY) {
    hits.value = []
    limited.value = false
    error.value = ''
    loading.value = false
    return
  }
  const mine = ++asked
  loading.value = true
  open.value = true
  try {
    let r
    if (mode.value === 'chapter') {
      const ch = chapter.value
      r = await api.searchBook(R.bookId, needle, { from: ch.from, to: ch.to, limit: 30 })
    } else if (mode.value === 'title') {
      r = await api.searchTitles(needle, { limit: 20 })
    } else if (mode.value === 'refs') {
      r = await api.searchRefs(R.bookId, needle, { limit: 30 })
    } else {
      r = await api.search(needle, { limit: 30 })
    }
    if (mine !== asked) return
    const list = r.hits || []
    // The corpus-wide modes answer in the canon's own order — book by book.
    // The reader is inside one book, so a hit in it is worth more than one in a
    // volume they have never opened; the order inside each group is left alone,
    // because that is reading order and re-ranking it would be a guess.
    hits.value =
      mode.value === 'chapter' || mode.value === 'refs' ? list : rankHits(list, R.bookId)
    limited.value = !!r.limited
    error.value = ''
  } catch (e) {
    if (mine !== asked) return
    hits.value = []
    limited.value = false
    error.value = e.message || '搜索失败'
  } finally {
    if (mine === asked) loading.value = false
  }
}

let timer = null
watch([q, mode], () => {
  clearTimeout(timer)
  // A pause, not every keystroke: the corpus search is a LIKE over a mediumtext
  // column, and a request per letter is a request per letter.
  timer = setTimeout(run, 220)
})
onBeforeUnmount(() => clearTimeout(timer))

function pick(h) {
  open.value = false
  if (!h) return
  // Another book: the reading page is the destination, and the segment travels
  // in the query string so a reload lands in the same place.
  if (h.bookId && h.bookId !== R.bookId) {
    router.push({ name: 'read', params: { bookId: h.bookId }, query: { seq: String(h.segment || 1) } })
    return
  }
  // This book: the view takes the jump, because a jump the reader asked for is
  // also a history entry and the address bar belongs to the view.
  if (h.segment) emit('goto', h.segment)
}

function more() {
  open.value = false
  router.push({ name: 'search', query: { q: q.value.trim() } })
}

function close() {
  open.value = false
}

// Escape from anywhere, and a click outside the panel: the reader is in the
// middle of a page and should not have to find the right control to get it back.
function onDocPointer(e) {
  if (!open.value) return
  const root = e.target?.closest?.('.rsearch')
  if (!root) open.value = false
}
onMounted(() => {
  registerSearchField(() => field.value?.focus())
  document.addEventListener('pointerdown', onDocPointer, true)
})
onBeforeUnmount(() => {
  registerSearchField(null)
  document.removeEventListener('pointerdown', onDocPointer, true)
})

const field = ref(null)
defineExpose({ focus: () => field.value?.focus() })</script>

<template>
  <div class="rsearch">
    <div class="rsearch-row" :class="{ 'is-open': open }">
      <SearchField
        ref="field"
        v-model="q"
        :placeholder="'搜索' + modeById(mode).label + '…'"
        :busy="loading"
        @submit="run"
        @escape="close"
        @focus="q.trim().length >= MIN_QUERY && (open = true)"
      />
      <SearchModeSwitch v-model="mode" :modes="SEARCH_MODES" />
    </div>

    <div v-if="open" class="rsearch-panel">
      <SearchResults
        :hits="hits"
        :mode="mode"
        :loading="loading"
        :query="q.trim()"
        :error="error"
        :limited="limited"
        :scope="scope"
        @pick="pick"
        @more="more"
      />
      <p v-if="!loading && !error && q.trim().length < MIN_QUERY" class="rsearch-hint">
        输入至少两个字开始搜索。<span class="rsearch-scope">{{ scope }}</span>
      </p>
    </div>
  </div>
</template>

<style scoped>
/* The row is the box; the field inside it draws no border of its own. That is
   what lets the mode switch sit inside the same outline — one control with a
   range selector, rather than a field with a button beside it. */
.rsearch {
  position: relative;
  width: 100%;
  max-width: 460px;
}
.rsearch-row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 36px;
  padding: 0 6px 0 11px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  transition: border-color var(--fast);
}
.rsearch-row:focus-within,
.rsearch-row.is-open {
  border-color: var(--accent);
}
.rsearch-panel {
  position: absolute;
  top: 42px;
  left: 0;
  right: 0;
  z-index: 70;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--surface);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.1);
}
.rsearch-hint {
  padding: 2px 12px 12px;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--muted);
}
.rsearch-scope {
  color: var(--meta);
}
/* On a phone the bar is narrow and a panel hanging off the field would be
   narrower than the field's own text. Full width under the bar instead. */
@media (max-width: 700px) {
  .rsearch-panel {
    position: fixed;
    top: 54px;
    left: 8px;
    right: 8px;
  }
}
</style>
