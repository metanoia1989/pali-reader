// Job 2, in a real browser: the four modes, at 1440px and at 390px, with the
// results inline under the field and a click that lands on the right segment.
//
//   node tests/prove-search.mjs [bookId]
//
// The screenshots land in $PROOF_DIR (default /tmp/pali-proof).
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

// Set an input the way a person does, so v-model hears it.
const TYPE = (sel, v) => `(() => {
  const el = document.querySelector(${JSON.stringify(sel)})
  if (!el) return false
  const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
  set.call(el, ${JSON.stringify(v)})
  el.dispatchEvent(new Event('input', { bubbles: true }))
  return true
})()`

const chrome = await launch({ width: 1440, height: 900 })
const p = await chrome.page
try {
  // Remember the last search response, so a click can be checked against the
  // segment the server actually returned rather than against a paragraph
  // number read off the screen.
  await p.call('Page.addScriptToEvaluateOnNewDocument', {
    source: `window.__lastSearch = null
      const _f = window.fetch
      window.fetch = function (u) {
        const url = String(u)
        return _f.apply(this, arguments).then((r) => {
          if (url.includes('/search')) {
            r.clone().json().then((j) => { window.__lastSearch = { url: url, body: j } }).catch(() => {})
          }
          return r
        })
      }`,
  })

  const ROWS = `[...document.querySelectorAll('.sres-hit')].map((e) => e.textContent.trim().slice(0, 60))`

  async function openMode(label) {
    await p.eval(`document.querySelector('.smode-btn').click()`)
    await sleep(150)
    const ok = await p.eval(`(() => {
      const items = [...document.querySelectorAll('.smode-item')]
      const el = items.find((e) => e.querySelector('.smode-name').textContent === ${JSON.stringify(label)})
      if (!el) return false
      el.click()
      return true
    })()`)
    await sleep(250)
    return ok
  }

  async function search(mode, query) {
    check(`模式「${mode}」可以切换`, await openMode(mode))
    await p.eval(TYPE('.rsearch input', query))
    // Wait for the answer, not for the first thing that appears: the loading
    // state is a `.sres-state` too, and stopping at it reports "0 results" for
    // a search that has not come back yet.
    await waitFor(
      p,
      `(() => {
        const panel = document.querySelector('.rsearch-panel')
        if (!panel) return false
        if (panel.querySelector('.spin')) return false
        return panel.querySelector('.sres-hit') || panel.querySelector('.sres-state') ? true : false
      })()`,
    )
    await sleep(300)
    return p.eval(`(() => {
      const rows = ${ROWS}
      const state = document.querySelector('.sres-state')
      return {
        rows, n: rows.length,
        state: state ? state.textContent.trim() : '',
        panel: !!document.querySelector('.rsearch-panel'),
        url: location.search,
      }
    })()`)
  }

  console.log('— 1440px：四种模式 —')
  await p.goto(`${BASE}/read/${BOOK}`, 2500)
  await waitFor(p, `!!document.querySelector('.rsearch input')`)

  // Put the reader inside a chapter with something in it, so 本章 has a range.
  await p.eval(`document.querySelectorAll('.col-toc .toc-item')[9].click()`)
  await sleep(1500)
  const chapter = await p.eval(`(() => {
    const el = document.querySelector('.col-toc .toc-item.is-active')
    return el ? el.querySelector('.tn').textContent : ''
  })()`)

  const r1 = await search('本章', 'dukkha')
  console.log(`    本章（${chapter}）：${r1.n} 条  ${r1.rows[0] || r1.state}`)
  check('本章有结果，而且就在输入框下面', r1.n > 0 && r1.panel, `${r1.n} rows`)
  await p.shot(`${OUT}/search-chapter-1440.png`)

  // The click has to land on the segment the server named.
  const target = await p.eval(`window.__lastSearch && window.__lastSearch.body.hits[0] ? window.__lastSearch.body.hits[0].segment : 0`)
  await p.eval(`document.querySelectorAll('.sres-hit')[0].click()`)
  await sleep(1600)
  const landed = await p.eval(`(() => {
    const el = document.querySelector('[data-seq="${target}"]')
    if (!el) return { found: false }
    return { found: true, top: Math.round(el.getBoundingClientRect().top), url: location.search }
  })()`)
  console.log(`    点了第 1 条（第 ${target} 段）→ 落在 y=${landed.top}，URL ${landed.url}`)
  check('点击结果跳到服务端给的那一段', landed.found && landed.top > -50 && landed.top < 400,
    `seq ${target} at y=${landed.top}`)
  check('结果面板点击后收起', !(await p.eval(`!!document.querySelector('.rsearch-panel')`)))
  await p.shot(`${OUT}/search-jump-1440.png`)

  const r2 = await search('经文名', '涅槃')
  console.log(`    经文名：${r2.n} 条  ${r2.rows[0] || r2.state}`)
  check('经文名有结果，带书名与译名', r2.n > 0 && /涅槃|相应|经/.test(r2.rows.join(' ')))
  await p.shot(`${OUT}/search-title-1440.png`)

  const r3 = await search('巴利全文', 'dukkhanirodha')
  console.log(`    巴利全文：${r3.n} 条  ${r3.rows[0] || r3.state}`)
  check('巴利全文有结果，并给出「查看全部」的出口',
    r3.n > 0 && (await p.eval(`!!document.querySelector('.sres-more')`)))
  await p.shot(`${OUT}/search-pali-1440.png`)

  const r4 = await search('译文', '涅槃')
  console.log(`    译文：${r4.n} 条  ${r4.rows[0] || r4.state}`)
  check('译文有结果', r4.n > 0, `${r4.n} rows`)
  check('译文命中标出语言',
    await p.eval(`!!document.querySelector('.sres-tag.is-ref')`))
  await p.shot(`${OUT}/search-refs-1440.png`)

  const r5 = await search('译文', 'zzzqqq')
  console.log(`    没有结果时：${r5.state}`)
  check('没有结果时说明搜索范围', r5.n === 0 && r5.state.includes('只搜索本书的中英参考译文'))
  await p.shot(`${OUT}/search-none-1440.png`)

  await p.eval(TYPE('.rsearch input', ''))
  await sleep(400)
  const empty = await p.eval(`(() => {
    const h = document.querySelector('.rsearch-hint')
    return { rows: ${ROWS}.length, hint: h ? h.textContent.trim() : '', panel: !!document.querySelector('.rsearch-panel') }
  })()`)
  console.log(`    空查询：${empty.hint}`)
  check('空查询不搜也不假装搜过', empty.rows === 0 && empty.hint.includes('至少两个字'))
  await p.shot(`${OUT}/search-empty-1440.png`)

  console.log('— 390px：手机上一样能用 —')
  await p.setViewport(390, 844, true)
  await p.goto(`${BASE}/read/${BOOK}`, 3000)
  await waitFor(p, `!!document.querySelector('.rsearch input')`)
  const mob = await p.eval(`(() => {
    const row = document.querySelector('.rsearch-row').getBoundingClientRect()
    const sw = document.querySelector('.smode-btn').getBoundingClientRect()
    const inp = document.querySelector('.rsearch input').getBoundingClientRect()
    return { row: Math.round(row.width), sw: Math.round(sw.width), inp: Math.round(inp.width), left: Math.round(row.left), right: Math.round(row.right) }
  })()`)
  console.log(`    搜索框宽 ${mob.row}px（输入 ${mob.inp}px + 模式切换 ${mob.sw}px）`)
  check('手机顶栏里搜索框和模式切换都放得下',
    mob.row > 120 && mob.inp > 60 && mob.sw > 30 && mob.right <= 390,
    JSON.stringify(mob))
  await p.shot(`${OUT}/search-mobile-bar-390.png`)

  const m1 = await search('本章', 'dukkha')
  const panel = await p.eval(`(() => {
    const b = document.querySelector('.rsearch-panel')?.getBoundingClientRect()
    return b ? { w: Math.round(b.width), l: Math.round(b.left), t: Math.round(b.top), h: Math.round(b.height) } : null
  })()`)
  console.log(`    手机本章：${m1.n} 条，面板 ${panel ? panel.w + '×' + panel.h : 'none'}`)
  check('手机本章有结果', m1.n > 0)
  check('手机上面板占满整行而不是挂在字段下面', !!panel && panel.w > 300 && panel.l >= 0,
    JSON.stringify(panel))
  const fits = await p.eval(`(() => {
    const sc = document.querySelector('.sres')
    return sc ? sc.scrollHeight <= sc.clientHeight + 1 || sc.clientHeight > 100 : false
  })()`)
  check('结果列表自己滚，不把正文顶走', fits)
  await p.shot(`${OUT}/search-chapter-390.png`)

  const m2 = await search('译文', '涅槃')
  console.log(`    手机译文：${m2.n} 条`)
  check('手机译文有结果', m2.n > 0)
  await p.shot(`${OUT}/search-refs-390.png`)

  await p.setViewport(1440, 900, false)
} finally {
  await chrome.close()
}

console.log(`\nscreenshots: ${OUT}/search-*.png`)
if (fails.length) {
  console.error(`\n${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('\nsearch proof: ok')
