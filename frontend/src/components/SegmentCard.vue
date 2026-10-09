<script setup>
// One block of the canon, with everything the reader can put under it.
//
// The stack is fixed and always in the same order, because the reader learns to
// read it once and then never has to think about it again:
//
//   原文 → 词 (the readings recorded) → 批 (my commentary) → 译 (my translation) → 参 (published)
//
// 参 is last because it is the only row the reader did not write, and it sits
// directly under the sentence it renders — the same place tipitaka-pali-reader
// puts it, and for the same reason: a translation is only useful while the
// words it renders are still on screen. It is shown by default (settings
// refMode 'all'); 阅读设置 can turn it off or ask for a click per paragraph.
//
// There is no card around any of this. A border round every paragraph breaks a
// continuous text into a hundred separate objects and costs the reader the one
// thing the page is for; the text column is one column.
import { computed, ref, watch } from 'vue'
import {
  BookOpen,
  Check,
  Languages,
  MessageSquarePlus,
  Pencil,
  PenLine,
  Trash2,
  X,
} from 'lucide-vue-next'
import PaliText from './PaliText.vue'
import { useSettings } from '../store/settings'

const props = defineProps({
  seg: { type: Object, required: true },
  picksFor: { type: Function, required: true },
  splitFor: { type: Function, required: true },
  notesFor: { type: Function, required: true },
  translation: { type: Object, default: null },
  // Every published translation of this segment, keyed by language. Which of
  // them is shown is the reader's 参考译文 setting.
  references: { type: Object, default: () => ({}) },
  openKey: { type: String, default: '' },
  signedIn: { type: Boolean, default: false },
  active: { type: Boolean, default: false },
  // The reader's per-sentence overrides for this segment: `{zh?: bool, en?: bool}`.
  // A missing key follows the setting; `false` is an override saying "not this
  // one", which is what makes the button work in 全部展开 mode.
  refsHere: { type: Object, default: () => ({}) },
  // The first segment of its paragraph carries the paragraph's own 中/英 pair,
  // which opens the whole paragraph at once. A paragraph is several sentences
  // now, so without it a reader would click once per sentence.
  isParaHead: { type: Boolean, default: false },
  paraSeqs: { type: Array, default: () => [] },
  // Which languages the paragraph could show at all, so the paragraph button is
  // not offered for a language no sentence of it has.
  paraHas: { type: Object, default: () => ({}) },
  // Whether every sentence of the paragraph is currently showing that language.
  // True only when they all are: a partly-open paragraph should read as closed,
  // so that one click opens the rest rather than closing what is open.
  paraOn: { type: Object, default: () => ({}) },
})
const emit = defineEmits([
  'pick',
  'unpick',
  'toggle-ref',
  'toggle-para-ref',
  'save-translation',
  'add-note',
  'delete-note',
  'need-auth',
])

const S = useSettings()
const isHeading = computed(() => props.seg.kind === 'heading')
// A numbered paragraph starts a new section of the text; it gets more air
// above it than a continuation line does.
const numbered = computed(() => /^\s*\d/.test(props.seg.text || ''))

// The reference lines to draw when the reader has asked for them, in the order
// the 参考译文 setting chose. "全部" shows both, each labelled with its language
// so they are not mistaken for one another.
// 全部展开 short-circuits the per-segment state: the reader has said they want
// the translations everywhere, so every paragraph shows what it has.
// The override wins; without one, the setting decides. `false` is a real
// value here — it is a reader saying "not this one" in 全部展开 mode — so the
// test is against `undefined`, not falsiness.
function shown(lang) {
  return props.refsHere[lang] ?? S.refDefaultOn
}

const refLines = computed(() => {
  const out = []
  for (const l of ['zh', 'en']) {
    if (!shown(l)) continue
    const text = props.references?.[l]
    if (text) out.push({ lang: l, text })
  }
  return out
})

