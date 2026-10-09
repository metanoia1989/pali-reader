// Render the reading surfaces and assert on the markup.
//
// This is the substitute for a browser when none is reachable: it mounts the
// real components through vue/server-renderer and inspects the HTML. It has
// already caught a component that was referenced but never imported, and it is
// what proves the row order, the icon-not-character tags and the per-language
// reference rows actually appear.
//
//   node tests/check-render.mjs
import { execFileSync } from 'node:child_process'
import { readdirSync, renameSync, rmSync } from 'node:fs'

const OUT = '.ssr'
const fails = []

function build(entry, out) {
  execFileSync('npx', ['vite', 'build', '--config', 'vite.ssr.config.mjs', '--ssr', entry,
    '--outDir', out, '--logLevel', 'error'], { stdio: 'inherit' })
}
// Some checks assert internally and print their own report; this runs one and
// lets a non-zero exit fail the whole run.
function run(entry, out) {
  build(entry, `${OUT}/${out}`)
  const dir = `${OUT}/${out}`
  const js = readdirSync(dir).find((f) => f.endsWith('.js'))
  const mjs = js.replace(/\.js$/, '.mjs')
  renameSync(`${dir}/${js}`, `${dir}/${mjs}`)
  execFileSync('node', [`${dir}/${mjs}`], { stdio: 'inherit' })
}

function render(out, arg) {
  // The emitted name follows the entry, but finding it is cheaper than
  // predicting it — and it cannot silently go stale when an entry is renamed.
  const dir = `${OUT}/${out}`
  // The bundle is run more than once with different arguments, so the renamed
  // copy is reused rather than renamed again.
  let mjs = readdirSync(dir).find((f) => f.endsWith('.mjs'))
  if (!mjs) {
    const js = readdirSync(dir).find((f) => f.endsWith('.js'))
    if (!js) throw new Error(`no bundle in ${dir}`)
    mjs = js.replace(/\.js$/, '.mjs')
    renameSync(`${dir}/${js}`, `${dir}/${mjs}`)
  }
  return execFileSync('node', [`${dir}/${mjs}`, ...(arg ? [arg] : [])], {
    encoding: 'utf8', maxBuffer: 32 * 1024 * 1024,
  })
}

function check(name, cond, detail = '') {
  if (cond) return console.log('  ok    ' + name)
  console.log('  FAIL  ' + name + (detail ? `  — ${detail}` : ''))
  fails.push(name)
}
const text = (s) => s.replace(/<span class="sr-only"[^>]*>[\s\S]*?<\/span>/g, '')
  .replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()

rmSync(OUT, { recursive: true, force: true })
build('tests/render-page.mjs', `${OUT}/page`)
build('tests/render.mjs', `${OUT}/cmp`)

console.log('— 弹窗摆放与拖动 —')
run('tests/check-popups.mjs', 'popups')

console.log('— 面板点选的规则 —')
run('tests/check-picks.mjs', 'picks')

console.log('— 阅读器 store —')
run('tests/check-store.mjs', 'store')

