// One screenshot of a book that used to place a small fraction of its lines,
// now showing its Chinese and English under the Pali.
//
//   node tests/shot-reader.mjs <book> <out.png> [scrollFraction]
//
// The reader's own default is 参考译文 → 点一下才显示, so this sets the same
// setting the panel offers ('all') in localStorage before loading the page and
// then reloads — a screenshot of a reader who turned them on, not a change to
// what a new reader sees.
import { launch } from './cdp.mjs'

const [book = 'tika_vi_04', out = '/tmp/reader.png', frac = '0.18'] = process.argv.slice(2)
const base = process.env.READER_URL || 'http://localhost:8099'

const chrome = await launch({ width: 1440, height: 1180 })
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

  // Wait until the segments (and their reference rows) are actually in the DOM.
  for (let i = 0; i < 40; i++) {
    const n = await page.eval(`document.querySelectorAll('.seg-pali, .seg-heading').length`)
    if (n > 4) break
    await new Promise((r) => setTimeout(r, 500))
  }

  const info = await page.eval(`(() => {
    const cards = [...document.querySelectorAll('.seg')]
    let withRefs = 0
    for (const c of cards) if (c.querySelector('.line--ref')) withRefs++
    return { cards: cards.length, withRefs, url: location.href }
  })()`)
  console.log(JSON.stringify(info))

  // scrollIntoView rather than scrollTo: the reading column is its own
  // scroll container, so scrolling the window moves nothing.
  const at = await page.eval(`(() => {
    const cards = [...document.querySelectorAll('.seg')]
    if (!cards.length) return -1
    const i = Math.min(cards.length - 1, Math.round(cards.length * ${Number(frac)}))
    cards[i].scrollIntoView({ block: 'start' })
    return i
  })()`)
  console.log('scrolled to card', at, 'of', await page.eval(`document.querySelectorAll('.seg').length`))
  await new Promise((r) => setTimeout(r, 900))
  await page.shot(out)
  console.log('wrote', out)
} finally {
  await chrome.close()
}
