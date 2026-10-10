// The English dictionary popup: where it goes, and what it draws.
//
// Checked here rather than only in a browser because the failures are quiet. A
// popup placed over the word that was tapped hides the thing the reader was
// reading; one placed off-screen cannot be read or closed; and a definition
// whose lines are not split prints the dictionary's own "n." tag in the middle
// of a sentence, which reads as a typo in the dictionary rather than in us.
import {
  EN_POPUP_MAX_H, EN_POPUP_W, EN_SHEET_BREAKPOINT, isEnSheet, placeEnPopup,
} from '../src/utils/enpopup.js'
import { expandSense, expandSenses } from '../src/utils/ensenses.js'

const fails = []
function check(name, cond, detail = '') {
  if (cond) return console.log('  ok    ' + name)
  console.log('  FAIL  ' + name + (detail ? `  — ${detail}` : ''))
  fails.push(name)
}

console.log('— 英文词义弹窗：贴在被点的那个词下面 —')
const desktop = { width: 1440, height: 900 }
// A word in the middle of a line of the English translation: the row is around
// 300px down the page and the word is 64px wide.
const word = { left: 700, right: 764, top: 300, bottom: 322 }
{
  const box = placeEnPopup(word, desktop)
  check('below the word, not over it', box.placement === 'below' && box.y >= word.bottom,
    `placement=${box.placement} y=${box.y} bottom=${word.bottom}`)
  check('and clear of it, not touching', box.y - word.bottom >= 8, `gap=${box.y - word.bottom}`)
  check('centred on the word', Math.abs(box.x + box.w / 2 - (word.left + word.right) / 2) <= 1,
    JSON.stringify(box))
  check('the caret points at the word', box.caretX > 0 && box.caretX < box.w, `caretX=${box.caretX}`)
  check('fully on screen', box.x >= 8 && box.x + box.w <= desktop.width, JSON.stringify(box))
  check('not wider than the column it explains', EN_POPUP_W <= 360)
}

console.log('— 靠近底边就翻到词的上面 —')
{
  const low = { left: 700, right: 764, top: 860, bottom: 882 }
  const box = placeEnPopup(low, desktop)
  check('flips above', box.placement === 'above', JSON.stringify(box))
  check('its bottom edge stays clear of the word', box.y <= low.top, `y=${box.y} top=${low.top}`)
  check('and it does not leave the screen', box.y - EN_POPUP_MAX_H > 0 || box.y > 0, `y=${box.y}`)
}

console.log('— 手机：底部的卡片，不需要悬停 —')
{
  const phone = { width: 390, height: 844 }
  check('390px is a phone', isEnSheet(phone.width))
  check('1440px is not', !isEnSheet(desktop.width))
  check('the breakpoint is below the reader’s own rail fold', EN_SHEET_BREAKPOINT < 1100)

  // Near the edges the card is pulled back on screen and the caret keeps
  // pointing at the word.
  const atEdge = placeEnPopup({ left: 4, right: 60, top: 400, bottom: 422 }, phone)
  check('a word at the left edge does not push it off screen', atEdge.x >= 8, JSON.stringify(atEdge))
  check('the caret still points at it', atEdge.caretX >= 18, `caretX=${atEdge.caretX}`)
  check('and the card fits the screen', atEdge.x + atEdge.w <= phone.width, JSON.stringify(atEdge))
  const atRight = placeEnPopup({ left: 350, right: 386, top: 400, bottom: 422 }, phone)
  check('a word at the right edge neither', atRight.x + atRight.w <= phone.width, JSON.stringify(atRight))

  // Without a word to point at (a keyboard-opened popup), it is still on screen.
  const noAnchor = placeEnPopup(null, phone)
  check('no anchor is still on screen',
    noAnchor.x >= 8 && noAnchor.x + noAnchor.w <= phone.width, JSON.stringify(noAnchor))
}

console.log('— 一个词条拆成一行一条义项 —')
{
  // ECDICT writes one meaning per line, each line carrying its own tag.
  const rows = expandSense({ pos: 'n.', def: '住处\n[医] 住房' })
  check('one row per line', rows.length === 2, JSON.stringify(rows))
  check('the entry’s own pos belongs to the first row',
    rows[0].pos === 'n.' && rows[0].def === '住处', JSON.stringify(rows[0]))
  check('a tag on a later line becomes its own pos column',
    rows[1].pos === '[医]' && rows[1].def === '住房', JSON.stringify(rows[1]))

  check('a single-line sense keeps its pos',
    JSON.stringify(expandSense({ pos: 'v.', def: '居住' })) ===
      JSON.stringify([{ pos: 'v.', def: '居住' }]))
  check('blank lines are not rows',
    expandSense({ pos: '', def: 'a\n\n  \nb' }).length === 2)
  check('a line that is only a tag is left whole',
    expandSense({ pos: '', def: '[计]' })[0].def === '[计]')
  check('no definition is no rows', expandSense({ pos: 'n.', def: '' }).length === 0)

  const dupes = expandSenses([
    { pos: 'n.', def: '使用' },
    { pos: 'n.', def: '使用' },
    { pos: '', def: 'n. 使用\n用途' },
  ])
  check('the same meaning is not printed twice',
    dupes.length === 2 && dupes[0].def === '使用' && dupes[1].def === '用途',
    JSON.stringify(dupes))
  check('nothing in, nothing out', expandSenses(null).length === 0)
}

console.log()
if (fails.length) {
  console.error(`${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('english dictionary checks: ok')
