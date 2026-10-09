<script setup>
// The full catalogue.
//
// A filter rail on the left and a wide grid on the right. The books are grouped
// by piṭaka and then by division, so the Aṅguttara does not sit in the same
// undifferentiated list as the Dīgha — the first version of this page was a
// single flat column and read as a heap.
import { computed, onMounted, ref, watch } from 'vue'
import { ChevronRight, Loader2, Search, X } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { useSettings } from '../store/settings'
import TopBar from '../components/TopBar.vue'

const route = useRoute()
const router = useRouter()
const S = useSettings()

const catalog = ref([])
const loading = ref(true)
const q = ref('')
const basketFilter = ref(String(route.query.basket || ''))
const divisionFilter = ref(String(route.query.division || ''))

// The Sutta piṭaka first — it holds 43 of the 61 books, and it is what someone
// opening the catalogue has usually come for. Vinaya and Abhidhamma follow.
// This is the same order the API returns its divisions in; it is repeated here
// only because the page groups by piṭaka and has to order those groups itself.
const PITAKA_ORDER = ['sutta', 'vinaya', 'abhidhamma', 'other']
const PITAKA_ZH = { vinaya: '律藏', sutta: '经藏', abhidhamma: '论藏', other: '藏外' }

onMounted(async () => {
  try {
    const c = await api.catalog()
    catalog.value = c.baskets || []
  } finally {
    loading.value = false
  }
})

watch([basketFilter, divisionFilter], () => {
  router.replace({
    query: {
      ...(basketFilter.value ? { basket: basketFilter.value } : {}),
      ...(divisionFilter.value ? { division: divisionFilter.value } : {}),
    },
  })
})

// Flatten to divisions, which is the unit the page renders.
const divisions = computed(() => {
  const out = []
  for (const b of catalog.value) {
    for (const c of b.categories) {
      out.push({ ...c, basket: b.code, basketName: b.name })
    }
  }
  return out
})

const basketOptions = computed(() => catalog.value.map((b) => ({ code: b.code, name: b.name })))

const filtered = computed(() => {
  const needle = q.value.trim().toLowerCase()
  return divisions.value
    .filter((d) => !basketFilter.value || d.basket === basketFilter.value)
    .filter((d) => !divisionFilter.value || d.id === divisionFilter.value)
    .map((d) => ({
      ...d,
      books: needle
        ? d.books.filter(
            (bk) =>
              (bk.nameZh || '').toLowerCase().includes(needle) ||
              bk.name.toLowerCase().includes(needle) ||
              bk.id.includes(needle),
          )
        : d.books,
    }))
    .filter((d) => d.books.length)
})

// Group the filtered divisions by piṭaka so the headings survive filtering.
const grouped = computed(() => {
  const byPitaka = new Map()
  for (const d of filtered.value) {
    const key = `${d.basket}/${d.pitaka || 'other'}`
    if (!byPitaka.has(key)) {
      byPitaka.set(key, {
        key,
        zh: PITAKA_ZH[d.pitaka] || '其他',
        basketName: d.basketName,
        divisions: [],
        books: 0,
      })
    }
    const g = byPitaka.get(key)
    g.divisions.push(d)
    g.books += d.books.length
  }
  return [...byPitaka.values()].sort((a, b) => {
    const ai = PITAKA_ORDER.indexOf(a.key.split('/')[1])
    const bi = PITAKA_ORDER.indexOf(b.key.split('/')[1])
    return ai - bi
  })
})

const total = computed(() => filtered.value.reduce((n, d) => n + d.books.length, 0))
const filtering = computed(() => !!(q.value || basketFilter.value || divisionFilter.value))

function clearAll() {
  q.value = ''
  basketFilter.value = ''
  divisionFilter.value = ''
}

// Inline handlers with two statements need explicit separators; naming them is
// clearer than packing them into one attribute.
function pickBasket(code) {
  basketFilter.value = code
  divisionFilter.value = ''
}
</script>

