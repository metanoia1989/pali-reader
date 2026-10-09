<script setup>
// 查词面板 — the reason this application exists.
//
// The information order is deliberate and comes from a panel the reader already
// used and liked: it answers "what could this word be?" before "what does this
// entry mean?".
//
//   词形分析   every possible reading of this form, as a table
//   拆解       if it is a compound, what it is made of
//   词条       one collapsible block per headword, each with its own declension
//
// Every row that states something about the word is takeable. Tapping a reading
// records it against the word in the text; tapping it again strikes it out. A
// Pāḷi form is ambiguous, so several can be held at once — that is the whole
// point, and it is why this panel has no single "apply" button.
//
// Links inside the panel — a part of a deconstruction, a root, a form in a
// declension — do not replace the panel. On a wide screen they open beside it,
// so the sentence and the grammar stay in view together.
import { computed, ref, watch } from 'vue'
import {
  BookmarkPlus,
  Check,
  ChevronLeft,
  ChevronRight,
  Copy,
  Loader2,
  Plus,
  Search,
  X,
} from 'lucide-vue-next'
import { api } from '../api'
import { useSettings } from '../store/settings'
import { grammarPayload, heldGrammar, heldMeaning, meaningPayload, sensesOf } from '../utils/picks'
import DeclensionTable from './DeclensionTable.vue'
import RichText from './RichText.vue'

const props = defineProps({
  // Empty when the reader has not tapped a word yet. The panel still opens —
  // the search field is the way in.
  word: { type: String, default: '' },
  segment: { type: Number, default: 0 },
  wordIndex: { type: Number, default: -1 },
  bookId: { type: String, default: '' },
  // The picks already recorded against this occurrence, so a row the reader has
  // taken can say so.
  picks: { type: Array, default: () => [] },
  signedIn: { type: Boolean, default: false },
  // A popup is the same panel with less room: no footer, no history.
  compact: { type: Boolean, default: false },
})
const emit = defineEmits(['add', 'remove', 'link', 'search', 'close', 'step', 'need-auth'])

const S = useSettings()

const res = ref(null)
const loading = ref(false)
const failed = ref('')
const copied = ref(false)
const savedToVocab = ref(false)
const showAllAnalyses = ref(false)
// Closed to begin with: the reader tapped one word to see what it is, and
// opening a whole entry for them buries the analysis table they actually asked
// for.
const expanded = ref({})
const picked = ref({})

const ANALYSIS_PREVIEW = 6

async function load(word) {
  if (!word) return
  loading.value = true
  failed.value = ''
  try {
    const r = await api.lookup(word)
    res.value = r
    showAllAnalyses.value = false
    expanded.value = {}
    picked.value = Object.fromEntries((r.headwords || []).map((_, i) => [i, 'dpd']))
  } catch (e) {
    failed.value = e.message || '查询失败'
    res.value = null
  } finally {
    loading.value = false
  }
}

watch(() => props.word, load, { immediate: true })
const hasWord = computed(() => !!props.word)

// --- searching from the panel --------------------------------------------
// A reader who wants a different word should not have to go back to the text,
// find it and tap it. Whatever is typed here goes through the same path as a
// link — it lands in the panel and in the history, so ‹ › walks it like any
// other word looked up.
const query = ref('')

watch(
  () => props.word,
  (w) => {
    query.value = ''
    void w
  },
)

function submitSearch() {
  const q = query.value.trim()
  if (!q) return
  emit('search', q)
}

// --- which dictionaries are offered ---------------------------------------
// 词典 setting: 全部 keeps every dictionary and leads with DPD, DPD alone,
// 仅中 and 仅英 keep one language. DPD is a choice in its own right because it
// is the most complete of them.
const sources = computed(() => {
  const others = []
  const seen = new Set()
  for (const m of res.value?.meanings || []) {
    if (m.source.startsWith('dpd')) continue
    if (seen.has(m.source)) continue
    seen.add(m.source)
    others.push(m)
  }
  return others.filter((m) => S.allowsLang(m.lang))
})