// Which languages this segment could show, whether or not they are showing.
// The 中 / 英 buttons are drawn from this, so a paragraph with no English does
// not offer an English button that would do nothing. 隐藏 is the reader saying
// they do not want the buttons at all.
//
// In 全部 they are not drawn either, because they would be dead controls: the
// setting has already said every paragraph shows what it has, so a button
// offering to show this one would be a switch already on — and drawing it
// unlit, as it was before, told the reader the opposite of what the page was
// doing.
// Always offered, in both modes. The setting says what a translation starts
// as; it does not take the button away, or a reader who chose 隐藏 would have
// no way back except the settings panel. One button opens this sentence; in
// 全部展开 the same button closes it.
const available = computed(() =>
  ['zh', 'en'].filter((l) => !!props.references?.[l]))

// The pair at the head of a paragraph, which works on every sentence of it.
const paraAvailable = computed(() =>
  !props.isParaHead ? [] : ['zh', 'en'].filter((l) => props.paraHas?.[l]))
const refSource = (lang) => (lang === 'en' ? 'ePitaka 英译' : 'ePitaka 中译')

// Every reading the reader has recorded against this segment, in text order.
// One chip group per word, so a sentence that has been worked through reads as
// a sentence with its grammar spelled out under it.
const marked = computed(() => {
  const text = props.seg.text || ''
  const toks = props.seg.tokens || []
  const out = []
  toks.forEach((tok, i) => {
    const picks = props.picksFor(props.seg.seq, i)
    if (!picks.length) return
    out.push({ index: i, surface: text.slice(tok[0], tok[0] + tok[1]), picks })
  })
  return out
})

// A chip's label. A grammar pick shows the case and number, which is the whole
// reason the reader recorded it; a meaning pick shows the gloss.
function chipLabel(p) {
  if (p.kind === 'grammar') return [p.pos, p.gender, p.case, p.number].filter(Boolean).join(' ')
  if (p.kind === 'meaning') return p.meaning
  return p.split
}

const segNotes = computed(() => props.notesFor(props.seg.seq, null))

// Notes are stamped by the server, whose clock may be in another timezone.
// "3 分钟前" is what a reader wants anyway, and it cannot be misread.
function timeAgo(iso) {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const secs = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (secs < 60) return '刚刚'
  const mins = Math.round(secs / 60)
  if (mins < 60) return `${mins} 分钟前`
  const hours = Math.round(mins / 60)
  if (hours < 24) return `${hours} 小时前`
  const days = Math.round(hours / 24)
  if (days < 30) return `${days} 天前`
  return new Date(iso).toLocaleDateString('zh-CN')
}

// --- my translation ------------------------------------------------------
// Saved on an explicit 保存, the same as a note. Autosave was clever and wrong:
// the reader could not tell whether a sentence they were still working on had
// been committed, and there was no way to abandon an edit.
const draft = ref(props.translation?.text || '')
const transOpen = ref(false)
const transSaving = ref(false)

watch(
  () => props.translation?.text,
  (v) => {
    if (!transOpen.value) draft.value = v || ''
  },
)

function toggleTranslation() {
  if (transOpen.value) {
    transOpen.value = false
    return
  }
  if (!props.signedIn) {
    emit('need-auth')
    return
  }
  draft.value = props.translation?.text || ''
  transOpen.value = true
}
function saveTranslation() {
  draft.value = draft.value.trim()
  if (!draft.value) {
    transOpen.value = false
    return
  }
  transSaving.value = true
  emit('save-translation', { segment: props.seg.seq, text: draft.value })
  transOpen.value = false
  transSaving.value = false
}
function cancelTranslation() {
  draft.value = props.translation?.text || ''
  transOpen.value = false
}

const savedAt = computed(() => {
  const t = props.translation?.updatedAt
  if (!t) return ''
  const d = new Date(t)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
})

// --- notes ---------------------------------------------------------------
const noteDraft = ref('')
const noteOpen = ref(false)
const noteSaving = ref(false)

function toggleNote() {
  if (noteOpen.value) {
    noteOpen.value = false
    return
  }
  if (!props.signedIn) {
    emit('need-auth')
    return
  }
  noteOpen.value = true
}
async function submitNote() {
  const body = noteDraft.value.trim()
  if (!body) {
    noteOpen.value = false
    return
  }
  noteSaving.value = true
  emit('add-note', { segment: props.seg.seq, wordIndex: -1, kind: 'segment', body })
  noteDraft.value = ''
  noteOpen.value = false
  noteSaving.value = false
}

