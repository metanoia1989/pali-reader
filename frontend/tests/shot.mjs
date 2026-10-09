// Screenshots of the reading column, for the report.
//
// The point of these is that a translation sits directly under the sentence it
// renders and that a heading carries its own — so the shots are taken at the
// top of two books, one of them a book whose alignment used to be the worst in
// the corpus (tika_vi_04 was placing 5% of its lines and stacking the rest).
//
//   node tests/shot.mjs
import { launch } from './cdp.mjs'

const BASE = process.env.PALI_URL || 'http://localhost:8099'
const c = await launch({ width: 1280, height: 1400 })
const p = await c.page

async function look(path, file, waitMs = 2200) {
  await p.goto(`${BASE}${path}`, waitMs)
  const info = await p.eval(`(() => {
    const segs = [...document.querySelectorAll('[data-seq]')]
    const rows = segs.map((s) => ({
      seq: +s.dataset.seq,
      head: s.classList.contains('seg-heading'),
      pali: (s.querySelector('.seg-pali, .seg-heading')?.innerText || '').slice(0, 60),
      refs: [...s.querySelectorAll('.line--ref .line-body')].map((r) => r.innerText.slice(0, 70)),
    }))
    return {
      segments: rows.length,
      withRef: rows.filter((r) => r.refs.length).length,
      firstRefs: rows.filter((r) => r.refs.length).slice(0, 4),
      headingWithRef: rows.find((r) => r.head && r.refs.length) || null,
    }
  })()`)
  await p.shot(file)
  return info
}

const a = await look('/read/mula_sa_01', '/tmp/shot-mula_sa_01.png')
console.log('mula_sa_01:', JSON.stringify(a, null, 1))

const b = await look('/read/tika_vi_04', '/tmp/shot-tika_vi_04.png')
console.log('tika_vi_04:', JSON.stringify(b, null, 1))

// And the same sentence with the translations switched off in 阅读设置, to show
// the setting still does what it says.
await p.eval(`(() => { localStorage.setItem('pali.settings', JSON.stringify({ ...JSON.parse(localStorage.getItem('pali.settings') || '{}'), refMode: 'off' })); return true })()`)
await p.goto(`${BASE}/read/mula_sa_01`, 2200)
const off = await p.eval(`document.querySelectorAll('.line--ref').length`)
await p.shot('/tmp/shot-mula_sa_01-refoff.png')
console.log('refs drawn with refMode=off:', off)

await c.close()