const head = computed(() => res.value?.headwords?.[0] || null)
const analyses = computed(() => res.value?.analyses || [])
const splits = computed(() => res.value?.splits || [])
const entries = computed(() => res.value?.headwords || [])

const shownAnalyses = computed(() =>
  showAllAnalyses.value ? analyses.value : analyses.value.slice(0, ANALYSIS_PREVIEW),
)

// The dictionary a given entry is currently showing: DPD by default, or
// whichever of the others the reader switched to.
function sourceFor(i) {
  const s = picked.value[i] || 'dpd'
  if (s === 'dpd') return { key: 'dpd', name: 'DPD', text: entries.value[i]?.meaning1 || '', sub: entries.value[i]?.meaningLit || '' }
  const m = sources.value.find((x) => x.source === s)
  return { key: s, name: m?.name || s, text: m?.text || '', sub: '' }
}

// The gloss lines of one entry, each independently takeable. DPD joins several
// senses into one string with ";" and the reader needs them apart: recording
// "teaching" should not also record "law".
function sensesOfEntry(i) {
  return sensesOf(sourceFor(i).text)
}

const posChips = computed(() => {
  const out = []
  const h = head.value
  if (h?.pos) out.push(h.pos)
  if (h?.compoundConstruction) out.push('复合词')
  if (h?.compoundType) out.push(h.compoundType)
  if (!out.length && analyses.value.length) out.push(analyses.value[0].pos)
  return out.slice(0, 4)
})

// --- taking a row, striking it out ----------------------------------------
const at = () => ({ segment: props.segment, wordIndex: props.wordIndex, word: props.word })

function isGrammarHeld(a) {
  return !!heldGrammar(a, props.picks)
}

function isMeaningHeld(sense, name) {
  return !!heldMeaning(sense, name, props.picks)
}

function toggleGrammar(a) {
  if (!props.signedIn) return emit('need-auth')
  const held = heldGrammar(a, props.picks)
  if (held) return emit('remove', { segment: props.segment, wordIndex: props.wordIndex, key: held.key })
  emit('add', grammarPayload(a, { ...at(), lemma: head.value?.lemma, lemmaId: head.value?.id }))
}

function toggleMeaning(sense, i) {
  if (!props.signedIn) return emit('need-auth')
  const src = sourceFor(i)
  const held = heldMeaning(sense, src.name, props.picks)
  if (held) return emit('remove', { segment: props.segment, wordIndex: props.wordIndex, key: held.key })
  emit('add', meaningPayload(sense, {
    ...at(),
    lemma: entries.value[i]?.lemma || head.value?.lemma,
    lemmaId: entries.value[i]?.id || 0,
    sourceKey: src.key,
    sourceName: src.name,
  }))
}

// A link inside the panel. The parent decides whether that replaces the panel
// or opens beside it; on a wide screen it opens beside it.
function follow(word, event) {
  emit('link', { word, el: event?.currentTarget || null })
}

function toggle(i) {
  expanded.value = { ...expanded.value, [i]: !expanded.value[i] }
}

async function copyWord() {
  const t = head.value?.lemma || props.word
  try {
    await navigator.clipboard.writeText(t)
    copied.value = true
    setTimeout(() => (copied.value = false), 1400)
  } catch {
    /* clipboard is unavailable over plain http on some browsers */
  }
}

async function addVocab() {
  if (!props.signedIn) return emit('need-auth')
  const h = head.value
  try {
    await api.addVocab({
      lemma: h?.lemma || res.value?.key || props.word,
      lemmaId: h?.id || 0,
      pos: h?.pos || '',
      meaning: h?.meaning1 || '',
    })
    savedToVocab.value = true
    setTimeout(() => (savedToVocab.value = false), 1600)
  } catch {
    /* a failure here is not worth interrupting the reading */
  }
}

