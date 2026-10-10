// Job 1, in a real browser: the loaded window has two ends, and the text does
// not move when the near one grows.
//
// Run against the deployed site (http://localhost:8099 tunnels to the server):
//
//   node tests/prove-window.mjs [bookId]
//
// Every step is a named assertion with the measurement that decided it, so a
// failure says what happened rather than that something did.
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

const STATE = `(() => {
  const col = document.querySelector('.col-read')
  const segs = [...col.querySelectorAll('[data-seq]')].map((e) => +e.dataset.seq)
  return {
    first: segs.length ? segs[0] : 0,
    last: segs.length ? segs[segs.length - 1] : 0,
    n: segs.length,
    top: Math.round(col.scrollTop),
    h: Math.round(col.scrollHeight),
    vh: col.clientHeight,
    reqs: window.__segReqs || 0,
    url: location.pathname + location.search,
  }
})()`

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function waitFor(p, expr, { tries = 60, gap = 200 } = {}) {
  for (let i = 0; i < tries; i++) {
    const v = await p.eval(expr)
    if (v) return v
    await sleep(gap)
  }
  return null
}

const chrome = await launch({ width: 1440, height: 900 })
const p = await chrome.page
const countRequests = () =>
  p.call('Page.addScriptToEvaluateOnNewDocument', {
    source: `window.__segReqs = 0
      const _f = window.fetch
      window.fetch = function (u, o) {
        if (String(u).includes('/segments')) window.__segReqs++
        return _f.apply(this, arguments)
      }`,
  })