const mine = computed(() => (props.translation?.text || '').trim())
</script>

<template>
  <article
    class="seg"
    :class="[
      { center: seg.kind === 'center', 'is-target': active },
      isHeading ? 'seg-heading' : '',
      isHeading ? 'lv' + (seg.level || 4) : '',
      !isHeading && numbered ? 'is-numbered' : '',
    ]"
    :data-seq="seg.seq"
  >
    <span class="sr">{{ seg.seq }}</span>

    <!-- The paragraph's own 中 / 英 pair, at its top-left. A paragraph is
         several sentences now, so opening it sentence by sentence is ten
         clicks; this opens the lot. Only the first sentence of the paragraph
         carries it, and only for a language some sentence of it actually has. -->
    <div v-if="paraAvailable.length" class="para-tools">
      <button
        v-for="l in paraAvailable"
        :key="l"
        type="button"
        class="seg-act is-lang"
        :class="{ 'is-on': paraOn[l] }"
        :aria-pressed="!!paraOn[l]"
        :title="
          (paraOn[l] ? '收起' : '显示') +
          '整段' +
          (l === 'zh' ? '的中文' : '的英文') +
          '参考译文'
        "
        @click="emit('toggle-para-ref', { seqs: paraSeqs, lang: l, on: !paraOn[l] })"
      >
        {{ l === 'zh' ? '中' : '英' }}
      </button>
    </div>

    <PaliText
      v-if="!isHeading"
      :seg="seg"
      :picks-for="picksFor"
      :split-for="splitFor"
      :open-key="openKey"
      @pick="(p) => emit('pick', p)"
    />
    <PaliText
      v-else
      :seg="seg"
      :picks-for="picksFor"
      :split-for="splitFor"
      :open-key="openKey"
      heading
      @pick="(p) => emit('pick', p)"
      @unpick="(p) => emit('unpick', p)"
      @link="(p) => emit('link', p)"
      @split="(p) => emit('split', p)"
    />

    <!-- Segment actions. On a pointer device they sit in the margin and appear
         on hover, so the column stays clean; on touch there is no hover, so
         they are always there and small. -->
    <div v-if="!isHeading" class="seg-tools">
      <button
        v-for="l in available"
        :key="l"
        class="seg-act is-lang"
        type="button"
        :class="{ 'is-on': shown(l) }"
        :aria-pressed="shown(l)"
        :title="(shown(l) ? '收起' : '显示') + '这一句' + (l === 'zh' ? '的中文' : '的英文') + '参考译文'"
        @click="emit('toggle-ref', { segment: seg.seq, lang: l })"
      >
        {{ l === 'zh' ? '中' : '英' }}
      </button>
      <button
        class="seg-act"
        type="button"
        :class="{ 'is-on': noteOpen || segNotes.length }"
        :aria-expanded="noteOpen"
        title="批注"
        @click="toggleNote"
      >
        <MessageSquarePlus :size="15" :stroke-width="1.7" aria-hidden="true" />
        <span class="sr-only">批注</span>
      </button>
      <button
        v-if="signedIn"
        class="seg-act"
        type="button"
        :class="{ 'is-on': transOpen || !!mine }"
        :aria-expanded="transOpen"
        title="我的翻译"
        @click="toggleTranslation"
      >
        <PenLine :size="15" :stroke-width="1.7" aria-hidden="true" />
        <span class="sr-only">我的翻译</span>
      </button>
    </div>

    <!-- 词 — what the reader has decided this word is, here. A Pāḷi form is
         ambiguous, so a word can carry several chips and any of them can be
         struck out as the sentence resolves. -->
    <div v-if="marked.length" class="line line--picks">
      <span class="line-tag"><BookOpen :size="13" :stroke-width="1.8" /></span>
      <div class="picks">
        <span v-for="m in marked" :key="m.index" class="pickgroup">
          <b class="pw pi">{{ m.surface }}</b>
          <span
            v-for="p in m.picks"
            :key="p.key"
            class="chip"
            :class="'chip--' + p.kind"
            :title="p.kind === 'meaning' && p.meaningSource ? '释义来自 ' + p.meaningSource : ''"
          >
            <span class="chip-sense">{{ chipLabel(p) }}</span>
            <button
              class="chip-x"
              type="button"
              title="删去这一条"
              @click.stop="emit('unpick', { segment: seg.seq, wordIndex: m.index, key: p.key })"
            >
              <X :size="11" :stroke-width="2.2" aria-hidden="true" />
            </button>
          </span>
        </span>
      </div>
    </div>

    <!-- 批 — the reader's own commentary -->
    <template v-if="!isHeading">
      <div v-for="n in segNotes" :key="n.id" class="line line--note">
        <span class="line-tag"><MessageSquarePlus :size="13" :stroke-width="1.8" /></span>
        <div class="line-body">
          <span class="line-time">{{ timeAgo(n.updatedAt) }}</span>
          {{ n.body }}
        </div>
        <button class="line-x" type="button" title="删除" @click="emit('delete-note', n)">
          <Trash2 :size="13" :stroke-width="1.9" aria-hidden="true" />
        </button>
      </div>

      <Transition name="pop">
        <div v-if="noteOpen" class="line line--note is-editing">
          <span class="line-tag"><MessageSquarePlus :size="13" :stroke-width="1.8" /></span>
          <div class="line-body">
            <textarea
              v-model="noteDraft"
              class="mine"
              rows="2"
              placeholder="写下你的批注…"
              autofocus
              @keydown.meta.enter="submitNote"
              @keydown.ctrl.enter="submitNote"
              @keydown.esc="noteOpen = false"
            />
            <div class="edit-actions">
              <button class="btn btn-primary btn-sm" :disabled="noteSaving" @click="submitNote">
                <Check :size="13" />保存
              </button>
              <button class="btn btn-quiet btn-sm" @click="noteOpen = false">取消</button>
            </div>
          </div>
        </div>
      </Transition>
    </template>

    <!-- 译 — the reader's own rendering -->
    <Transition name="pop">
      <div v-if="!isHeading && transOpen" class="line line--own is-editing">
        <span class="line-tag"><PenLine :size="13" :stroke-width="1.8" /></span>
        <div class="line-body">
          <textarea
            v-model="draft"
            class="mine"
            rows="1"
            placeholder="在此填写你的翻译…"
            autofocus
            @keydown.meta.enter="saveTranslation"
            @keydown.ctrl.enter="saveTranslation"
            @keydown.esc="cancelTranslation"
          />
          <div class="edit-actions">
            <button class="btn btn-primary btn-sm" :disabled="transSaving" @click="saveTranslation">
              <Check :size="13" />保存
            </button>
            <button class="btn btn-quiet btn-sm" @click="cancelTranslation">取消</button>
          </div>
        </div>
      </div>
    </Transition>
    <div v-if="!isHeading && !transOpen && mine" class="line line--own">
      <span class="line-tag"><PenLine :size="13" :stroke-width="1.8" /></span>
      <div class="line-body">
        {{ mine }}
        <span class="line-time">{{ savedAt ? '已保存 · ' + savedAt : '' }}</span>
      </div>
      <button class="line-x" type="button" title="修改" @click="toggleTranslation">
        <Pencil :size="13" :stroke-width="1.9" aria-hidden="true" />
      </button>
    </div>

    <!-- 参 — the published translations this segment has. Drawn for headings
         too: a heading is a row of the same data as any other sentence, and its
         translation is filed under the same key, so the reader gets
         「1. naḷavaggo / 1. 芦苇品」 in the reading column as well as in the
         contents. -->
    <template v-if="refLines.length">
      <div v-for="(r, i) in refLines" :key="r.lang" class="line line--ref">
        <span class="line-tag"><Languages :size="13" :stroke-width="1.8" /></span>
        <div class="line-body" :lang="r.lang === 'zh' ? 'zh-Hans' : 'en'">
          {{ r.text }}<span class="src">{{ refSource(r.lang) }}</span>
        </div>
      </div>
    </template>
  </article>