const decoder = { '&amp;': '&', '&lt;': '<', '&gt;': '>', '&quot;': '"' }
function plain(s) {
  return (s || '').replace(/&(amp|lt|gt|quot;)/g, (m) => decoder[m] || m)
}

// The root's meaning, looked up among the roots the response carried.
function rootOf(key) {
  if (!key) return null
  return (res.value?.roots || []).find((r) => r.root === key) || null
}

function parts(text) {
  return String(text || '')
    .split('+')
    .map((p) => p.trim())
    .filter(Boolean)
}

// DPD marks a case ending inside a part with <b>; import stored that as a
// sentinel byte, which is not part of the word to look up.
function bare(p) {
  return String(p || '').replace(/[\u0001\u0002]/g, '')
}
</script>

<template>
  <div class="panel dicty" :class="{ compact }">
    <div class="panel-head">
      <!-- One row: the field the reader types in, then the controls that act on
           what is showing. Two stacked rows spent 34px of a 900px panel saying
           the same thing twice — the label above it already says 词典. -->
      <div class="head-row">
        <label class="q">
          <Search :size="14" :stroke-width="1.8" aria-hidden="true" />
          <input
            v-model="query"
            type="search"
            placeholder="查一个词…"
            autocomplete="off"
            spellcheck="false"
            @keydown.enter.prevent="submitSearch"
          />
        </label>

        <div class="head-actions">
          <template v-if="!compact">
            <button class="iconbtn sm" title="上一个词" aria-label="上一个词" @click="emit('step', -1)">
              <ChevronLeft :size="16" />
            </button>
            <button class="iconbtn sm" title="下一个词" aria-label="下一个词" @click="emit('step', 1)">
              <ChevronRight :size="16" />
            </button>
          </template>
          <button
            class="iconbtn sm add-vocab"
            :title="savedToVocab ? '已加入生词本' : '加入生词本'"
            aria-label="加入生词本"
            :disabled="!hasWord || !res"
            @click="addVocab"
          >
            <Check v-if="savedToVocab" :size="15" />
            <BookmarkPlus v-else :size="15" />
          </button>
          <button class="iconbtn sm" title="复制词形" aria-label="复制词形" @click="copyWord">
            <Check v-if="copied" :size="15" />
            <Copy v-else :size="15" />
          </button>
          <button class="iconbtn sm" title="关闭" aria-label="关闭" @click="emit('close')">
            <X :size="16" />
          </button>
        </div>
      </div>

      <template v-if="loading">
        <div class="loading-row"><Loader2 :size="16" class="spin" /><span>查询中…</span></div>
      </template>

      <template v-else-if="res">
        <div class="headword pi">
          {{ head?.lemma || res.key }}
          <span v-if="head?.homonym" class="hom">{{ head.homonym }}</span>
        </div>
        <div v-if="head?.phonetic" class="ipa">{{ head.phonetic }}</div>
        <div class="chips">
          <span v-for="(c, i) in posChips" :key="i" class="tag" :class="{ plain: i > 0 }">{{ c }}</span>
          <span v-if="res.freq" class="tag plain" :title="`全语料出现 ${res.freq} 次`">
            语料 {{ res.freq }} 次
          </span>
        </div>
      </template>
    </div>

    <div class="panel-body">
      <p v-if="!hasWord" class="empty" style="padding: 28px 10px">
        在上面输入一个词，或点击经文里的任一单词。
      </p>

      <div v-else-if="failed" class="empty">{{ failed }}</div>

      <template v-else-if="!loading && res">
        <p v-if="!res.found && !entries.length && !splits.length" class="empty" style="padding: 24px 8px">
          词典中没有这个形式，也没有可用的拆解。
        </p>

        <!-- B. 词形分析 — every row is takeable -->
        <section v-if="analyses.length">
          <div class="sechead">
            <span class="sectitle">词形分析</span>
            <span class="secmeta">{{ analyses.length }} 种读法 · 点 + 记下</span>
          </div>
          <div class="awrap">
            <table class="atable">
              <thead>
                <tr>
                  <th>pos</th>
                  <th>性</th>
                  <th>格</th>
                  <th>数</th>
                  <th class="aof">of</th>
                  <th>word</th>
                  <th v-if="!compact" class="atake"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(a, i) in shownAnalyses" :key="i" :class="{ held: isGrammarHeld(a) }">
                  <td>{{ a.pos }}</td>
                  <td>{{ a.gender || '—' }}</td>
                  <td>{{ a.case || '—' }}</td>
                  <td>{{ a.number || '—' }}</td>
                  <td class="aof">of</td>
                  <td>
                    <button class="wlink pi" @click="follow(a.lemma, $event)">{{ a.lemma }}</button>
                  </td>
                  <td v-if="!compact" class="atake">
                    <button
                      class="take"
                      :class="{ on: isGrammarHeld(a) }"
                      :title="isGrammarHeld(a) ? '删去这一条' : '记下这一条'"
                      @click="toggleGrammar(a)"
                    >
                      <Check v-if="isGrammarHeld(a)" :size="12" :stroke-width="2.4" />
                      <Plus v-else :size="12" :stroke-width="2.4" />
                    </button>
                  </td>
                </tr>
                <tr v-if="analyses.length > ANALYSIS_PREVIEW && !showAllAnalyses" class="more">
                  <td :colspan="compact ? 6 : 7" @click="showAllAnalyses = true">
                    展开全部 {{ analyses.length }} 条 ▾
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section v-else-if="entries.length">
          <div class="sechead"><span class="sectitle">词形分析</span></div>
          <p class="caption">
            词典没有给这个词形分析——多半是不变词（ind），也可能是个未被收录的形式。
          </p>
        </section>

        <!-- C. 拆解 -->
        <section v-if="splits.length">
          <div class="sechead">
            <span class="sectitle">拆解</span>
            <span class="secmeta">{{ head?.compoundType || '' }}</span>
          </div>
          <div class="deconbox">
            <button
              v-for="(sp, i) in splits.slice(0, 6)"
              :key="i"
              class="deconline"
              :class="{ plain: !sp.resolved }"
              :title="sp.resolved ? '点击任一部分继续查词' : '部分词语词典中未收录'"
              @click="follow(bare(sp.parts[0]), $event)"
            >
              <template v-for="(p, j) in sp.parts" :key="j">
                <span v-if="j" class="op">+</span>
                <span class="deconword pi"><RichText :text="p" /></span>
              </template>
            </button>
          </div>
        </section>

        <!-- D. 词条 — each gloss line is takeable on its own -->
        <section v-if="entries.length">
          <div class="sechead">
            <span class="sectitle">词条</span>
            <span class="secmeta">{{ entries.length }} 条 · 点一行记下</span>
          </div>

          <div v-for="(h, i) in entries" :key="h.id || i" class="entry" :class="{ open: expanded[i] }">
            <button class="ehead" :aria-expanded="!!expanded[i]" @click="toggle(i)">
              <ChevronRight :size="13" class="echev" />
              <span class="elemma pi">{{ h.lemma }}</span>
              <span v-if="h.homonym" class="ehom">{{ h.homonym }}</span>
              <span v-if="h.pos" class="epos">{{ h.pos }}</span>
              <span class="egloss">{{ plain(h.meaning1 || h.meaningLit) }}</span>
            </button>

            <div v-show="expanded[i]" class="ebody">
              <!-- 其他词典：点选切换这一条用哪部 -->
              <div v-if="sources.length" class="srcrow">
                <button class="srcchip" :aria-pressed="(picked[i] || 'dpd') === 'dpd'" @click="picked = { ...picked, [i]: 'dpd' }">
                  DPD
                </button>
                <button
                  v-for="m in sources"
                  :key="m.source"
                  class="srcchip"
                  :aria-pressed="picked[i] === m.source"
                  @click="picked = { ...picked, [i]: m.source }"
                >
                  {{ m.name }}
                </button>
              </div>

              <!-- 释义：一条一行，各自可以记下。
                   浮窗里只读——那里的词不是正文中的某一处，记下去没有锚点。 -->
              <ul class="senses">
                <li v-for="(sense, si) in sensesOfEntry(i)" :key="si">
                  <button
                    v-if="compact"
                    class="sense is-readonly"
                    @click="toggleMeaning(sense, i)"
                  >
                    <span class="sense-text">{{ sense }}</span>
                  </button>
                  <button
                    v-else
                    class="sense"
                    :class="{ on: isMeaningHeld(sense, sourceFor(i).name) }"
                    @click="toggleMeaning(sense, i)"
                  >
                    <span class="sense-take">
                      <Check v-if="isMeaningHeld(sense, sourceFor(i).name)" :size="11" :stroke-width="2.6" />
                      <Plus v-else :size="11" :stroke-width="2.6" />
                    </span>
                    <span class="sense-text">{{ sense }}</span>
                  </button>
                </li>
              </ul>
              <p v-if="sourceFor(i).sub" class="caption">lit. {{ plain(sourceFor(i).sub) }}</p>

              <!-- 词条信息 -->
              <table class="gtable">
                <tbody>
                  <tr v-if="h.grammar"><th>Grammar</th><td>{{ h.grammar }}</td></tr>
                  <tr v-if="h.rootKey">
                    <th>Root</th>
                    <td>
                      <button class="wlink pi" @click="follow(h.rootKey, $event)">{{ h.rootKey }}</button>
                      <span v-if="rootOf(h.rootKey)" class="rootgloss">
                        {{ rootOf(h.rootKey).sign ? rootOf(h.rootKey).sign + ' ' : '' }}{{ rootOf(h.rootKey).meaning }}
                      </span>
                    </td>
                  </tr>
                  <tr v-if="h.familyRoot">
                    <th>Root Family</th>
                    <td>
                      <button class="wlink pi" @click="follow(h.familyRoot, $event)">{{ h.familyRoot }}</button>
                    </td>
                  </tr>
                  <tr v-if="h.construction">
                    <th>Construction</th>
                    <td class="ctd">
                      <template v-for="(p, j) in parts(h.construction)" :key="j">
                        <span v-if="j" class="op">+</span>
                        <button class="wlink pi" @click="follow(bare(p), $event)">
                          <RichText :text="p" />
                        </button>
                      </template>
                    </td>
                  </tr>
                  <tr v-if="h.sanskrit"><th>Sanskrit</th><td class="pi">{{ h.sanskrit }}</td></tr>
                  <tr v-if="h.synonym">
                    <th>近义</th>
                    <td>
                      <button class="wlink pi" @click="follow(h.synonym, $event)">{{ h.synonym }}</button>
                    </td>
                  </tr>
                  <tr v-if="h.antonym"><th>反义</th><td class="pi">{{ h.antonym }}</td></tr>
                </tbody>
              </table>

              <div v-if="h.declension" class="declblock">
                <DeclensionTable :decl="h.declension" :limit="0" @pick="(f, ev) => follow(f, ev)" />
              </div>
              <p v-else-if="h.grammar" class="caption">
                DPD 未给这个词条附变格表（可能是动词、不变词或外来词）。
              </p>
            </div>
          </div>
        </section>

        <p v-if="!compact" class="provenance">
          英文释义、语法、变格与拆解来自 DPD（Digital Pāḷi Dictionary）；中文释义来自随附的巴漢／漢譯词典。
        </p>
      </template>
    </div>

  </div>
