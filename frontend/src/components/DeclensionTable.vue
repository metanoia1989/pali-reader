<script setup>
// A DPD inflection grid.
//
// The cells hold endings, and the stem is DPD's stem without a connecting
// vowel — "dhammacakk" for dhammacakka. Drawing stem plus ending rather than
// whole words is the point: the reader sees that a whole column shares one
// ending, and that this occurrence is one slot among them. The ending is in ink
// blue so the eye can skip the shared part.
//
// The cell matching the word the reader tapped is washed, and every form is a
// link so a declension can be followed sideways.
import { computed } from 'vue'

const props = defineProps({
  decl: { type: Object, required: true },
  // 0 shows the whole grid; a positive number is a preview, and the matching
  // row is appended if it fell past the cut.
  limit: { type: Number, default: 0 },
})
const emit = defineEmits(['pick'])

const rows = computed(() => {
  const all = props.decl.rows || []
  const indexed = all.map((r, i) => ({ ...r, index: i }))
  if (!props.limit || props.limit <= 0) return indexed
  const keep = indexed.slice(0, props.limit)
  const hit = props.decl.hit
  if (hit && hit.row >= keep.length && all[hit.row]) keep.push(indexed[hit.row])
  return keep
})

const stem = computed(() => props.decl.stem || '')

function isHit(rowIndex, colIndex) {
  const h = props.decl.hit
  return !!h && h.row === rowIndex && h.column === colIndex
}

// The cell the form was matched in may hold several forms, and usually does.
// Only the one that produced the word the reader tapped is marked — washing the
// whole cell said "one of these", when the point is "this one".
function hitSuffix(colIndex) {
  const h = props.decl.hit
  return h && h.column === colIndex ? h.suffix : null
}
</script>

<template>
  <div>
    <div class="sechead">
      <span class="sectitle">变格表</span>
      <span class="secmeta">{{ decl.like || decl.pattern }}</span>
    </div>

    <div class="dwrap">
      <table class="dtable">
        <thead>
          <tr>
            <th class="dcase">格</th>
            <th v-for="(c, i) in decl.columns" :key="i">{{ c }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.index">
            <th class="dcase">{{ r.case }}</th>
            <td v-for="(cell, ci) in r.cells" :key="ci" :class="{ hitcell: isHit(r.index, ci) }">
              <template v-if="cell && cell.length">
                <span
                  v-for="(suffix, si) in cell"
                  :key="si"
                  class="tap"
                  :class="{ matched: isHit(r.index, ci) && hitSuffix(ci) === suffix }"
                  role="button"
                  tabindex="0"
                  :title="`${decl.stem}${suffix}`"
                  @click="emit('pick', decl.stem + suffix, $event)"
                  @keydown.enter.prevent="emit('pick', decl.stem + suffix, $event)"
                >
                  <span class="stem">{{ stem }}</span><b>{{ suffix }}</b>
                </span>
              </template>
              <span v-else class="none">—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <p class="caption">
      词干 <span class="pi">{{ stem }}</span> 之后的墨蓝部分是语尾；命中格已标出。点击任一形式可继续查词。
    </p>
  </div>
</template>

<style scoped>
/* A six-column adjective grid does not fit a 420px panel, so it scrolls
   sideways rather than wrapping into an unreadable block. */
.dwrap {
  overflow-x: auto;
  overscroll-behavior-x: contain;
}
.dtable {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.dtable th,
.dtable td {
  text-align: left;
  padding: 4px 6px;
  border-bottom: 1px solid var(--border-soft);
  vertical-align: top;
  white-space: nowrap;
}
.dtable thead th {
  font-size: 10.5px;
  font-weight: 600;
  letter-spacing: 0.3px;
  color: var(--dict-th-ink);
  background: var(--dict-th-bg);
  border-bottom-color: var(--border);
}
.dcase {
  width: 42px;
  font-size: 10.5px;
  font-weight: 500;
  color: var(--meta);
  letter-spacing: 0.3px;
}
.dtable td {
  font-family: var(--sans);
  font-size: var(--fs-read-sm);
  color: var(--fg-2);
}
.dtable td .stem {
  color: var(--meta);
}
/* The ending is the part being taught, so it is the part set apart. */
.dtable td b {
  font-weight: 600;
  color: var(--fg);
}
/* One form per line. Joining them with commas made every cell as wide as the
   sum of its forms, which a 400px panel cannot hold: the last column was
   always cut off. Stacked, the width is the widest single form. */
.dtable td .none {
  color: var(--border-strong);
}
/* A whisper on the cell so the eye can find the row, and the amber mark on the
   single form that matches. */
.dtable td.hitcell {
  background: var(--surface-warm);
}
.tap {
  display: block;
  cursor: pointer;
}
.tap:hover .stem {
  color: var(--fg-2);
}
.tap.matched {
  background: var(--dict-mark-bg);
  border-radius: 2px;
  box-shadow: 0 0 0 2px var(--dict-mark-bg);
}
/* The mark says WHICH form matched; it must not also flatten how the form is
   built. The stem keeps the ordinary weight and the ending keeps the weight the
   other cells give it — bolding both made the one cell the reader is studying
   the one cell that no longer shows where the stem stops. */
.tap.matched .stem {
  color: var(--dict-mark-ink);
  font-weight: 400;
}
.tap.matched b {
  color: var(--dict-mark-ink);
  font-weight: 600;
}
</style>
