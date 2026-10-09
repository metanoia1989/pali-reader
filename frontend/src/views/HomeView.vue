<script setup>
// The front page.
//
// The canon divides three ways, but not evenly: the Vinaya has 5 books, the
// Abhidhamma 13, and the Sutta piṭaka 43 spread over five nikāyas. Three equal
// columns therefore produced one column three times the height of the others,
// which reads as a broken page rather than a map.
//
// So the block is a bento that follows the content: Vinaya and Abhidhamma side
// by side on the first row, and the Sutta piṭaka across the full width beneath
// them with its five nikāyas as five columns of their own. Each list is capped
// so no column runs away; the count and a link carry the rest.
import { computed, onMounted, ref } from 'vue'
import {
  BookOpen,
  ChevronRight,
  Library,
  Loader2,
  ScrollText,
  Settings2,
} from 'lucide-vue-next'
import { api } from '../api'
import { useAuth } from '../store/auth'
import { useSettings } from '../store/settings'
import SettingsPanel from '../components/SettingsPanel.vue'
import BrandMark from '../components/BrandMark.vue'

const auth = useAuth()
const S = useSettings()

// How many books a column shows before deferring to the catalogue.
const PREVIEW = 6

const catalog = ref([])
const stats = ref(null)
const progress = ref([])
const loading = ref(true)
const expanded = ref({})

onMounted(async () => {
  try {
    const [c, s] = await Promise.all([api.catalog(), api.stats()])
    catalog.value = c.baskets || []
    stats.value = s
  } catch {
    /* the static parts of the page still render */
  }
  if (auth.signedIn) {
    try {
      const p = await api.progress()
      progress.value = (p.items || []).slice(0, 3)
      await S.loadFromAccount()
    } catch {
      /* no progress yet */
    }
  }
  loading.value = false
})

const fmt = (n) => (n == null ? '—' : Number(n).toLocaleString('en-US'))

const mula = computed(() => catalog.value.find((b) => b.code === 'mula') || null)
const commentaries = computed(() => catalog.value.filter((b) => b.code !== 'mula'))

const divisions = computed(() => mula.value?.categories || [])

const vinaya = computed(() => divisions.value.filter((d) => d.pitaka === 'vinaya'))
const sutta = computed(() => divisions.value.filter((d) => d.pitaka === 'sutta'))
const abhidhamma = computed(() => divisions.value.filter((d) => d.pitaka === 'abhidhamma'))

const count = (ds) => ds.reduce((n, d) => n + d.books.length, 0)

// The books a column shows, and how many it is holding back.
function shown(division) {
  const key = division.id
  const all = division.books
  if (expanded.value[key] || all.length <= PREVIEW) return all
  return all.slice(0, PREVIEW)
}
function hidden(division) {
  return Math.max(0, division.books.length - shown(division).length)
}

const others = computed(() =>
  commentaries.value.flatMap((b) =>
    b.categories.map((c) => ({ ...c, basket: b.code, basketName: b.name })),
  ),
)

function bookLink(b) {
  const hit = progress.value.find((p) => p.bookId === b.id)
  return hit && hit.segment > 1 ? `/read/${b.id}?seq=${hit.segment}` : `/read/${b.id}`
}

const name = (zh, pi) => S.primaryName(zh, pi)
const pali = (zh, pi) => S.secondaryName(zh, pi)
</script>