</template>

<style scoped>
.dicty {
  background: var(--surface);
}
.dicty.compact {
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow);
}
.panel-head {
  padding: 7px 10px 8px;
  border-bottom: 1px solid var(--border-soft);
}
.iconbtn.sm {
  width: 28px;
  height: 28px;
}
/* Barely there: a hairline, no fill, and it takes the accent only while it has
   focus. It has to read as part of the panel, not as a form. */
/* The field and the controls share one row and one baseline. The field keeps a
   hairline under it so it still reads as a field; the controls sit beside it
   rather than under it, which is 34px the panel gets back. */
.head-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
/* On the full-width panel the field takes the room it has. In a floating popup
   it stops short, and the gap between it and the icons is bare header — which
   is what the reader grabs to move the popup, instead of a text field. */
.panel.compact .q {
  flex: 0 1 190px;
}
/* The bookmark lives in the header where space is scarce, and in the footer
   where it is not. */

@media (max-width: 1100px) {
  .q {
    flex: 0 1 190px;
  }
}
.head-actions {
  flex: none;
  /* Pinned right. With the field no longer filling the row the actions would
     otherwise sit against it, which reads as one crowded control. */
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 2px;
}
/* No rule under it. A line, box or fill that is always drawn says "this is a
   form"; the panel is a reading surface and the search is one thing you can do
   in it. So it is drawn like the rest of the interface — a wash that appears on
   hover and stays while it has focus — and until you reach for it the row is
   just a magnifier and a placeholder. */