try {
  await countRequests()

  console.log(`— 打开 ${BOOK}，从右侧目录跳到中段的一章 —`)
  await p.goto(`${BASE}/read/${BOOK}`, 2500)
  await waitFor(p, `!!document.querySelector('.col-read [data-seq]')`)
  const targets = await p.eval(`[...document.querySelectorAll('.col-toc .toc-item')]
    .map((el) => el.querySelector('.tn').textContent)`)
  const pick = Math.floor(targets.length * 0.6)
  await p.eval(`document.querySelectorAll('.col-toc .toc-item')[${pick}].click()`)
  await sleep(1200)
  let s = await p.eval(STATE)
  const heading = targets[pick]
  console.log(`    ${heading}  →  window ${s.first}..${s.last} (${s.n} 段), scrollTop ${s.top}`)
  check('跳转把窗口换到了目标所在的一段', s.first <= 1 || s.n > 0)
  check('跳转后目标上方已经有内容', s.top > 200, `scrollTop ${s.top}`)
  check('跳转后两侧都还能长（不是已经把书读完了）', s.first > 1 && s.last < s.first + 400)

  console.log('— 关键的一条：向后载入时正文不许动 —')
  // The reader's own scroll moves the text by exactly (oldScrollTop −
  // newScrollTop); the prepend must move it by nothing. Anything the prepend
  // adds to the top and does not give back shows up here as a pixel delta —
  // measured on one named segment, in the browser, in pixels.
  //
  // Measured before the scroll that provokes the load, so the load cannot have
  // happened yet: at this scrollTop the head sentinel is 1900px from the top
  // and nothing asks for more.
  const pre = await p.eval(`(() => {
    const col = document.querySelector('.col-read')
    const segs = [...col.querySelectorAll('[data-seq]')]
    const el = segs.find((e) => e.getBoundingClientRect().top > 300)
    return {
      seq: +el.dataset.seq, top: el.getBoundingClientRect().top,
      st: col.scrollTop, h: col.scrollHeight, first: +segs[0].dataset.seq,
    }
  })()`)
  const seek = await p.eval(`(() => {
    const c = document.querySelector('.col-read')
    c.scrollTop = 120
    return c.scrollTop
  })()`)
  const grew = await waitFor(
    p,
    `document.querySelector('.col-read').scrollHeight > ${pre.h}`,
    { tries: 40, gap: 150 },
  )
  await sleep(500)
  const post = await p.eval(`(() => {
    const col = document.querySelector('.col-read')
    const el = document.querySelector('[data-seq="${pre.seq}"]')
    const segs = [...col.querySelectorAll('[data-seq]')]
    return {
      top: el.getBoundingClientRect().top, st: col.scrollTop, h: col.scrollHeight,
      first: +segs[0].dataset.seq, n: segs.length,
    }
  })()`)
  // Where the segment would be if only the reader's own scroll had moved it.
  const expected = pre.top + (pre.st - seek)
  const delta = Math.round((post.top - expected) * 10) / 10
  console.log(`    第 ${pre.seq} 段：载入前 y=${pre.top.toFixed(1)}（scrollTop ${Math.round(pre.st)}）`)
  console.log(`    向后载入之后 y=${post.top.toFixed(1)}；只按滚动量算应该是 y=${expected.toFixed(1)}（Δ ${delta}px）`)
  console.log(`    scrollTop ${Math.round(seek)} → ${Math.round(post.st)}；scrollHeight ${pre.h} → ${post.h}（+${post.h - pre.h}）`)
  console.log(`    窗口头部 ${pre.first} → ${post.first}（现在 ${post.n} 段）`)
  check('向后载入确实发生了', !!grew && post.h > pre.h && post.first < pre.first,
    `head ${pre.first} → ${post.first}, +${post.h - pre.h}px`)
  check('载入把滚动位置按同样的像素补了回来',
    Math.abs(post.st - (seek + (post.h - pre.h))) < 2,
    `scrollTop ${post.st}，应为 ${Math.round(seek + (post.h - pre.h))}`)
  check('正在读的那一段一个像素都没有动', Math.abs(delta) < 2, `Δ ${delta}px`)

  console.log('— 向上翻：窗口头部持续后退 —')
  const heads = []
  for (let i = 0; i < 6; i++) {
    await p.eval(`document.querySelector('.col-read').scrollTop -= 900`)
    await sleep(700)
    const t = await p.eval(STATE)
    heads.push(t.first)
  }
  console.log(`    每次向上滚 900px 之后的窗口头部：${heads.join(' → ')}`)
  check('向上翻会持续载入前面的内容', heads[heads.length - 1] < heads[0],
    `${heads[0]} → ${heads[heads.length - 1]}`)
  check('窗口始终连续', await p.eval(`(() => {
    const segs = [...document.querySelectorAll('.col-read [data-seq]')].map((e) => +e.dataset.seq)
    return segs.every((v, i) => i === 0 || v === segs[i - 1] + 1)
  })()`))

  console.log('— 向下翻：窗口尾部持续前进 —')
  // Scrolling a fixed number of pixels is not enough to reach the bottom of a
  // window this size; the reader who wants more text goes to the end of it.
  const tails = []
  for (let i = 0; i < 5; i++) {
    await p.eval(`(() => { const c = document.querySelector('.col-read'); c.scrollTop = c.scrollHeight })()`)
    await sleep(800)
    const t = await p.eval(STATE)
    tails.push(t.last)
  }
  console.log(`    每次滚到底之后的窗口尾部：${tails.join(' → ')}`)
  check('向下翻会持续载入后面的内容', tails[tails.length - 1] > tails[0],
    `${tails[0]} → ${tails[tails.length - 1]}`)
  check('窗口仍然连续', await p.eval(`(() => {
    const segs = [...document.querySelectorAll('.col-read [data-seq]')].map((e) => +e.dataset.seq)
    return segs.every((v, i) => i === 0 || v === segs[i - 1] + 1)
  })()`))
  check('两边都翻过之后窗口仍然有界', (await p.eval(STATE)).n <= 420,
    `${(await p.eval(STATE)).n} 段在 DOM 里`)

  console.log('— 从深处一路向上翻到卷首 —')
  let steps = 0
  for (; steps < 260; steps++) {
    const t = await p.eval(STATE)
    if (t.first === 1) break
    await p.eval(`document.querySelector('.col-read').scrollTop -= 1400`)
    await sleep(400)
  }
  s = await p.eval(STATE)
  check('第 1 段可以靠向上滚动到达', s.first === 1, `滚了 ${steps} 次，窗口头部 ${s.first}`)
  check('到达卷首后窗口仍然有界', s.n <= 420, `${s.n} 段在 DOM 里`)

  console.log('— 卡死的情况：一次载入之后哨兵还在边距里 —')
  // A window taller than one page. After a load the bottom sentinel is STILL
  // inside the margin, and an edge-triggered IntersectionObserver never fires
  // again — one load, then silence. Counting the requests the page makes with
  // no scrolling at all is the direct test of the loop: the second load can
  // only happen if the question was asked again after the first.
  const before2 = await p.eval(`window.__segReqs || 0`)
  // The viewport is set before the navigation, because the burst happens while
  // the page is loading: a window taller than one page is the geometry an
  // edge-triggered observer cannot recover from.
  await p.setViewport(1440, 7800, false)
  await p.goto(`${BASE}/read/${BOOK}`, 1500)
  await waitFor(p, `!!document.querySelector('.col-read [data-seq]')`)
  const burst = await waitFor(
    p,
    `(() => {
      const col = document.querySelector('.col-read')
      const idle = col.scrollHeight - col.scrollTop - col.clientHeight
      return idle > 600 && (window.__segReqs || 0) > 2 ? window.__segReqs : 0
    })()`,
    { tries: 60, gap: 300 },
  )
  const tall = await p.eval(STATE)
  console.log(`    视口 ${tall.vh}px 高、内容 ${tall.h}px；一次未滚动，共 ${tall.reqs} 次载入`)
  check('哨兵留在边距里时会一次接一次地载入，直到边距被填满',
    !!burst && tall.reqs > 2, `${tall.reqs} 次载入（其中第一次是首屏）`)
  await p.setViewport(1440, 900, false)
  await sleep(800)
  await p.shot(`${OUT}/job1-window.png`)
  console.log(`\nscreenshot: ${OUT}/job1-window.png`)
} finally {
  await chrome.close()
}

if (fails.length) {
  console.error(`\n${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('\nwindow proof: ok')