<template>
  <div class="shell">
    <header class="topbar">
      <router-link to="/" class="brand" style="color: inherit">
        <span class="mark"><BrandMark :size="17" /></span>
        <span class="nm">巴利三藏阅读器</span>
      </router-link>
      <div class="searchwrap" />
      <nav class="bar-actions">
        <router-link to="/catalog" class="btn btn-quiet btn-sm">全部典籍</router-link>
        <router-link to="/vocab" class="btn btn-quiet btn-sm">生词本</router-link>
        <button
          class="iconbtn"
          title="阅读设置"
          aria-label="阅读设置"
          @click="S.panelOpen = !S.panelOpen"
        >
          <Settings2 :size="18" />
        </button>
        <router-link v-if="!auth.signedIn" to="/login" class="btn btn-ghost btn-sm">登录</router-link>
        <router-link v-else to="/vocab" class="avatar" :title="auth.user?.displayName || ''">
          {{ auth.initial }}
        </router-link>
      </nav>
      <div v-if="S.panelOpen" class="set-anchor">
        <div class="set-scrim" @click="S.panelOpen = false" />
        <SettingsPanel class="set-panel" />
      </div>
    </header>

    <main class="page">
      <div class="page-inner">
        <section class="hero">
          <div>
            <p class="eyebrow" style="color: var(--accent)">Tipiṭaka · 三藏</p>
            <h1 class="h1">巴利三藏阅读器</h1>
            <p class="lede">
              点击经文里的任一单词即可查词、拆解复合词、展开变格表；选定释义与词性后写在原文之下，
              每一段都可以留下批注与翻译。为大量读原典而做。
            </p>
          </div>
          <div class="metrics">
            <div class="metric">
              <div class="mv num">{{ fmt(stats?.books) }}</div>
              <div class="ml">部典籍</div>
            </div>
            <div class="metric">
              <div class="mv num">{{ fmt(stats?.segments) }}</div>
              <div class="ml">个句段</div>
            </div>
            <div class="metric">
              <div class="mv num">{{ fmt(stats?.headwords) }}</div>
              <div class="ml">条词目</div>
            </div>
            <div class="metric">
              <div class="mv num">{{ fmt(stats?.lookupKeys) }}</div>
              <div class="ml">个可查词形</div>
            </div>
            <div class="metric">
              <div class="mv num">{{ fmt(stats?.translated) }}</div>
              <div class="ml">段参考译文</div>
            </div>
          </div>
        </section>

        <section v-if="progress.length" class="resume-wrap">
          <div class="sechead"><span class="eyebrow">继续阅读</span></div>
          <div class="resume">
            <router-link
              v-for="p in progress"
              :key="p.bookId"
              class="card resume-card"
              :to="`/read/${p.bookId}?seq=${p.segment}`"
            >
              <div style="flex: 1; min-width: 0">
                <div class="rtitle">{{ name(p.bookNameZh, p.bookName) }}</div>
                <div class="rsub num">§{{ p.para }} · 第 {{ p.segment }} 段 / 共 {{ p.total }} 段</div>
                <div class="rbar"><i :style="{ width: p.percent + '%' }" /></div>
              </div>
              <ChevronRight :size="18" style="color: var(--meta)" />
            </router-link>
          </div>
        </section>

        <section class="block">
          <div class="sechead">
            <span class="eyebrow">根本三藏 · 61 部</span>
            <router-link to="/catalog" class="ghostlink" style="margin: 0">
              全部典籍 <ChevronRight :size="14" />
            </router-link>
          </div>

          <div v-if="loading" class="loading"><Loader2 :size="20" class="spin" /></div>

          <div v-else class="bento">
            <!-- The Sutta piṭaka first and full width: it holds 43 of the 61
                 books, and it is what a reader is most often looking for. -->
            <article class="card pitaka wide">
              <header class="phead">
                <BookOpen :size="19" style="color: var(--accent)" />
                <h2 class="ptitle">经藏</h2>
                <span class="psub num">Sutta Piṭaka · {{ count(sutta) }} 部 · 五部尼柯耶</span>
              </header>
              <div class="nikayas">
                <div v-for="d in sutta" :key="d.id" class="nikaya">
                  <div class="nhead">
                    <span class="nlabel">{{ S.primaryName(d.nameZh, d.name) }}</span>
                    <span class="ncount num">{{ d.books.length }} 部</span>
                  </div>
                  <ul class="blist">
                    <li v-for="bk in shown(d)" :key="bk.id">
                      <router-link :to="bookLink(bk)" class="brow stacked">
                        <span class="bn">{{ name(bk.nameZh, bk.name) }}</span>
                        <span v-if="pali(bk.nameZh, bk.name)" class="bp pi">
                          {{ pali(bk.nameZh, bk.name) }}
                        </span>
                      </router-link>
                    </li>
                  </ul>
                  <button v-if="hidden(d)" class="morelink" @click="expanded[d.id] = true">
                    还有 {{ hidden(d) }} 部 ▾
                  </button>
                </div>
              </div>
            </article>

            <!-- Then the two piṭakas that are a single list of books each. -->
            <div class="pair">
              <article v-for="p in [{ key: 'vinaya', zh: '律藏', pi: 'Vinaya Piṭaka', icon: ScrollText, ds: vinaya },
                                     { key: 'abhidhamma', zh: '论藏', pi: 'Abhidhamma Piṭaka', icon: Library, ds: abhidhamma }]"
                       :key="p.key" class="card pitaka">
                <header class="phead">
                  <component :is="p.icon" :size="19" style="color: var(--accent)" />
                  <h2 class="ptitle">{{ p.zh }}</h2>
                  <span class="psub num">{{ p.pi }} · {{ count(p.ds) }} 部</span>
                </header>
                <div class="pbody">
                  <template v-for="d in p.ds" :key="d.id">
                    <ul class="blist">
                      <li v-for="bk in shown(d)" :key="bk.id">
                        <router-link :to="bookLink(bk)" class="brow">
                          <span class="bn">{{ name(bk.nameZh, bk.name) }}</span>
                          <span v-if="pali(bk.nameZh, bk.name)" class="bp pi">
                            {{ pali(bk.nameZh, bk.name) }}
                          </span>
                        </router-link>
                      </li>
                    </ul>
                    <button v-if="hidden(d)" class="morelink" @click="expanded[d.id] = true">
                      还有 {{ hidden(d) }} 部 ▾
                    </button>
                  </template>
                </div>
              </article>
            </div>
          </div>
        </section>

        <section class="block">
          <div class="sechead"><span class="eyebrow">注疏与藏外</span></div>
          <div class="others">
            <router-link
              v-for="c in others"
              :key="c.basket + c.id"
              class="card other"
              :to="`/catalog?basket=${c.basket}&division=${c.id}`"
            >
              <span class="oname">{{ S.primaryName(c.nameZh, c.name) }}</span>
              <span class="ocount num">{{ c.books.length }} 部 · {{ c.basketName }}</span>
            </router-link>
          </div>
        </section>

        <footer class="foot">
          <p>
            巴利原文为 Chaṭṭha Saṅgāyana（第六次结集）本；词典数据来自
            <a href="https://github.com/digitalpalidictionary/dpd-db" target="_blank" rel="noopener">DPD</a>
            与随附的巴漢／漢譯词典；参考译文来自 ePitaka 的中译与英译。
          </p>
          <p style="margin-top: 6px">各数据源版权归原作者所有，本项目仅供个人学习使用。</p>
        </footer>
      </div>
    </main>
  </div>