.q {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 7px;
  border-radius: var(--radius-sm);
  color: var(--meta);
  transition: background var(--fast), color var(--fast);
}
.q:hover {
  background: var(--surface-warm);
  color: var(--fg-2);
}
.q:focus-within {
  background: var(--tag-faint);
  color: var(--accent);
}
.q input {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--fg);
  outline: none;
}
.q input::placeholder {
  color: var(--meta);
  opacity: 0.75;
}
/* Safari draws its own clear button on type=search, which looks foreign here. */
.q input::-webkit-search-decoration,
.q input::-webkit-search-cancel-button {
  -webkit-appearance: none;
}
.loading-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  color: var(--meta);
  font-size: 12.5px;
}
.headword {
  margin-top: 4px;
  font-size: 23px;
  font-weight: 600;
  line-height: 1.2;
  color: var(--fg);
  overflow-wrap: anywhere;
}
.headword .hom {
  margin-left: 6px;
  vertical-align: 6px;
  font-size: 12px;
  font-family: var(--sans);
  color: var(--meta);
}
.ipa {
  margin-top: 3px;
  font-size: 12.5px;
  color: var(--meta);
}

/* 词形分析 */
.awrap {
  overflow-x: auto;
  overscroll-behavior-x: contain;
}
.atable {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.atable th,
.atable td {
  padding: 3px 5px;
  text-align: left;
  border-bottom: 1px solid var(--border-soft);
  white-space: nowrap;
}
.atable thead th {
  font-size: 10.5px;
  font-weight: 600;
  letter-spacing: 0.4px;
  color: var(--muted);
  background: var(--surface-warm);
}
.atable tbody tr:hover td,
.atable tbody tr.held td {
  background: var(--tag-faint);
}
.atable td {
  font-size: var(--fs-read-sm);
  /* The panel inherits the reading column's 1.55, which is right for prose and
     loose for a table: at 13px it is 20px of line box on a 6px pad. */
  line-height: 1.3;
  color: var(--fg-2);
}
.atable .aof {
  width: 20px;
  color: var(--meta);
  font-size: 10.5px;
}
.atake {
  width: 28px;
  text-align: right;
}
.take {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: var(--radius-xs);
  color: var(--meta);
}
.take:hover {
  background: var(--tag-faint);
  color: var(--accent);
}
.take.on {
  background: var(--accent);
  color: var(--surface);
}
.atable tr.more td {
  padding: 7px;
  text-align: center;
  font-size: 12px;
  color: var(--accent);
  font-weight: 500;
  cursor: pointer;
}
.atable tr.more td:hover {
  background: var(--tag-faint);
}

/* 释义 — one sense per row, each takeable on its own. */
.senses {
  margin: 2px 0 5px;
}
.senses li + li {
  margin-top: 2px;
}
.sense {
  display: flex;
  align-items: baseline;
  gap: 7px;
  width: 100%;
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  text-align: left;
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--fg);
}
.sense:hover,
.sense.on {
  background: var(--tag-faint);
}
.sense.is-readonly {
  cursor: default;
}
.sense.is-readonly:hover {
  background: transparent;
}
.sense.on {
  color: var(--accent);
}
.sense-take {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--surface-warm);
  color: var(--meta);
}
.sense.on .sense-take {
  background: var(--accent);
  color: var(--surface);
}
.sense-text {
  flex: 1;
  min-width: 0;
}

