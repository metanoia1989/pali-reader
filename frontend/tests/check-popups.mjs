// Where a popup opened from inside the dictionary panel goes, and how dragging
// moves it.
//
// Checked here rather than only in a browser because the failures are quiet: a
// popup off-screen cannot be closed or read, and one that lands somewhere
// unrelated to the click sends the eye to the wrong part of the page.
import { createPinia, setActivePinia } from 'pinia'

const fails = []
function check(name, cond, detail = '') {
  if (cond) return console.log('  ok    ' + name)
  console.log('  FAIL  ' + name + (detail ? `  — ${detail}` : ''))
  fails.push(name)
}

setActivePinia(createPinia())
const { POPUP_W, placePopup, dragTo } = await import('../src/utils/popups.js')

const desktop = { width: 1600, height: 1000 }
const laptop = { width: 1280, height: 720 }
// A link inside the left panel, which is where these popups are opened from.
const inPanel = { left: 120, right: 260, top: 300 }

console.log('— 贴着被点的那个词 —')
for (const [name, vp] of [['1600×1000', desktop], ['1280×720', laptop]]) {
  const box = placePopup(inPanel, vp)
  check(`${name}: opens to the right of the click`,
    box.x >= inPanel.right, `x=${box.x} right=${inPanel.right}`)
  check(`${name}: and clear of it, not on top`,
    box.x - inPanel.right >= 8, `gap=${box.x - inPanel.right}`)
  check(`${name}: lines up with the click`,
    box.y === inPanel.top, `y=${box.y} top=${inPanel.top}`)
  check(`${name}: fully on screen`,
    box.x >= 0 && box.x + box.w <= vp.width && box.y >= 0, JSON.stringify(box))
}

console.log('— 右边放不下就翻到左边 —')
const farRight = { left: 1350, right: 1520, top: 400 }
const flipped = placePopup(farRight, desktop)
check('flips to the left of the click',
  flipped.x + POPUP_W <= farRight.left, `x=${flipped.x} left=${farRight.left}`)
check('and is still on screen', flipped.x >= 8, `x=${flipped.x}`)

console.log('— 从浮窗里再开一个，向下错开 —')
const one = placePopup(inPanel, desktop, 0)
const two = placePopup(inPanel, desktop, 1)
const three = placePopup(inPanel, desktop, 2)
check('each steps down from the last',
  two.y > one.y && three.y > two.y, `${one.y}/${two.y}/${three.y}`)
check('and stays fully visible',
  three.y + 320 <= desktop.height, `y=${three.y}`)
check('the tenth is still on screen',
  placePopup(inPanel, laptop, 9).y >= 0)

console.log('— 靠近底边时上移而不是溢出 —')
const low = placePopup({ left: 120, right: 260, top: 980 }, desktop)
check('slides up to fit', low.y + 320 <= desktop.height, JSON.stringify(low))

console.log('— 拖动 —')
const start = placePopup(inPanel, desktop)
check('a drag moves by the delta',
  JSON.stringify(dragTo(start, { dx: -100, dy: 60 }, desktop)) ===
  JSON.stringify({ x: start.x - 100, y: start.y + 60 }))
const corner = dragTo(start, { dx: -99999, dy: -99999 }, desktop)
check('dragging past the top-left stops at the edge', corner.x === 4 && corner.y === 4, JSON.stringify(corner))
// A popup dragged fully off-screen could never be grabbed again.
const away = dragTo(start, { dx: 99999, dy: 99999 }, desktop)
check('dragging past the bottom-right leaves a corner on screen',
  away.x === desktop.width - 120 && away.y === desktop.height - 48, JSON.stringify(away))
check('a zero drag does not move it',
  JSON.stringify(dragTo(start, { dx: 0, dy: 0 }, desktop)) === JSON.stringify({ x: start.x, y: start.y }))

console.log('— 宽度 —')
check('never wider than the window allows', POPUP_W <= 400)

console.log()
if (fails.length) {
  console.error(`${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('popup checks: ok')