</template>

<style scoped>
.hero {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 48px;
  flex-wrap: wrap;
  padding: 24px 0 8px;
}
.h1 {
  margin-top: 14px;
  font-size: 46px;
  font-weight: 500;
  line-height: 1.12;
  letter-spacing: -0.8px;
}
.lede {
  max-width: 560px;
  margin-top: 16px;
  font-size: 15.5px;
  line-height: 1.75;
  color: var(--muted);
}
.metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 30px;
  padding-bottom: 6px;
}
.mv {
  font-size: 23px;
  font-weight: 500;
  color: var(--accent);
  line-height: 1.2;
}
.ml {
  margin-top: 2px;
  font-size: 11.5px;
  color: var(--meta);
}
.block {
  margin-top: 40px;
}
.sechead {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 14px;
}
.resume-wrap {
  margin-top: 34px;
}
.resume {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 10px;
}
.resume-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 15px 18px;
  color: inherit;
}
.resume-card:hover {
  text-decoration: none;
  background: var(--surface);
  box-shadow: var(--whisper);
}
.rtitle {
  font-size: 15px;
  font-weight: 500;
}
.rsub {
  margin-top: 3px;
  font-size: 11.5px;
  color: var(--meta);
}
.rbar {
  height: 3px;
  margin-top: 9px;
  border-radius: 999px;
  background: var(--border);
  overflow: hidden;
}
.rbar i {
  display: block;
  height: 100%;
  background: var(--accent);
}

/* The bento: two narrow piṭakas on one row, the wide one beneath. Widths
   follow the content rather than forcing three equal thirds. */
.bento {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
/* Widths follow the content. The Vinaya has 5 books and the Abhidhamma 13, so
   equal halves would leave one card half empty; `align-items: start` lets each
   hug its own height instead of stretching the shorter one. */
.pair {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.45fr);
  gap: 14px;
  align-items: start;
}
.pitaka {
  display: flex;
  flex-direction: column;
  padding: 20px 20px 16px;
}
.phead {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 10px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-soft);
}
.ptitle + .psub {
  margin-top: 0;
}
.ptitle {
  font-size: 19px;
  font-weight: 500;
  line-height: 1.25;
}
.psub {
  margin-top: 3px;
  font-size: 11.5px;
  color: var(--meta);
}
.pbody {
  flex: 1;
  margin-top: 10px;
}
.nikayas {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 0;
  margin-top: 12px;
}
/* A hairline between the nikāya columns, so five lists read as five and not
   as one wide field of names. */