/* A Pāḷi word that leads somewhere opens beside the panel, never over it. */
.wlink {
  padding: 0 1px;
  font-family: var(--sans);
  font-size: var(--fs-read-sm);
  color: var(--accent);
  border-radius: var(--radius-xs);
}
.wlink:hover {
  background: var(--tag-faint);
  text-decoration: underline;
  text-decoration-style: dotted;
  text-underline-offset: 3px;
}
.ctd {
  line-height: 1.9;
}
.op {
  margin: 0 4px;
  color: var(--meta);
}
.rootgloss {
  margin-left: 6px;
  font-size: 12px;
  color: var(--muted);
}

/* 拆解 — the one block that gets a ground of its own, because the whole block
   is one statement: this word comes apart, like this. */
.deconbox {
  padding: 2px 0;
}
.deconline {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 2px;
  width: 100%;
  padding: 6px 9px;
  margin-bottom: 3px;
  border: 1px solid #cfdcec;
  border-radius: var(--radius-md);
  background: var(--tag-faint);
  text-align: left;
}
.deconline.plain {
  border-color: var(--border);
  background: var(--surface-warm);
}
.deconline:hover {
  border-color: var(--accent);
}
.deconword {
  font-family: var(--sans);
  font-size: var(--fs-read);
  line-height: 1.5;
  font-weight: 500;
  color: var(--accent);
}
.deconline.plain .deconword {
  color: var(--fg-2);
}