</template>

<style scoped>
.seg {
  position: relative;
  padding: 0 4px;
}

/* Hidden until the pointer is on the segment, on a device that has a pointer.
   The page is wide and these sit beside every sentence; drawn always they are
   a column of small boxes competing with the text.
   On a touch screen there is no hover to reveal them, so there they are always
   drawn — `hover: none` is the condition, not a width: the question is whether
   the device can hover, not how wide it is. */
/* Drawn by default, and hidden only where hovering is possible.
   Written this way round on purpose: if a browser does not know the `hover`
   feature the safe answer is to show the controls, not to hide them behind an
   interaction the device may not have.
   `:has(.is-on)` used to keep them visible wherever a translation was open —
   which in 全部展开 meant every segment, so the whole page was a column of
   buttons and hovering changed nothing. The open translation is its own signal;
   the button does not have to repeat it. */
.seg-tools {
  position: absolute;
  top: 0;
  right: -6px;
  display: flex;
  gap: 2px;
  opacity: 1;
  transition: opacity var(--fast);
}
@media (hover: hover) {
  .seg-tools {
    opacity: 0;
  }
  .seg:hover .seg-tools,
  .seg-tools:hover,
  .seg-tools:focus-within {
    opacity: 1;
  }
}
/* The paragraph's own pair, above its first sentence. Same rule as .seg-tools:
   revealed by the pointer, or always drawn where there is no pointer. */
