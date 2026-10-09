<script setup>
// The catalogue rail: 三藏 → 部 / 尼柯耶 → 书 → 当前书的章节.
//
// The rail is the answer to "where am I", so it opens itself to the current
// book and lists that book's sections beneath it. Collapsed to a rail on narrow
// screens; on a phone it is the drawer.
import { computed, ref, watch } from 'vue'
import { ChevronRight, PanelLeftClose } from 'lucide-vue-next'
import { useSettings } from '../store/settings'
import { House } from 'lucide-vue-next'
import { tocStyle } from '../utils/numbering'

const props = defineProps({
  catalog: { type: Array, default: () => [] },
  current: { type: String, default: '' },
  // The current book's table of contents, listed under it in the rail.
  toc: { type: Array, default: () => [] },
  // The published translation of each heading, looked up by segment. Nullable
  // on purpose: a Function-typed prop uses its `default` AS the value rather
  // than calling it, so a default that "returns a function" hands the component
  // the wrong thing and the page renders that function's source text. Callers
  // pass it, and the template checks before calling.
  tocRefFor: { type: Function, default: null },
  activeSeq: { type: Number, default: 0 },
  collapsed: { type: Boolean, default: false },
})
const emit = defineEmits(['collapse', 'goto'])

const S = useSettings()
const open = ref({})
const showToc = ref(true)

// Open the branch holding the book being read.
//
// This has to run when the catalogue arrives as well as when the book changes:
// on a cold load the book is known before the tree is, and watching only the
// book left the rail shut with the reader somewhere inside it.
function reveal() {
  const id = props.current
  if (!id || !props.catalog.length) return
  for (const b of props.catalog) {
    for (const c of b.categories) {
      if (c.books.some((x) => x.id === id)) {
        open.value[b.code] = true
        open.value[b.code + '/' + c.id] = true
      }
    }
  }
}
watch(() => props.current, reveal, { immediate: true })
watch(() => props.catalog, reveal, { immediate: true })

const baskets = computed(() => props.catalog || [])

// Chapters and suttas only — the rail is for getting somewhere, and the
// contents rail on the right already lists every sub-heading. The names are
// shown exactly as the canon writes them ("1. brahmajālasuttaṃ"); no number of
// ours is added.
const chapters = computed(() => props.toc.filter((t) => (t.level || 9) === 3 || (t.level || 9) === 4))

const activeChapter = computed(() => {
  let idx = -1
  for (let i = 0; i < chapters.value.length; i++) {
    if (chapters.value[i].seq <= props.activeSeq) idx = i
    else break
  }
  return idx
})

function toggle(key) {
  open.value[key] = !open.value[key]
}

const bookName = (bk) => S.primaryName(bk.nameZh, bk.name)
const bookPali = (bk) => S.secondaryName(bk.nameZh, bk.name)
</script>

<template>
  <div v-if="collapsed" class="trail">
    <button class="iconbtn" title="展开目录" @click="emit('collapse')">
      <PanelLeftClose :size="18" style="transform: rotate(180deg)" />
    </button>
  </div>

  <div v-else class="tree-full">
    <div class="tree-head">
      <!-- The icon and the word are one link home. A reader tapping 「三藏」 is
           asking for the whole canon, which is the catalogue page — so the
           heading says what it is and does what it says, and the drawer does
           not need a separate row above it saying the same thing. -->
      <router-link to="/" class="tree-home" title="全部典籍">
        <House :size="15" :stroke-width="1.8" aria-hidden="true" />
        <span class="eyebrow">三藏 · Tipiṭaka</span>
      </router-link>
      <button
        class="iconbtn"
        style="width: 32px; height: 32px"
        title="收起目录"
        aria-label="收起目录"
        @click="emit('collapse')"
      >
        <PanelLeftClose :size="16" />
      </button>
    </div>
    <div class="tree-scroll">
      <div v-for="b in baskets" :key="b.code" style="margin-bottom: 4px">
        <button class="tnode" :aria-expanded="!!open[b.code]" @click="toggle(b.code)">
          <ChevronRight :size="13" class="chev" />
          <span class="lbl">
            <span class="ln">
              {{ S.primaryName(b.name, b.namePi) }}
              <span class="pi pl">{{ S.secondaryName(b.name, b.namePi) }}</span>
            </span>
            <span class="sub">{{ b.bookCount }} 部</span>
          </span>
        </button>
        <div v-show="open[b.code]" class="tgroup">
          <div v-for="c in b.categories" :key="c.id">
            <button
              class="tnode"
              :aria-expanded="!!open[b.code + '/' + c.id]"
              @click="toggle(b.code + '/' + c.id)"
            >
              <ChevronRight :size="12" class="chev" />
              <span class="lbl">
                <span class="ln">
                  {{ S.primaryName(c.nameZh, c.namePi || c.name) }}
                  <span class="pi pl">{{ S.secondaryName(c.nameZh, c.namePi || c.name) }}</span>
                </span>
                <span class="sub">{{ c.books.length }} 部</span>
              </span>
            </button>
            <div v-show="open[b.code + '/' + c.id]" class="tgroup">
              <template v-for="bk in c.books" :key="bk.id">
                <div class="bookrow" :class="{ 'is-cur': bk.id === current }">
                  <router-link :to="`/read/${bk.id}`" class="tnode book">
                    <span class="lbl">
                      {{ bookName(bk) }}
                      <span v-if="bookPali(bk)" class="sub pi">{{ bookPali(bk) }}</span>
                    </span>
                  </router-link>
                  <!-- The current book's own sections, so the rail says which
                       chapter the reader is in and lets them jump. -->
                  <button
                    v-if="bk.id === current && chapters.length"
                    class="toc-toggle"
                    :aria-expanded="showToc"
                    :title="showToc ? '收起章节' : '展开章节'"
                    @click.stop="showToc = !showToc"
                  >
                    <ChevronRight
                      :size="12"
                      class="chev"
                      :style="{ transform: showToc ? 'rotate(90deg)' : 'none' }"
                    />
                  </button>
                </div>
                <div v-if="bk.id === current && showToc && chapters.length" class="chapters">
                  <button
                    v-for="(t, i) in chapters"
                    :key="i"
                    class="chapter"
                    :class="{ 'is-cur': i === activeChapter }"
                    :style="{ paddingLeft: 6 + tocStyle(t.level).indent + 'px' }"
                    @click="emit('goto', t.seq)"
                  >
                    <span class="ct">
                      {{ t.name }}
                      <span v-if="tocRefFor && tocRefFor(t.seq)" class="cref">{{
                        tocRefFor(t.seq)
                      }}</span>
                    </span>
                  </button>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tree-full {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}
