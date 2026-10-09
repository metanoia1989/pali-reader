<script setup>
// Corpus search.
//
// Two modes, because the reader has two different questions. "经文" searches
// the text itself and returns the passages. "词条" searches the dictionary and
// returns entries — the right mode when they met a form in a book and want to
// know what it is without leaving the keyboard.
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { BookOpen, Loader2, Search } from 'lucide-vue-next'
import { api } from '../api'
import TopBar from '../components/TopBar.vue'

const route = useRoute()
const router = useRouter()

const q = ref(String(route.query.q || ''))
const mode = ref('text')
const hits = ref([])
const words = ref([])
const loading = ref(false)
const searched = ref(false)

async function run() {
  const needle = q.value.trim()
  if (needle.length < 2) return
  loading.value = true
  searched.value = true
  try {
    if (mode.value === 'text') {
      const r = await api.search(needle, { limit: 80 })
      hits.value = r.hits || []
    } else {
      const r = await api.suggest(needle, 40)
      words.value = r.items || []
    }
  } catch {
    hits.value = []
    words.value = []
  } finally {
    loading.value = false
  }
}

onMounted(run)
watch(() => route.query.q, (v) => {
  if (v && v !== q.value) {
    q.value = String(v)
    run()
  }
})
watch(mode, run)

function open(hit) {
  router.push(`/read/${hit.bookId}?seq=${hit.segment}`)
}
</script>

<template>
  <div class="shell">
    <TopBar :crumbs="[{ label: '搜索' }]" />
    <main class="page">
      <div class="page-inner" style="max-width: 900px">
        <p class="eyebrow" style="color: var(--accent)">Search</p>
        <h1 style="margin-top: 8px; font-size: 30px; font-weight: 500">全文检索</h1>

        <form class="searchbox" @submit.prevent="run">
          <div class="search" style="background: var(--surface); height: 42px">
            <Search :size="16" />
            <input v-model="q" type="search" placeholder="输入巴利语词形或短语…" autofocus />
          </div>
          <button class="btn btn-primary" type="submit">搜索</button>
        </form>

        <div class="modes">
          <button class="tab" :aria-selected="mode === 'text'" @click="mode = 'text'">经文</button>
          <button class="tab" :aria-selected="mode === 'word'" @click="mode = 'word'">词条</button>
        </div>

        <div v-if="loading" style="display: grid; place-items: center; padding: 50px">
          <Loader2 :size="20" class="spin" style="color: var(--meta)" />
        </div>

        <template v-else-if="mode === 'text'">
          <p v-if="searched" class="secmeta num" style="margin: 12px 0">
            找到 {{ hits.length }} 段
          </p>
          <div class="hits">
            <button v-for="(h, i) in hits" :key="i" class="card hit" @click="open(h)">
              <div class="hh">
                <span class="hb">{{ h.bookName }}</span>
                <span class="hn num">§{{ h.para || h.segment }}</span>
              </div>
              <p class="hs pi" v-html="h.snippet" />
            </button>
          </div>
          <p v-if="searched && !hits.length" class="empty">
            没有找到。可以试试更短的词干（例如用 dhamm 而不是 dhamma），
            或者改用「词条」模式查词典。
          </p>
        </template>

        <template v-else>
          <div class="words">
            <button
              v-for="w in words"
              :key="w.id"
              class="card word"
              @click="router.push(`/search?q=${encodeURIComponent(w.lemma)}`)"
            >
              <div class="wh">
                <span class="pi wl">{{ w.lemma }}</span>
                <span v-if="w.homonym" class="tag plain tiny">{{ w.homonym }}</span>
                <span v-if="w.pos" class="tag tiny">{{ w.pos }}</span>
              </div>
              <p class="wm">{{ w.meaning1 || w.meaningLit }}</p>
            </button>
          </div>
          <p v-if="searched && !words.length" class="empty">词典里没有以这几个字母开头的词条。</p>
        </template>
      </div>
    </main>
  </div>
</template>

<style scoped>
.searchbox {
  display: flex;
  gap: 8px;
  margin: 20px 0 14px;
}
.searchbox .search {
  flex: 1;
}
.modes {
  display: flex;
  gap: 16px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 12px;
}
.tab {
  padding: 8px 1px;
  margin-bottom: -1px;
  border-bottom: 2px solid transparent;
  color: var(--muted);
  font-size: 13.5px;
  font-weight: 500;
}
.tab[aria-selected='true'] {
  color: var(--accent);
  border-bottom-color: var(--accent);
}
.hits,
.words {
  display: grid;
  gap: 8px;
}
.hit,
.word {
  padding: 12px 14px;
  text-align: left;
  transition: box-shadow var(--base);
}
.hit:hover,
.word:hover {
  box-shadow: var(--whisper);
}
.hh {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 5px;
}
.hb {
  font-size: 11.5px;
  letter-spacing: 0.3px;
  color: var(--accent);
}
.hn {
  font-size: 11px;
  color: var(--meta);
}
.hs {
  font-size: 13.5px;
  line-height: 1.65;
  color: var(--fg-2);
}
.hs :deep(mark) {
  background: var(--tag-soft);
  color: var(--accent);
  border-radius: 2px;
  padding: 0 2px;
}
.wh {
  display: flex;
  align-items: center;
  gap: 6px;
}
.wl {
  font-size: 14.5px;
  font-weight: 500;
}
.wm {
  margin-top: 3px;
  font-size: 12.5px;
  color: var(--muted);
}
.spin {
  animation: spin 700ms linear infinite;
}
</style>