.para-tools {
  position: absolute;
  top: 0;
  /* In the margin, not on the text. At left:0 these sat on top of the paragraph
     number — the first thing the reader looks at to find their place. */
  left: -66px;
  justify-content: flex-end;
  width: 60px;
  display: flex;
  gap: 2px;
  opacity: 1;
  transition: opacity var(--fast);
}
@media (hover: hover) {
  .para-tools {
    opacity: 0;
  }
  .seg:hover .para-tools,
  .para-tools:hover,
  .para-tools:focus-within {
    opacity: 1;
  }
}
.seg-act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 27px;
  height: 27px;
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--muted);
  box-shadow: var(--ring);
  transition: background var(--fast), color var(--fast);
}
@media (max-width: 1000px) {
  .para-tools {
    position: static;
    width: auto;
    margin-bottom: 4px;
  }
}
.seg-act:hover,
.seg-act.is-on {
  background: var(--tag-faint);
  color: var(--accent);
}
/* The 中 / 英 buttons carry a character rather than a glyph: a language is not
   an icon, and an icon for it would have to be learned. */
.seg-act.is-lang {
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0;
}
.seg-act.is-lang.is-on {
  background: var(--accent);
  color: var(--surface);
}
.seg-act.is-lang.is-on:hover {
  background: var(--accent-active);
  color: var(--surface);
}
.seg:hover .seg-tools,
.seg:focus-within .seg-tools {
  opacity: 1;
}

/* A row under the text: an icon chip, the content, and whatever dismisses it.
   The ground carries ownership; the one accent rule is reserved for 译. */
.line {
  display: flex;
  align-items: baseline;
  gap: 9px;
  margin: 8px 0 0 2px;
  padding: 7px 10px;
  border-radius: var(--radius-md);
  font-size: 13.5px;
  line-height: 1.7;
}
/* The tag is an icon, not a character: at this size a single Han glyph costs
   more width than it earns, and the row is for the reader's own words. */
.line-tag {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: var(--radius-xs);
}
.line-body {
  flex: 1;
  min-width: 0;
  white-space: pre-wrap;
}
.line-time {
  margin-left: 8px;
  font-size: 11px;
  color: var(--meta);
}
.line-x {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: var(--radius-sm);
  color: inherit;
  opacity: 0.55;
}
.line-x:hover {
  opacity: 1;
  background: rgba(179, 58, 58, 0.08);
  color: #b33a3a;
}

/* 词 — the readings the reader recorded. */
.line--picks {
  flex-wrap: wrap;
  row-gap: 5px;
  background: var(--row-word);
}
.line--picks .line-tag {
  background: var(--surface);
  color: var(--muted);
}
.picks {
  display: flex;
  flex-wrap: wrap;
  gap: 5px 16px;
  min-width: 0;
}
.pickgroup {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}
.pw {
  font-weight: 600;
  color: var(--fg);
}
.chip {
  display: inline-flex;
  align-items: baseline;
  gap: 3px;
  padding: 1px 3px 1px 7px;
  border-radius: var(--radius-xs);
  background: var(--surface);
  font-size: 12px;
  line-height: 1.6;
  white-space: nowrap;
}
.chip-sense {
  color: var(--accent);
}
/* A grammar reading is a fact about the form; a meaning is a decision about it.
   One warm chip and one cool chip separate them with no second accent colour. */
