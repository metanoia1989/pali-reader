// The English dictionary popup, driven in a real browser at both widths.
//
//   node tests/shot-endict.mjs [book] [outdir]
//
// Three things are being proved here that no server-side render can prove:
//
//   1. a word in the English 参考译文 opens a small card with a real definition
//      — at 1440px anchored to the word, at 390px as a card at the bottom;
//   2. the Pāḷi text still opens the Pāḷi panel, and the English card goes away
//      when it does;
//   3. the Chinese translation still opens nothing at all.
//
// The reader's default is 参考译文 → 隐藏, so this sets the same setting the
// settings panel offers ('all') in localStorage and reloads — a screenshot of a
// reader who turned the translations on, not a change to what a new reader sees.
import { mkdirSync } from 'node:fs'
import { launch } from './cdp.mjs'

const book = process.argv[2] || 'mula_an_02'
const outdir = process.argv[3] || '/tmp'
const base = process.env.READER_URL || 'http://localhost:8099'

mkdirSync(outdir, { recursive: true })
const report = {}
const fail = []
const check = (name, cond, detail = '') => {
  report[name] = cond ? 'ok' : `FAIL ${detail}`
  if (!cond) fail.push(`${name} ${detail}`)
  console.log(`${cond ? '  ok   ' : '  FAIL '} ${name}${detail && !cond ? ' — ' + detail : ''}`)
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// The popup, as the reader sees it: what is on screen, not what was fetched.
const READ_POPUP = `(() => {
  const el = document.querySelector('.en-pop')
  if (!el) return null
  const r = el.getBoundingClientRect()
  const word = el.querySelector('.hw')
  return {
    sheet: el.classList.contains('en-pop--sheet'),
    placement: el.classList.contains('en-pop--above') ? 'above'
      : el.classList.contains('en-pop--below') ? 'below' : 'sheet',
    word: word ? word.textContent.trim() : '',
    phonetic: el.querySelector('.ph')?.textContent.trim() || '',
    via: el.querySelector('.en-via')?.textContent.trim() || '',
    state: el.querySelector('.en-state')?.textContent.trim() || '',
    rows: [...el.querySelectorAll('.en-sense')].map((s) => ({
      pos: s.querySelector('.en-pos').textContent.trim(),
      def: s.querySelector('.en-def').textContent.trim(),
    })),
    rect: { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), width: Math.round(r.width) },
  }
})()`

// Click the first word of the English row whose text matches, the way a reader
// would: by its letters, not by an index into the DOM.
const clickEnWord = (text) => `(() => {
  const words = [...document.querySelectorAll('.line--ref .enw')]
  const el = ${text ? `words.find((w) => w.textContent.trim() === ${JSON.stringify(text)})` : 'words[0]'}
  if (!el) return false
  el.scrollIntoView({ block: 'center' })
  el.click()
  return true
})()`

// The card exists as soon as the word is tapped and fills in when the answer
// arrives; every reading below waits for the answer, because a screenshot of
// "正在查 …" would prove nothing about the dictionary.
async function openWord(page, text) {
  const clicked = await page.eval(clickEnWord(text))
  if (!clicked) return null
  await waitFor(page, `!!document.querySelector('.en-pop .en-sense, .en-pop .en-state')`)
  await sleep(250)
  return page.eval(READ_POPUP)
}

async function waitFor(page, expr, tries = 40) {
  for (let i = 0; i < tries; i++) {
    if (await page.eval(expr)) return true
    await sleep(250)
  }
  return false
}

const chrome = await launch({ width: 1440, height: 1000 })
const page = await chrome.page
try {
  await page.goto(`${base}/read/${book}`)
  await page.eval(`(() => {
    const s = JSON.parse(localStorage.getItem('pali.settings') || '{}')
    s.refMode = 'all'
    localStorage.setItem('pali.settings', JSON.stringify(s))
    return true
  })()`)
  await page.goto(`${base}/read/${book}`, 3500)
  await waitFor(page, `document.querySelectorAll('.line--ref .enw').length > 4`)
  report.enWords = await page.eval(`document.querySelectorAll('.line--ref .enw').length`)
  report.zhClickable = await page.eval(
    `document.querySelectorAll('.line--ref [lang="zh-Hans"] .enw, .line--ref [lang="zh-Hans"] .w').length`)

  // ---- 1440px: a word with a real definition -----------------------------
  let pop = await openWord(page, 'dwelling')
  check('点了英文译文里的 dwelling', !!pop)
  check('桌面：弹窗贴在被点的词下面且不压住那个词',
    !!pop && !pop.sheet && pop.placement === 'below', JSON.stringify(pop))
  check('桌面：弹窗里是 dwelling 的释义',
    !!pop && pop.word === 'dwelling' && pop.rows.length > 0, JSON.stringify(pop))
  report.popup1440 = pop
  await page.shot(`${outdir}/en-dict-1440.png`)

  // A word the dictionary answers from another form: the popup must say which
  // form answered, or "are → be的现在式…" looks like a mistake.
  report.inflected = await openWord(page, 'are')
  check('桌面：are 由 be 的词条回答，并写明是哪一条',
    !!report.inflected && report.inflected.word === 'be' &&
      /be/.test(report.inflected.via) && report.inflected.rows.length > 0,
    JSON.stringify(report.inflected))
  await page.shot(`${outdir}/en-dict-inflected-1440.png`)

  // ---- the Chinese translation opens nothing -----------------------------
  // Done before any Pāḷi word is tapped, so "nothing opened" is unambiguous:
  // there is no panel left over from an earlier step to mistake for an answer.
  const zhClicked = await page.eval(`(() => {
    const body = document.querySelector('.line--ref [lang="zh-Hans"]')
    if (!body) return false
    body.scrollIntoView({ block: 'center' })
    // A real tap in the middle of the Chinese text: whatever is under the
    // pointer, which is a character of the translation and nothing else.
    const r = body.getBoundingClientRect()
    const el = document.elementFromPoint(Math.round(r.left + 20), Math.round(r.top + r.height / 2))
    if (!el) return false
    el.click()
    return el.className || el.tagName
  })()`)
  await sleep(600)
  report.afterZh = {
    clicked: zhClicked,
    enPop: await page.eval(`!!document.querySelector('.en-pop')`),
    paliPanel: await page.eval(`!!document.querySelector('.dict-wrap')`),
  }
  check('点了中文译文', !!zhClicked)
  check('中文译文点不出英文弹窗', report.afterZh.enPop === false)
  check('中文译文点不出巴利词典面板', report.afterZh.paliPanel === false)
  check('点别处时英文弹窗自己收起来', report.afterZh.enPop === false)
  await page.shot(`${outdir}/en-dict-chinese-does-nothing-1440.png`)

  // ---- the Pāḷi text still opens the Pāḷi panel ---------------------------
  await page.eval(`(() => {
    const w = [...document.querySelectorAll('.seg-pali .w')].find((e) => e.textContent.trim().length > 4)
    w.scrollIntoView({ block: 'center' }); w.click(); return true
  })()`)
  await waitFor(page, `!!document.querySelector('.dict-wrap .panel')`)
  await sleep(900)
  report.paliPanel = await page.eval(`!!document.querySelector('.dict-wrap .panel')`)
  report.enPopAfterPali = await page.eval(`!!document.querySelector('.en-pop')`)
  check('点巴利语的词仍然开巴利词典面板', report.paliPanel === true)
  check('巴利面板打开时英文小弹窗被清掉', report.enPopAfterPali === false)
  await page.shot(`${outdir}/en-dict-pali-unaffected-1440.png`)

  // ---- 390px: the same thing as a card at the bottom ---------------------
  await page.setViewport(390, 844, true)
  await page.goto(`${base}/read/${book}`, 3500)
  await waitFor(page, `document.querySelectorAll('.line--ref .enw').length > 4`)
  pop = await openWord(page, 'dwelling')
  check('390px：点了英文译文里的 dwelling', !!pop)
  check('390px：弹窗是底部的卡片，不需要悬停',
    !!pop && pop.sheet && pop.rect.bottom > 700 && pop.rect.width > 300, JSON.stringify(pop))
  check('390px：里面有 dwelling 的释义',
    !!pop && pop.word === 'dwelling' && pop.rows.length > 0, JSON.stringify(pop))
  report.popup390 = pop
  await page.shot(`${outdir}/en-dict-390.png`)

  // Tapping anywhere else puts it away — the glance does not need a close
  // button to be findable on a phone.
  await page.eval(`(() => { document.querySelector('.bookhead')?.click(); return true })()`)
  await sleep(400)
  check('390px：点别处就收起来',
    (await page.eval(`!!document.querySelector('.en-pop')`)) === false)
  await page.shot(`${outdir}/en-dict-390-closed.png`)

  // And a word with no entry says so rather than opening an empty box. The
  // Pāḷi the translations keep is the honest case for this: the English
  // dictionary has never heard of it.
  const miss = await page.eval(`(() => {
    const w = [...document.querySelectorAll('.line--ref .enw')]
      .find((e) => /^(bhikkhus|dhamma|jhāna|nibbāna|suttas)$/i.test(e.textContent.trim()))
    return w ? w.textContent.trim() : null
  })()`)
  if (miss) {
    report.missing = { word: miss, popup: await openWord(page, miss) }
    check('没有词条时明说没有词条',
      /词典暂无收录/.test(report.missing.popup?.state || ''), JSON.stringify(report.missing))
    await page.shot(`${outdir}/en-dict-390-no-entry.png`)
  } else {
    report.missing = 'no Pāḷi loanword on the loaded page'
    console.log('  ..   页面上没有巴利语借词，跳过“无词条”这一张')
  }
} finally {
  await chrome.close()
}

console.log('\n' + JSON.stringify(report, null, 2))
if (fail.length) {
  console.error(`\n${fail.length} check(s) failed.`)
  process.exit(1)
}
console.log('\nenglish dictionary browser checks: ok')