/* 词条 */
/* Square, like the tab bar. Without a border or a background of its own there
   is nothing for a radius to round — an entry is a row in the panel, not a card
   sitting on it. */
.entry {
  line-height: 1.4;
  overflow: hidden;
}
.entry + .entry {
  margin-top: 3px;
}
/* No side padding: the entry already sits inside the panel's own margin, so a
   second inset only narrows the row that carries the most text — the same
   reason the expanded body has none. Vertical padding stays: it is what makes
   the row a comfortable target. */
.ehead {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  padding: 5px 0;
  text-align: left;
  transition: background var(--fast);
}
.ehead:hover {
  background: var(--surface-warm);
}
.echev {
  flex: none;
  color: var(--meta);
  transition: transform var(--fast);
}
.ehead[aria-expanded='true'] .echev {
  transform: rotate(90deg);
}
.elemma {
  flex: none;
  font-size: 15px;
  font-weight: 600;
  color: var(--fg);
}
.ehom {
  flex: none;
  font-size: 10.5px;
  color: var(--meta);
  vertical-align: 4px;
}
.epos {
  flex: none;
  font-size: 11px;
  color: var(--muted);
}
.egloss {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* No side padding: the card is already inside the panel's own margin, and a
   second one only narrows the rows that carry the most text. */
.ebody {
  padding: 1px 0 7px;
  line-height: 1.45;
}
.srcrow {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin: 4px 0 6px;
}
.srcchip {
  padding: 3px 9px;
  border-radius: var(--radius-xs);
  background: var(--surface-warm);
  font-size: 11px;
  color: var(--fg-2);
}
.srcchip:hover {
  background: var(--tag-faint);
}
.srcchip[aria-pressed='true'] {
  background: var(--accent);
  color: var(--surface);
}
.declblock {
  margin-top: 8px;
  padding-top: 7px;
  border-top: 1px solid var(--border-soft);
}
.spin {
  animation: spin 700ms linear infinite;
}
</style>