.tree-home {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: inherit;
  border-radius: var(--radius-sm);
}
.tree-home:hover .eyebrow {
  color: var(--accent);
}
.tree-head {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 6px 10px 14px;
  border-bottom: 1px solid var(--border-soft);
}
.tree-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 8px 24px;
}
.tnode {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px;
  border-radius: var(--radius-sm);
  text-align: left;
  color: var(--fg-2);
  line-height: 1.35;
  transition: background var(--fast), color var(--fast);
}
.tnode:hover {
  background: var(--surface-warm);
  color: var(--fg);
}
.chev {
  flex: none;
  color: var(--meta);
  transition: transform var(--fast);
}
.tnode[aria-expanded='true'] .chev {
  transform: rotate(90deg);
}
/* Same size and colour as every other contents row — the chapter list, the
   contents rail. It used to be 13px, and at the same colour a smaller face
   reads as a lighter one, so the rail looked like two different lists. */
.lbl {
  flex: 1;
  min-width: 0;
  font-size: 15.5px;
}
/* The Pāḷi half of a book name. It was 11px in a Han fallback and could not be
   read; it is a Latin serif at a real size now, and lighter than the Chinese
   rather than smaller than legibility. */
/* Two names on one line: the one the reader asked for leads, the other follows
   in the Pāḷi face. Both are the same size — the count sits on its own line
   below, so nothing here has to shrink to fit. */
.ln {
  display: block;
  line-height: 1.4;
}
.pl {
  margin-left: 6px;
  font-size: 0.94em;
  color: var(--meta);
}
/* The secondary line — a book's Pāḷi name, a division's count. Same ratio as
   the contents rail: 15.5 for the name, this for what follows it. It was 14px,
   which made the aside larger than the thing it was aside about. */
.sub {
  display: block;
  font-family: var(--sans);
  font-size: 12.5px;
  line-height: 1.45;
  color: var(--meta);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bookrow {
  position: relative;
  display: flex;
  align-items: center;
}
.bookrow .book {
  flex: 1;
  min-width: 0;
}
.toc-toggle {
  flex: none;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: var(--radius-xs);
  color: var(--meta);
}
.toc-toggle:hover {
  background: var(--surface-warm);
  color: var(--accent);
}
.is-cur {
  color: var(--accent);
  background: var(--tag-faint);
  box-shadow: inset 2px 0 0 var(--accent);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}
.is-cur:hover {
  background: var(--tag-soft);
  color: var(--accent);
}
/* The current book is marked by the tint and the left rule, not by weight: a
   rail where one row is heavier than its neighbours reads as a hierarchy
   difference, and this is a state. */
.is-cur /* Same size and colour as every other contents row — the chapter list, the
   contents rail. It used to be 13px, and at the same colour a smaller face
   reads as a lighter one, so the rail looked like two different lists. */
.lbl {
  font-weight: 400;
}
.is-cur .sub {
  color: var(--accent);
  opacity: 0.75;
}
.tgroup {
  margin-left: 12px;
  padding-left: 8px;
  border-left: 1px solid var(--border-soft);
}
.chapters {
  margin: 2px 0 6px 14px;
  padding-left: 8px;
  border-left: 1px solid var(--border-soft);
}
/* The same size as the contents rail on the right. This list used to be set at
   12px, which read as a footnote to the book title above it rather than as the
   way into the book. */
.chapter {
  display: flex;
  align-items: baseline;
  gap: 6px;
  width: 100%;
  padding: 4px 6px;
  border-radius: var(--radius-sm);
  text-align: left;
  font-size: 15.5px;
  line-height: 1.5;
  color: var(--fg-2);
  transition: background var(--fast), color var(--fast);
}
.chapter:hover {
  background: var(--surface-warm);
  color: var(--fg);
}
.chapter .cn {
  flex: none;
  font-size: 12px;
  color: var(--meta);
}
.chapter .cref {
  display: block;
  margin-top: 1px;
  font-size: 12px;
  color: var(--meta);
}
.chapter .ct {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chapter.is-cur {
  color: var(--accent);
  background: var(--tag-faint);
  box-shadow: none;
}
.chapter.is-cur .cn {
  color: var(--accent);
  opacity: 0.8;
}
.trail {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid var(--border-soft);
}
</style>