<template>
  <div class="shell">
    <TopBar :crumbs="[{ label: '全部典籍' }]" />
    <main class="page">
      <div class="page-inner">
        <div class="cat-head">
          <div>
            <p class="eyebrow" style="color: var(--accent)">Tipiṭaka</p>
            <h1 class="cat-title">全部典籍</h1>
          </div>
          <p class="cat-count num">{{ total }} 部</p>
        </div>

        <div class="cat-body">
          <aside class="filters">
            <div class="search" style="background: var(--surface); margin-bottom: 16px">
              <Search :size="15" />
              <input v-model="q" type="search" placeholder="按书名筛选…" />
            </div>

            <div class="fgroup">
              <span class="flabel">三藏</span>
              <button class="fitem" :aria-pressed="!basketFilter" @click="pickBasket('')">
                全部
              </button>
              <button
                v-for="b in basketOptions"
                :key="b.code"
                class="fitem"
                :aria-pressed="basketFilter === b.code"
                @click="pickBasket(b.code)"
              >
                {{ b.name }}
              </button>
            </div>

            <div v-if="basketFilter" class="fgroup">
              <span class="flabel">部 / 尼柯耶</span>
              <button class="fitem" :aria-pressed="!divisionFilter" @click="divisionFilter = ''">
                全部
              </button>
              <button
                v-for="d in divisions.filter((x) => x.basket === basketFilter)"
                :key="d.id"
                class="fitem"
                :aria-pressed="divisionFilter === d.id"
                @click="divisionFilter = d.id"
              >
                {{ S.primaryName(d.nameZh, d.name) }}
                <span class="fcount num">{{ d.books.length }}</span>
              </button>
            </div>

            <button v-if="filtering" class="ghostlink" style="margin: 14px 0 0" @click="clearAll">
              <X :size="13" />清除筛选
            </button>
          </aside>

          <div class="results">
            <div v-if="loading" class="loading"><Loader2 :size="20" class="spin" /></div>

            <template v-else>
              <section v-for="g in grouped" :key="g.key" class="pgroup">
                <div class="ghead">
                  <h2 class="gtitle">{{ g.zh }}</h2>
                  <span class="gsub num">{{ g.basketName }} · {{ g.books }} 部</span>
                </div>
                <div v-for="d in g.divisions" :key="d.id" class="division">
                  <div class="dhead">
                    <span class="dlabel">{{ S.primaryName(d.nameZh, d.name) }}</span>
                    <span class="dpali pi">{{ d.name }}</span>
                    <span class="dcount num">{{ d.books.length }} 部</span>
                  </div>
                  <div class="bookgrid">
                    <router-link
                      v-for="bk in d.books"
                      :key="bk.id"
                      :to="`/read/${bk.id}`"
                      class="card bookcard"
                    >
                      <span class="bcz">{{ S.primaryName(bk.nameZh, bk.name) }}</span>
                      <span v-if="S.secondaryName(bk.nameZh, bk.name)" class="bcp pi">
                        {{ S.secondaryName(bk.nameZh, bk.name) }}
                      </span>
                      <span class="bcm num">{{ bk.segCount }} 段 · {{ bk.pageCount }} 页</span>
                      <ChevronRight :size="15" class="bcx" />
                    </router-link>
                  </div>
                </div>
              </section>
              <p v-if="!grouped.length" class="empty">没有匹配的书名。</p>
            </template>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<style scoped>
.cat-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  padding-bottom: 22px;
}
.cat-title {
  margin-top: 8px;
  font-size: 32px;
  font-weight: 500;
}
.cat-count {
  font-size: 13px;
  color: var(--meta);
}
.cat-body {
  display: grid;
  grid-template-columns: 208px minmax(0, 1fr);
  gap: 32px;
  align-items: start;
}
.filters {
  position: sticky;
  top: calc(var(--topbar-h) + 20px);
}
.fgroup {
  margin-bottom: 18px;
}
.flabel {
  display: block;
  margin-bottom: 6px;
  font-size: 10.5px;
  font-weight: 500;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--meta);
}
.fitem {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  padding: 5px 9px;
  border-radius: var(--radius-sm);
  text-align: left;
  font-size: 13px;
  color: var(--fg-2);
  transition: background var(--fast), color var(--fast);
}
.fitem:hover {
  background: var(--surface-warm);
  color: var(--fg);
}
.fitem[aria-pressed='true'] {
  background: var(--tag-soft);
  color: var(--accent);
  font-weight: 500;
}
.fcount {
  font-size: 10.5px;
  color: var(--meta);
}
.fitem[aria-pressed='true'] .fcount {
  color: var(--accent);
  opacity: 0.75;
}
.pgroup {
  margin-bottom: 34px;
}
.ghead {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding-bottom: 10px;
  margin-bottom: 14px;
  border-bottom: 1px solid var(--border);
}
.gtitle {
  font-size: 20px;
  font-weight: 500;
}
.gsub {
  font-size: 11.5px;
  color: var(--meta);
}
.division + .division {
  margin-top: 20px;
}
.dhead {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 8px;
}
.dlabel {
  font-size: 13px;
  font-weight: 500;
  color: var(--fg-2);
}
.dpali {
  flex: 1;
  min-width: 0;
  font-size: 11.5px;
  color: var(--meta);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dcount {
  flex: none;
  font-size: 10.5px;
  color: var(--meta);
}
.bookgrid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(224px, 1fr));
  gap: 8px;
}
.bookcard {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 12px 32px 12px 14px;
  color: inherit;
}
.bookcard:hover {
  text-decoration: none;
  background: var(--surface);
  box-shadow: var(--whisper);
}
.bcz {
  font-size: 13.5px;
  font-weight: 500;
  line-height: 1.35;
  overflow-wrap: anywhere;
}
.bcp {
  font-family: var(--sans);
  font-size: 12.5px;
  color: var(--meta);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bcm {
  margin-top: 3px;
  font-size: 11px;
  color: var(--meta);
}
.bcx {
  position: absolute;
  right: 11px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--border-strong);
}
.loading {
  display: grid;
  place-items: center;
  padding: 60px;
  color: var(--meta);
}
.spin {
  animation: spin 700ms linear infinite;
}
@media (max-width: 900px) {
  .cat-body {
    grid-template-columns: 1fr;
    gap: 18px;
  }
  .filters {
    position: static;
  }
}
</style>
