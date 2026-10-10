// Drive the loaded window the way a scrolling reader drives it.
//
// This is the part of the infinite-scroll bug that no render assertion can
// reach: the reader is a contiguous run of segments with two ends, either of
// which must be able to grow, and growing the near end must put the text back
// exactly where it was. Both are arithmetic over a scroller's height, so they
// are checked here — with a fake scroller whose height is a function of the
// segments the store holds, and a fake corpus that answers /segments for any
// range.
//
// The pixel assertion is the one that matters. A prepend that is not anchored
// moves every line down by the height of what was added, and the reader sees
// "the text jumped" — which is the same bug as "it will not load any more",
// seen from the other side.
//
//   node tests/check-window.mjs
import { createPinia, setActivePinia } from 'pinia'

const store = new Map()
globalThis.localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
}

const fails = []
function check(name, cond, detail = '') {
  if (cond) return console.log('  ok    ' + name)
  console.log('  FAIL  ' + name + (detail ? `  — ${detail}` : ''))
  fails.push(name)
}

// --- a corpus, and a scroller whose height follows it ----------------------
const TOTAL = 3627
const ROW = 120 // px per segment. Uniform on purpose: this checks arithmetic, not layout.
const seg = (seq) => ({ seq, kind: 'prose', para: seq, level: 0, text: `segment ${seq}`, tokens: [] })

let segmentCalls = 0
const res = (body) => ({ ok: true, status: 200, json: async () => body, text: async () => JSON.stringify(body) })

globalThis.fetch = async (url) => {
  const u = String(url)
  if (u.includes('/marks')) return res({ bookId: 'long', picks: [], notes: [], translations: [], refs: {} })
  if (u.includes('/segments')) {
    segmentCalls++
    const q = new URL(u, 'http://x').searchParams
    const from = Number(q.get('from') || 1)
    const count = Number(q.get('count') || 60)
    const items = []
    for (let s = from; s < from + count && s <= TOTAL; s++) items.push(seg(s))
    return res({
      bookId: 'long', from, count: items.length, total: TOTAL, items,
      done: from + count - 1 >= TOTAL,
    })
  }
  if (u.includes('/api/books/')) return res({ book: { id: 'long', name: 'dīghanikāyo', nameZh: '长部' }, toc: [] })
  return res({})
}

const { useReader } = await import('../src/store/reader.js')
const { MAX_SEGMENTS, anchorShift, edgeLoads, pumpEdges, trimPlan } = await import('../src/utils/window.js')

setActivePinia(createPinia())
const R = useReader()

// The scroller. scrollHeight is derived, so it changes exactly when the DOM
// would: the fake cannot drift from what the store actually holds.
const col = {
  scrollTop: 0,
  clientHeight: 800,
  get scrollHeight() {
    return R.segments.length * ROW
  },
}
R.bindScroller(col)

// What the view asks: is either sentinel inside the margin? The sentinels sit
// at the two ends of the loaded run, so their distance from the viewport is the
// amount of text still in hand on that side.
const need = () => {
  const top = -col.scrollTop
  const bottom = col.scrollHeight - col.scrollTop
  const n = edgeLoads({
    col: { top: 0, bottom: col.clientHeight },
    top: { top, bottom: top + 1 },
    bottom: { top: bottom, bottom: bottom + 1 },
  })
  return { prev: n.prev && !R.doneStart, next: n.next && !R.doneEnd }
}
const pump = () =>
  pumpEdges({
    need,
    loadPrev: () => R.loadPrev(),
    loadMore: () => R.loadMore(),
    settle: () => Promise.resolve(),
  })

const indexOf = (seq) => R.segments.findIndex((s) => s.seq === seq)
// Where a segment sits on screen, in px from the top of the viewport. This is
// the number the reader sees; if it changes between two moments while they are
// standing still, the text moved under them.
const onScreen = (seq) => indexOf(seq) * ROW - col.scrollTop

console.log('— 打开一本书 —')
await R.open('long')
check('第一屏载入', R.segments.length === 60, `got ${R.segments.length}`)
check('书的开头已在身后', R.doneStart === true && R.doneEnd === false)

console.log('— 从目录跳到中间 —')
await R.loadAt(1800)
check('目标落在窗口里', indexOf(1800) >= 0, 'seq 1800 not loaded')
check('窗口在目标两侧都留了余地',
  indexOf(1800) > 0 && R.segments.length - 1 - indexOf(1800) > 0,
  `index ${indexOf(1800)} of ${R.segments.length}`)
check('两侧都还能长', R.doneStart === false && R.doneEnd === false)

// The reader lands with the target at the top of the column, two thirds of the
// way into the book.
let pos = 1800 * ROW
const place = () => {
  col.scrollTop = Math.max(0, pos - R.segments[0].seq * ROW)
  R.activeSegment = Math.min(TOTAL, Math.max(1, Math.round(pos / ROW)))
}
R.activeSegment = 1800
place()

const anchor = 1820 // inside the first window, below the reader's eye
const y0 = onScreen(anchor)
check('起点可测', y0 === 50 * ROW - (1800 - 1770) * ROW, `y=${y0}`)

