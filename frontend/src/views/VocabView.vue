<script setup>
// The vocabulary list. Its useful axis is rarity, not recency: the words a
// reader has kept that occur least often in the canon are the ones worth
// drilling, so the list is sorted by corpus frequency by default.
import { computed, onMounted, ref } from 'vue'
import { ArrowUpDown, Loader2, Search, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import { useAuth } from '../store/auth'
import TopBar from '../components/TopBar.vue'

const auth = useAuth()
const items = ref([])
const loading = ref(true)
const q = ref('')
const sort = ref('rare')

async function load() {
  loading.value = true
  try {
    const r = await api.vocab(q.value.trim())
    items.value = r.items || []
  } finally {
    loading.value = false
  }
}
onMounted(load)

const sorted = computed(() => {
  const list = [...items.value]
  if (sort.value === 'rare') list.sort((a, b) => (a.count || 0) - (b.count || 0))
  else if (sort.value === 'common') list.sort((a, b) => (b.count || 0) - (a.count || 0))
  else list.sort((a, b) => (b.addedAt || '').localeCompare(a.addedAt || ''))
  return list
})

async function remove(it) {
  await api.removeVocab(it.lemma)
  items.value = items.value.filter((x) => x.lemma !== it.lemma)
}
</script>

<template>
  <div class="shell">
    <TopBar :crumbs="[{ label: '生词本' }]" />
    <main class="page">
      <div class="page-inner" style="max-width: 980px">
        <p class="eyebrow" style="color: var(--accent)">Vocabulary</p>
        <h1 style="margin-top: 8px; font-size: 30px; font-weight: 500">生词本</h1>
        <p class="lede">
          在查词面板里点「生词本」即可收词。语料频次越低，说明这个词越少见，通常也越值得先记。
        </p>

        <div v-if="!auth.signedIn" class="empty">
          生词本需要登录。<router-link to="/login">登录</router-link> 或
          <router-link to="/register">注册</router-link>。
        </div>

        <template v-else>
          <div class="bar">
            <form class="search" style="background: var(--surface); max-width: 300px" @submit.prevent="load">
              <Search :size="15" />
              <input v-model="q" type="search" placeholder="搜索生词…" />
            </form>
            <button
              class="btn btn-secondary btn-sm"
              @click="sort = sort === 'rare' ? 'common' : sort === 'common' ? 'new' : 'rare'"
            >
              <ArrowUpDown :size="14" />
              {{ { rare: '最罕见优先', common: '最常见优先', new: '最近加入' }[sort] }}
            </button>
            <span class="secmeta num" style="margin-left: auto">{{ sorted.length }} 条</span>
          </div>

          <div v-if="loading" style="display: grid; place-items: center; padding: 60px">
            <Loader2 :size="20" class="spin" style="color: var(--meta)" />
          </div>

          <div v-else-if="!sorted.length" class="empty">还没有生词。</div>

          <div v-else class="list">
            <div v-for="it in sorted" :key="it.lemma" class="card vrow">
              <div style="flex: 1; min-width: 0">
                <div class="vhead">
                  <span class="vw pi">{{ it.lemma }}</span>
                  <span v-if="it.pos" class="tag plain tiny">{{ it.pos }}</span>
                  <span class="tag tiny num" :title="'全语料出现 ' + (it.count || 0) + ' 次'">
                    {{ it.count || 0 }} 次
                  </span>
                </div>
                <p v-if="it.meaning" class="vm">{{ it.meaning }}</p>
                <p v-if="it.note" class="vn">{{ it.note }}</p>
              </div>
              <button class="iconbtn" title="移除" @click="remove(it)"><Trash2 :size="15" /></button>
            </div>
          </div>
        </template>
      </div>
    </main>
  </div>
</template>

<style scoped>
.lede {
  max-width: 620px;
  margin-top: 10px;
  font-size: 14px;
  line-height: 1.7;
  color: var(--muted);
}
.bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 22px 0 16px;
}
.list {
  display: grid;
  gap: 8px;
}
.vrow {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
}
.vhead {
  display: flex;
  align-items: center;
  gap: 7px;
  flex-wrap: wrap;
}
.vw {
  font-size: 15px;
  font-weight: 500;
}
.vm {
  margin-top: 4px;
  font-size: 13px;
  color: var(--muted);
  line-height: 1.6;
}
.vn {
  margin-top: 3px;
  font-size: 12px;
  color: var(--meta);
}
.spin {
  animation: spin 700ms linear infinite;
}
</style>