.nikaya + .nikaya {
  padding-left: 20px;
  border-left: 1px solid var(--border-soft);
}
.nikaya:not(:last-child) {
  padding-right: 20px;
}
.nhead {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 7px;
  margin-bottom: 6px;
  border-bottom: 1px solid var(--border-soft);
}
.nlabel {
  font-size: 11.5px;
  font-weight: 500;
  letter-spacing: 0.8px;
  color: var(--accent);
}
.ncount {
  font-size: 10.5px;
  color: var(--meta);
}
.brow {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  padding: 5px 8px;
  border-radius: var(--radius-sm);
  color: inherit;
  font-size: 13.5px;
}
.brow:hover {
  text-decoration: none;
  background: var(--surface-warm);
  color: var(--accent);
}
/* The Chinese name takes the room it needs and the Pāḷi what is left; without
   min-width: 0 a flex item refuses to shrink and the two overlap. */
.bn {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bp {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 52%;
  font-family: var(--sans);
  font-size: 12.5px;
  color: var(--meta);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: right;
}
/* A nikāya column is a fifth of the page: two names side by side would both be
   truncated, so the Pāḷi sits under the Chinese where it has the full width. */
.brow.stacked {
  display: block;
  padding: 5px 8px 7px;
}
.brow.stacked .bn,
.brow.stacked .bp {
  display: block;
  max-width: none;
  text-align: left;
  overflow: visible;
  white-space: normal;
}
.brow.stacked .bp {
  margin-top: 1px;
}
.morelink {
  display: block;
  width: 100%;
  padding: 6px 8px;
  border-radius: var(--radius-sm);
  text-align: left;
  font-size: 12px;
  color: var(--accent);
}
.morelink:hover {
  background: var(--tag-faint);
}

.others {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
  gap: 8px;
}
.other {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 12px 14px;
  color: inherit;
}
.other:hover {
  text-decoration: none;
  background: var(--surface);
  box-shadow: var(--whisper);
}
.oname {
  font-size: 13.5px;
  font-weight: 500;
}
.ocount {
  font-size: 11px;
  color: var(--meta);
}

.loading {
  display: grid;
  place-items: center;
  padding: 60px;
  color: var(--meta);
}
.foot {
  margin-top: 60px;
  padding-top: 20px;
  border-top: 1px solid var(--border-soft);
  font-size: 11.5px;
  line-height: 1.8;
  color: var(--meta);
}
.set-anchor {
  position: absolute;
  top: 50px;
  right: 10px;
  z-index: 55;
}
.set-scrim {
  position: fixed;
  inset: 0;
  z-index: -1;
}
.set-panel {
  position: relative;
}
.spin {
  animation: spin 700ms linear infinite;
}
/* Once the lists wrap, a vertical rule no longer separates columns — it lands
   on the first item of the second row, which sits against the card's own edge
   and reads as a doubled border. Below the wrap there are no rules, only space;
   each list carries its own heading, so nothing is lost. */
@media (max-width: 1180px) {
  .nikayas {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    /* Space in BOTH directions. Dropping the vertical rules without putting a
       column gap in their place left two lists touching, which is what the
       rules had been doing. */
    gap: 18px 22px;
  }
  .nikaya + .nikaya {
    border-left: 0;
    padding-left: 0;
  }
  .nikaya:not(:last-child) {
    padding-right: 0;
  }
}
@media (max-width: 900px) {
  .pair {
    grid-template-columns: 1fr;
  }
  .nikayas {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 18px 20px;
  }
  .hero {
    gap: 24px;
  }
}
@media (max-width: 620px) {
  .nikayas {
    grid-template-columns: 1fr;
    gap: 18px 0;
  }
  .h1 {
    font-size: 31px;
  }
  .lede {
    font-size: 14.5px;
  }
  .metrics {
    gap: 20px;
  }
  .mv {
    font-size: 19px;
  }
}
</style>