console.log('— 阅读页（点击模式）—')
const page = render('page', 'click')
check('every segment rendered', (page.match(/data-seq="/g) || []).length >= 8)
check('verse keeps its line breaks', page.includes('<br'))
check('numbered paragraph is marked', page.includes('is-numbered'))
check('词 row with grammar and meaning chips',
  page.includes('line--picks') && page.includes('chip--grammar') && page.includes('chip--meaning'))
check('批 row', page.includes('line--note'))
check('译 row', page.includes('line--own'))
// Per segment: opened on one paragraph only, so exactly one segment carries
// 参 rows and no other does.
const refSegs = [...page.matchAll(/data-seq="(\d+)"[\s\S]*?(?=data-seq="|$)/g)]
  .filter((m) => m[0].includes('line--ref'))
  .map((m) => m[1])
check('参 rows appear on the segment that was opened', refSegs.length >= 1, `segments with 参: ${refSegs}`)
check('参 rows appear on no other segment', refSegs.length === 1, `segments with 参: ${refSegs}`)
check('the contents rail shows heading translations',
  page.includes('1. 天子相应') && page.includes('1. 芦苇品'), 'no tref in the page')
check('both languages rendered for it',
  (page.match(/lang="zh-Hans"/g) || []).length === 1 && (page.match(/lang="en"/g) || []).length === 1)
check('中 / 英 buttons offered', page.includes('>中</button>') && page.includes('>英</button>'))
check('no screen-reader label leaks as text',
  !['批注', '我的翻译', '参考译文'].some((w) => text(page).includes(w)))

// 全部 is the default, and the point of it is that the reader is not asked:
// every segment that has a translation shows it, headings included, because a
// heading is a row of the same data under the same key.
console.log('— 阅读页（全部模式，默认）—')
const inline = render('page', 'all')
const inlineSegs = [...inline.matchAll(/data-seq="(\d+)"[\s\S]*?(?=data-seq="|$)/g)]
  .filter((m) => m[0].includes('line--ref'))
  .map((m) => m[1])
check('每一段有译文的都自己显示出来，不用问', inlineSegs.length >= 2, `segments with 参: ${inlineSegs}`)
check('正文段带中文与英文两行',
  (inline.match(/lang="zh-Hans"/g) || []).length >= 2 && (inline.match(/lang="en"/g) || []).length >= 2)
check('标题也带自己的参考译文',
  /data-seq="6"[\s\S]*?line--ref[\s\S]*?1\. 渡流经/.test(inline),
  'heading 6 carries no 参 row')
// The buttons are kept in every mode but 隐藏. In 点击展开 they open a sentence;
// in 全部展开 the same button closes it, so the reader can drop one translation
// without turning the whole setting off.
check('全部模式下仍然画中／英按钮', inline.includes('>中</button>') && inline.includes('>英</button>'))
check('段落左上角有自己的一对按钮',
  inline.includes('para-tools') && /para-tools[\s\S]{0,400}>中<\/button>/.test(inline))
check('没有译文的段不凭空多出一行',
  !inlineSegs.includes('3'), `segments with 参: ${inlineSegs}`)

console.log('— 目录栏 —')
const tree = render('cmp', 'tree')
const tv = text(tree)
check('basket carries Chinese and Pāḷi', tv.includes('根本三藏') && tv.includes('Mūlasāsana'))
check('division carries Chinese and Pāḷi', tv.includes('经藏 · 相应部') && tv.includes('Saṃyuttanikāya'))
check('book carries Chinese and Pāḷi',
  tv.includes('有偈品相应') && tv.includes('sagāthāvaggasaṃyuttapāḷi'))
check('chapters listed, with none of our own numbering', tv.includes('1. devatāsaṃyuttaṃ') && !tv.includes('§'))
check('no function source leaks into the page', !tv.includes('=>'), tv.slice(0, 200))
check('every level set at one size', !tree.includes('font-size'))
// Three levels of heading translation: the reader should be able to tell what
// a Pāḷi chapter name means before travelling to it.
check('headings carry their translations in the rail',
  tv.includes('1. 天子相应') && tv.includes('1. 芦苇品') && tv.includes('1. 渡流经'),
  tv.slice(0, 160))

console.log('— 段卡片 —')
const cards = render('cmp', 'cards')
check('heading level applied', cards.includes('seg-heading') && cards.includes('lv2'))
check('line tags are icons, not characters',
  !/class="line-tag"[^>]*>\s*[词批译参]/.test(cards))
check('reference rows hidden until asked for', !cards.includes('line--ref'))
// The edition's own emphasis and the CST variant readings both survive having
// their markup stripped at import; each is restored at a recorded offset, and
// an offset that is off by one puts the mark on the wrong word.
check('commentary bold run restored',
  /<b class="ed"[^>]*>kāmayamānassā<\/b>/.test(cards), cards.match(/<b class="ed[^>]*>[^<]*/)?.[0])
check('variant reading restored in place',
  cards.includes('anubaddhā (ka. sī. pī.)') && /class="v"/.test(cards))

rmSync(OUT, { recursive: true, force: true })
if (fails.length) {
  console.error(`\n${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('\nrender checks: ok')