.chip--grammar {
  background: var(--chip-gram-bg);
}
.chip--grammar .chip-sense {
  color: var(--chip-gram-ink);
}
.chip--meaning {
  background: var(--chip-mean-bg);
}
.chip--meaning .chip-sense {
  color: var(--chip-mean-ink);
}
.chip--split .chip-sense {
  color: var(--meta);
}
.chip-x {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 3px;
  color: var(--meta);
}
.chip-x:hover {
  background: rgba(179, 58, 58, 0.1);
  color: #b33a3a;
}

/* My hand in the margin: warm paper, lifted off the parchment by a ring rather
   than a rule, and the one row whose text leans very slightly. */
.line--note {
  background: var(--row-note);
  box-shadow: 0 0 0 1px var(--row-note-ring);
  color: var(--fg-2);
}
.line--note .line-tag {
  background: var(--ico-note);
  color: #7a6a4a;
}
.line--note.is-editing,
.line--own.is-editing {
  background: var(--surface);
  box-shadow: var(--ring);
}
/* My finished sentence. The accent rule sits in the margin beside the row
   rather than as a border on it — four cards each wearing a left border is the
   stock dashboard shape, and repeating it four times is what made the first
   attempt read as one block. */
.line--own {
  position: relative;
  background: var(--row-trans);
  color: var(--fg);
  box-shadow: 0 0 0 1px var(--ring-soft);
}
.line--own::before {
  content: '';
  position: absolute;
  left: -10px;
  top: 3px;
  bottom: 3px;
  width: 2px;
  border-radius: 2px;
  background: var(--accent);
}
.line--own .line-tag {
  background: var(--chip-mean-bg);
  color: var(--accent);
}
/* Somebody else's text, and the only row the reader did not write. It is set
   apart by structure — an indent and hairlines — and by carrying no chip and no
   dismiss button, NOT by being dimmed: someone else's translation is worth
   reading at full contrast, and greying it out confuses "not mine" with
   "unimportant". */
/* Aligned with 批 and 译, not indented under them. The indent made the row sit
   a whole step to the right of everything else, so the eye read it as a
   sub-item of the note above rather than as a sibling. Its own icon and the
   hairline are enough to say "someone else wrote this". */
.line--ref {
  background: transparent;
  padding: 9px 10px;
  border-radius: 0;
  border-top: 1px solid var(--ref-rule);
  border-bottom: 1px solid var(--ref-rule);
  color: var(--fg-2);
}
.line--ref + .line--ref {
  margin-top: 0;
  border-top: 0;
}
.line--ref .line-tag {
  background: transparent;
  color: var(--muted);
}
.line--ref .src {
  color: var(--accent);
}

/* Opening and closing an editor is a small event in a long page; the motion is
   what says which row just changed. */
.pop-enter-active,
.pop-leave-active {
  transition: opacity 160ms ease, transform 160ms ease;
}
.pop-enter-from,
.pop-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
.edit-actions {
  display: flex;
  gap: 8px;
  margin-top: 7px;
}
.edit-actions .btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.mine {
  width: 100%;
  padding: 4px 6px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  font: inherit;
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--fg);
  resize: vertical;
}
.mine:focus {
  outline: none;
  border-color: var(--accent);
}
.save-state {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  font-size: 11px;
  color: var(--meta);
}
.save-state.dirty {
  color: #8a5a16;
}
.src {
  margin-left: 8px;
  font-size: 11px;
  color: var(--meta);
}

/* A narrow column has nowhere to put an absolutely positioned pair, so it goes
   in flow below the sentence, right-aligned.
   It does NOT set opacity. This rule used to, under a comment saying "no hover
   on touch" — its intent was the pointer, but it asked about the width, so on a
   narrow desktop window the buttons were drawn always and could not be hidden.
   Whether the device can hover is asked once, by `hover: none` above; this asks
   only whether there is room. */
@media (max-width: 1100px) {
  .seg-tools {
    position: static;
    margin: 6px 0 0 2px;
    justify-content: flex-end;
  }
}
</style>
