// Job 3, in a real browser.
//
//   node tests/prove-url-toc.mjs [bookId]
//
// 3a: the address bar follows the reader one contents cell at a time, a reload
//     lands on that cell, and scrolling does not fill the Back stack.
// 3b: the contents opens already scrolled to the chapter being read — in the
//     desktop rail AND in the phone's drawer, which are two renderings of one
//     component and were two different behaviours before.
import { launch } from './cdp.mjs'
import { mkdirSync } from 'node:fs'

const BOOK = process.argv[2] || 'tika_an_04'
const OUT = process.env.PROOF_DIR || '/tmp/pali-proof'
const BASE = process.env.PALI_BASE || 'http://localhost:8099'
mkdirSync(OUT, { recursive: true })

const fails = []
function check(name, ok, detail = '') {
  console.log(`  ${ok ? 'ok  ' : 'FAIL'}  ${name}${detail ? '  — ' + detail : ''}`)
  if (!ok) fails.push(name)
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
async function waitFor(p, expr, { tries = 50, gap = 200 } = {}) {
  for (let i = 0; i < tries; i++) {
    const v = await p.eval(expr)
    if (v) return v
    await sleep(gap)
  }
  return null
}

// How far the active contents cell is from the top of its own scrolling box.
const ACTIVE_IN_BOX = (scope) => `(() => {
  const box = document.querySelector('${scope} .tocscroll')
  const el = document.querySelector('${scope} .toc-item.is-active')
  if (!box || !el) return null
  const b = box.getBoundingClientRect(), e = el.getBoundingClientRect()
  return {
    off: Math.round(e.top - b.top), boxH: Math.round(b.height),
    name: el.querySelector('.tn').textContent, scrollTop: Math.round(box.scrollTop),
  }
})()`

const chrome = await launch({ width: 1440, height: 900 })
const p = await chrome.page
try {
  console.log('— 3a：地址栏跟着读者走 —')
  await p.goto(`${BASE}/read/${BOOK}`, 2500)
  await waitFor(p, `!!document.querySelector('.col-read [data-seq]')`)
  const start = await p.eval(`({ url: location.search, len: history.length })`)
  console.log(`    打开时：${start.url || '(无查询串)'}，history.length ${start.len}`)

  const n = await p.eval(`document.querySelectorAll('.col-toc .toc-item').length`)
  const pick = Math.floor(n * 0.55)
  await p.eval(`document.querySelectorAll('.col-toc .toc-item')[${pick}].click()`)
  await sleep(1500)
  const afterJump = await p.eval(`({ url: location.search, len: history.length,
    name: document.querySelector('.col-toc .toc-item.is-active')?.querySelector('.tn').textContent })`)
  console.log(`    点了目录第 ${pick} 条「${afterJump.name}」：${afterJump.url}，history.length ${start.len} → ${afterJump.len}`)
  check('跳到某一章之后地址栏变了', /[?&]seq=\d+/.test(afterJump.url), afterJump.url)
  check('主动跳转会留下一条历史记录（Back 能回到原处）',
    afterJump.len > start.len, `${start.len} → ${afterJump.len}`)

  const jumpedSeq = Number(/[?&]seq=(\d+)/.exec(afterJump.url)?.[1] || 0)
  const atTop = await p.eval(`(() => {
    const el = document.querySelector('[data-seq="${jumpedSeq}"]')
    return el ? Math.round(el.getBoundingClientRect().top) : null
  })()`)
  check('目录点的那一段就在屏幕上方', atTop !== null && atTop > -80 && atTop < 300, `y=${atTop}`)

  console.log('— 滚动只改地址栏，不加历史记录 —')
  const lenBefore = afterJump.len
  const seen = []
  for (let i = 0; i < 10; i++) {
    await p.eval(`(() => { const c = document.querySelector('.col-read'); c.scrollTop += 2600 })()`)
    await sleep(500)
    const s = await p.eval(`({ url: location.search, len: history.length })`)
    const seq = Number(/[?&]seq=(\d+)/.exec(s.url)?.[1] || 0)
    if (!seen.length || seen[seen.length - 1] !== seq) seen.push(seq)
  }
  const lenAfter = await p.eval(`history.length`)
  console.log(`    滚动中地址栏依次变成：${seen.join(' → ')}`)
  console.log(`    history.length：滚动前 ${lenBefore} → 滚动后 ${lenAfter}`)
  check('滚动会更新地址栏', seen.length > 1, `${seen.length} 个不同的 seq`)
  check('滚动不产生历史记录（Back 不会被几百次滚动塞满）',
    lenAfter === lenBefore, `${lenBefore} → ${lenAfter}`)

  console.log('— 刷新之后落在同一章 —')
  const before = await p.eval(`({ url: location.search,
    name: document.querySelector('.col-toc .toc-item.is-active')?.querySelector('.tn').textContent })`)
  await p.goto(`${BASE}/read/${BOOK}${before.url}`, 3000)
  await waitFor(p, `!!document.querySelector('.col-toc .toc-item.is-active')`)
  await sleep(600)
  const after = await p.eval(`({ url: location.search,
    name: document.querySelector('.col-toc .toc-item.is-active')?.querySelector('.tn').textContent,
    y: (() => {
      const seq = Number(/[?&]seq=(\\d+)/.exec(location.search)?.[1] || 0)
      const el = document.querySelector('[data-seq="' + seq + '"]')
      return el ? Math.round(el.getBoundingClientRect().top) : null
    })(),
    winTop: window.scrollY })`)
  console.log(`    刷新前 ${before.url}（${before.name}）→ 刷新后 ${after.url}（${after.name}），目录那一段在 y=${after.y}`)
  check('刷新后仍停在同一章', after.name === before.name, `${before.name} → ${after.name}`)
  check('刷新后停在地址栏说的那一段上（浏览器自己的滚动恢复没有抢走位置）',
    after.y !== null && after.y > -80 && after.y < 300, `y=${after.y}`)

  console.log('— Back 回到跳转前的章节 —')
  const beforeJump = await p.eval(`location.search`)
  await p.eval(`document.querySelectorAll('.col-toc .toc-item')[${Math.floor(n * 0.55) + 6}].click()`)
  await sleep(1500)
  const pushed = await p.eval(`({ url: location.search, len: history.length })`)
  // Not awaited across the navigation: `history.back()` tears the execution
  // context down, and a promise still waiting inside it never resolves.
  await p.eval(`(() => { setTimeout(() => history.back(), 0); return true })()`)
  await sleep(2000)
  const back = await p.eval(`({ url: location.search, len: history.length })`)
  console.log(`    ${beforeJump} → 点了另一章 ${pushed.url} → Back → ${back.url}`)
  check('主动跳转会留下一条历史记录（Back 能回到原处）',
    (await p.eval(`history.length`)) >= 0 && back.url === beforeJump,
    `Back 到了 ${back.url}，应为 ${beforeJump}`)

  console.log('— 3b：目录打开时就在读者那一章（桌面栏） —')
  const rail = await p.eval(ACTIVE_IN_BOX('.col-toc'))
  console.log(`    桌面栏：当前项距栏顶 ${rail.off}px / 栏高 ${rail.boxH}px（${rail.name}）`)
  check('桌面栏把当前章节滚进了视野',
    rail.off >= 0 && rail.off <= rail.boxH - 40, JSON.stringify(rail))
  check('只有一份标题目录在 DOM 里（桌面）',
    (await p.eval(`document.querySelectorAll('.tocscroll').length`)) === 1)
  await p.shot(`${OUT}/toc-rail-1440.png`)

  console.log('— 3b：目录打开时就在读者那一章（手机抽屉） —')
  await p.setViewport(390, 844, true)
  await p.goto(`${BASE}/read/${BOOK}`, 3000)
  await waitFor(p, `!!document.querySelector('.col-read [data-seq]')`)
  // Deep in the book: several chapters down, without touching the contents.
  for (let i = 0; i < 12; i++) {
    await p.eval(`(() => { const c = document.querySelector('.col-read'); c.scrollTop = c.scrollHeight })()`)
    await sleep(500)
  }
  await sleep(800)
  const deep = await p.eval(`({ url: location.search,
    name: document.querySelector('.col-read [data-seq]') ? 'ok' : '',
    seg: [...document.querySelectorAll('.col-read [data-seq]')]
      .map((e) => +e.dataset.seq).find((s) => document.querySelector('[data-seq="' + s + '"]').getBoundingClientRect().top > 0) })`)
  console.log(`    读到深处：地址栏 ${deep.url}，屏幕上的第一段是第 ${deep.seg} 段`)
  check('手机上没有右侧栏（它本来就是抽屉）',
    (await p.eval(`document.querySelectorAll('.col-toc').length`)) === 0)

  await p.eval(`document.querySelector('.topbar .iconbtn[title="标题目录"]').click()`)
  await waitFor(p, `!!document.querySelector('.drawer .toc-item.is-active')`)
  await sleep(700)
  const drawer = await p.eval(ACTIVE_IN_BOX('.drawer'))
  console.log(`    抽屉：当前项距抽屉顶 ${drawer.off}px / 抽屉高 ${drawer.boxH}px，scrollTop ${drawer.scrollTop}（${drawer.name}）`)
  console.log(`    抽屉里选中的是：${drawer.name}；正文显示的第 ${deep.seg} 段`)
  check('抽屉打开时已经滚到当前章节',
    drawer.off >= 0 && drawer.off <= drawer.boxH - 40, JSON.stringify(drawer))
  check('抽屉确实滚动过（不是碰巧在第一屏）', drawer.scrollTop > 0, `scrollTop ${drawer.scrollTop}`)
  check('只有一份标题目录在 DOM 里（手机）',
    (await p.eval(`document.querySelectorAll('.tocscroll').length`)) === 1)
  check('手机抽屉与桌面栏用的是同一个组件',
    (await p.eval(`!!document.querySelector('.drawer .tocscroll')`)) === true)
  await p.shot(`${OUT}/toc-drawer-390.png`)

  await p.setViewport(1440, 900, false)
} finally {
  await chrome.close()
}

console.log(`\nscreenshots: ${OUT}/toc-*.png`)
if (fails.length) {
  console.error(`\n${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('\nurl + contents proof: ok')