console.log('— 向上翻：另一个方向也要能长 —')
pos -= 4000
place()
// Measured where the reader actually is when the load fires, not where they
// were before they scrolled: the promise is that the load does not move the
// text, not that scrolling does not.
const before = onScreen(anchor)
const loads = await pump()
check('向上翻触发了载入', loads >= 1, `pump loaded ${loads}`)
check('窗口头部向前长了', R.segments[0].seq < 1770, `head ${R.segments[0].seq}`)
check('窗口仍然连续', R.segments.every((s, i) => i === 0 || s.seq === R.segments[i - 1].seq + 1))
check('正文没有在读者眼皮底下移动',
  onScreen(anchor) === before,
  `seq ${anchor} was at y=${before}, now y=${onScreen(anchor)}`)

console.log('— 一路向上翻到书的开头 —')
let guard = 0
while (!R.doneStart && guard++ < 400) {
  pos = Math.max(R.segments[0].seq * ROW, pos - 4000)
  place()
  await pump()
}
check('翻到头了', R.doneStart === true, `guard ${guard}`)
check('第 1 段就在窗口里', R.segments[0].seq === 1, `head ${R.segments[0].seq}`)
check('没有把整本书塞进 DOM', R.segments.length <= MAX_SEGMENTS, `${R.segments.length} segments`)

console.log('— 再向下翻 —')
// The loop above leaves the window at the cap, so this one is counted in
// scrolling rather than in segments loaded.
for (let i = 0; i < 30; i++) {
  pos += 4000
  place()
  await pump()
}
check('向下也能长', R.segments.length >= MAX_SEGMENTS, `${R.segments.length} segments`)
check('两个方向都翻过之后窗口仍然连续',
  R.segments.every((s, i) => i === 0 || s.seq === R.segments[i - 1].seq + 1))

console.log('— 窗口有界，而且不丢读者看的那一段 —')
check('窗口不超过上限', R.segments.length <= MAX_SEGMENTS, `${R.segments.length}`)
check('读者所在的一段还在窗口里', indexOf(R.activeSegment) >= 0, `active ${R.activeSegment}`)

console.log('— 裁掉头部时正文也不动 —')
{
  // At the cap, so this load trims the head — the case where the content above
  // the reader is removed and scrollTop has to follow it down.
  const a = R.activeSegment
  const was = onScreen(a)
  const head = R.segments[0].seq
  place()
  await R.loadMore()
  check('确实裁掉了头部', R.segments[0].seq > head, `${head} -> ${R.segments[0].seq}`)
  check('裁掉头部之后正文没有移动',
    onScreen(a) === was,
    `seq ${a} was at y=${was}, now y=${onScreen(a)}`)
}

console.log('— 卡死的情况：载入之后哨兵还在边距里 —')
{
  // A load that does not push the sentinel out of the margin. Under the
  // IntersectionObserver this was the end of the road: no second transition,
  // no second load, no error, no way back. The rule asks the layout again.
  let loads = 0
  const answered = await pumpEdges({
    need: () => ({ prev: false, next: loads < 3 }),
    loadMore: async () => {
      loads++
      return true
    },
    settle: async () => {},
  })
  check('哨兵仍在边距里就继续载入', loads === 3, `${loads} loads, pump returned ${answered}`)
}
{
  let loads = 0
  await pumpEdges({
    need: () => ({ prev: false, next: loads < 1 }),
    loadMore: async () => {
      loads++
      return true
    },
    settle: async () => {},
  })
  check('已经没有更多时不再重试', loads === 1, `${loads} loads`)
}
{
  // A request that fails must stop the burst, not retry into the same failure.
  let loads = 0
  await pumpEdges({
    need: () => ({ prev: false, next: true }),
    loadMore: async () => {
      loads++
      return false
    },
    settle: async () => {},
  })
  check('请求失败就停下来，不空转', loads === 1, `${loads} loads`)
}

console.log('— 上限的算术 —')
{
  check('没有超出上限就不裁', trimPlan(400, 200, 400).head === 0)
  const f = trimPlan(460, 230, 400, 'head')
  check('向前翻时从头部裁', f.head === 60 && f.tail === 0, JSON.stringify(f))
  const b = trimPlan(460, 230, 400, 'tail')
  check('向后翻时从尾部裁', b.tail === 60 && b.head === 0, JSON.stringify(b))
  const atEnd = trimPlan(460, 459, 400, 'head')
  check('读者在末尾时不会把身后裁光', atEnd.head <= 400 - 60, JSON.stringify(atEnd))
  check('永远不会裁掉读者看的那一段', trimPlan(1000, 500, 400, 'head').head <= 500)
  check('裁的总数正好是超出的部分', (() => {
    const t = trimPlan(1000, 500, 400, 'head')
    return t.head + t.tail === 600
  })())
}
{
  check('向上补了多少像素就补回多少', anchorShift(1000, 1720) === 720)
  check('裁掉头部时往回减同样的像素', anchorShift(1720, 1000) === -720)
}

console.log()
if (fails.length) {
  console.error(`${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('window checks: ok')
