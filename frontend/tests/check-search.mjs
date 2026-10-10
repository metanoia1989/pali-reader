// The search's data rules and the contents list's arithmetic.
//
// Both are the kind of thing that is wrong quietly. A chapter range computed
// from the loaded window instead of the contents table answers "not found" to
// a passage the reader can see; a list that never scrolls shows the top of a
// book when the reader is in chapter forty; a list that scrolls on every
// update fights the reader's own scrolling.
import { chapterRange, hitWhere, MIN_QUERY, rankHits, SEARCH_MODES } from '../src/utils/search.js'
import { followScrollTop } from '../src/utils/scroll.js'

const fails = []
function check(name, cond, detail = '') {
  if (cond) return console.log('  ok    ' + name)
  console.log('  FAIL  ' + name + (detail ? `  — ${detail}` : ''))
  fails.push(name)
}

const TOC = [
  { name: '1. naḷavaggo', seq: 5, level: 3 },
  { name: '1. oghataraṇasuttaṃ', seq: 6, level: 3 },
  { name: '2. arahatasuttaṃ', seq: 30, level: 3 },
  { name: '3. sīlasuttaṃ', seq: 60, level: 3 },
]

console.log('— 四种模式 —')
check('四种模式都在', SEARCH_MODES.length === 4, `${SEARCH_MODES.length}`)
check('每种模式都有名字和说明',
  SEARCH_MODES.every((m) => m.label && m.hint && m.id))
check('模式 id 不重复', new Set(SEARCH_MODES.map((m) => m.id)).size === 4)
check('两个字起', MIN_QUERY === 2)

console.log('— 本章的范围 —')
{
  const r = chapterRange(TOC, 6)
  check('选中的一章从它自己的标题开始', r.from === 6, `from ${r.from}`)
  check('到下一个标题前一段结束', r.to === 29, `to ${r.to}`)
  check('名字就是目录里那一行', r.name === '1. oghataraṇasuttaṃ', r.name)
}
{
  const r = chapterRange(TOC, 40)
  check('读到中间也落在正确的一章', r.from === 30 && r.to === 59, `${r.from}..${r.to}`)
}
{
  const r = chapterRange(TOC, 900)
  check('最后一章没有上界', r.from === 60 && r.to === 0, `${r.from}..${r.to}`)
}
{
  const r = chapterRange(TOC, 1)
  check('标题之前没有章', r.from === 0 && r.index === -1, JSON.stringify(r))
  check('没有章的说明是空的', r.name === '')
}
{
  const r = chapterRange([], 100)
  check('没有目录时不炸', r.from === 0 && r.to === 0)
}

console.log('— 结果排序 —')
{
  const hits = [
    { bookId: 'other', segment: 1 },
    { bookId: 'mine', segment: 90 },
    { bookId: 'other', segment: 2 },
    { bookId: 'mine', segment: 10 },
  ]
  const ranked = rankHits(hits, 'mine')
  check('本书的命中排在最前',
    ranked[0].bookId === 'mine' && ranked[1].bookId === 'mine')
  check('组内保持阅读顺序', ranked[0].segment === 90 && ranked[1].segment === 10)
  check('别的书一个不少', ranked.length === 4)
  check('没有命中也不会炸', rankHits(null, 'mine').length === 0)
}

console.log('— 结果行说清楚它在哪 —')
check('正文命中带书名和段号',
  hitWhere({ bookName: '长部', para: 12 }) === '长部 · §12')
check('标题命中只说名字', hitWhere({ kind: 'heading', bookName: '相应部' }) === '相应部')
check('没有段号时不写空段号', hitWhere({ bookName: '长部' }) === '长部')

console.log('— 目录跟着读者走 —')
const box = { top: 100, height: 600, bottom: 700 }
{
  // Comfortably inside: the list must not move at all. A list that re-centres
  // on every update cannot be scrolled by hand.
  const item = { top: 300, bottom: 330 }
  check('已经在看得见的地方就不动', followScrollTop(item, box, 500) === null)
}
{
  // Above the safe area: bring it back to a third of the way down.
  const item = { top: 90, bottom: 120 }
  const next = followScrollTop(item, box, 500)
  check('跑到上边去了就拉回来', next === 500 + (90 - 100) - 200, `${next}`)
}
{
  // Below the safe area.
  const item = { top: 780, bottom: 810 }
  const next = followScrollTop(item, box, 500)
  check('掉到下边去了也拉回来', next === 500 + (780 - 100) - 200, `${next}`)
}
{
  const item = { top: 90, bottom: 120 }
  check('不会滚到负数', followScrollTop(item, box, 10) === 0)
}
{
  check('量不到元素就什么都不做', followScrollTop(null, box, 500) === null)
  check('列表还没布局就什么都不做', followScrollTop({ top: 1, bottom: 2 }, { top: 0, height: 0 }, 0) === null)
}
{
  // The case that matters on a phone: the drawer has just been created, so the
  // list is at scrollTop 0 and the active item is far below it.
  const item = { top: 100 + 2400, bottom: 100 + 2430 }
  const next = followScrollTop(item, box, 0)
  check('刚打开的抽屉会滚到当前章节', next === 2400 - 200, `${next}`)
}

console.log()
if (fails.length) {
  console.error(`${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('search checks: ok')
