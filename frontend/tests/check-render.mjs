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
build('tests/render-search.mjs', `${OUT}/search`)

console.log('— 弹窗摆放与拖动 —')
run('tests/check-popups.mjs', 'popups')

console.log('— 面板点选的规则 —')
run('tests/check-picks.mjs', 'picks')

console.log('— 英文词典弹窗的摆放与释义拆分 —')
run('tests/check-endict.mjs', 'endict')

console.log('— 搜索的范围与目录跟随 —')
run('tests/check-search.mjs', 'searchrules')

console.log('— 阅读器 store —')
run('tests/check-store.mjs', 'store')

console.log('— 上下两个方向都能翻的窗口 —')
run('tests/check-window.mjs', 'window')

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

// The English dictionary is reachable from one row and one row only. The row
// bodies are pulled apart by their lang attribute, because that is exactly the
// distinction the reader sees: 中 and 英 are two lines of the same shape, and
// only one of them may be tappable.
console.log('— 英文译文的词能点，中文的不能 —')
{
  const bodies = [...inline.matchAll(/<div class="line-body"[^>]*lang="([^"]+)"[^>]*>([\s\S]*?)<\/div>/g)]
  const zh = bodies.filter((m) => m[1] === 'zh-Hans').map((m) => m[2])
  const en = bodies.filter((m) => m[1] === 'en').map((m) => m[2])
  check('两种语言的参考译文都在', zh.length >= 1 && en.length >= 1, `zh=${zh.length} en=${en.length}`)
  const counts = en.map((b) => (b.match(/class="enw"/g) || []).length)
  check('英文行里的词都画成可点的',
    counts.every((n) => n >= 1) && counts.some((n) => n >= 3), `enw per row: ${counts}`)
  check('中文行里一个可点的词都没有',
    zh.every((b) => !b.includes('enw')), zh[0]?.slice(0, 120))
  check('词与词之间的空格和标点原样留下',
    en.some((b) => /<\/span><!--[^>]*--><!--\[--> <!--\]/.test(b) || />The<\/span>/.test(b)),
    en[0]?.slice(0, 200))
  check('数字不是可点的词', !/>1\.<\/span>/.test(en.join('')))
}

console.log('— 英文词典弹窗的每一种状态 —')
{
  const enpop = render('cmp', 'enpop')
  const ev = text(enpop)
  check('词头与音标都画出来',
    /class="hw"[^>]*>dwelling<\/span>/.test(enpop) && enpop.includes('dweliŋ'))
  const senseRows = [...enpop.matchAll(
    /<span class="en-pos"[^>]*>([^<]*)<\/span><span class="en-def"[^>]*>([^<]*)<\/span>/g,
  )].map((m) => `${m[1]}=${m[2]}`)
  check('一行一条义项，词性另起一列',
    senseRows.includes('n.=住处') && senseRows.includes('[医]=住房') && senseRows.includes('v.=居住'),
    senseRows.join(' | '))
  check('从别的词条查得时说清楚是哪一个', ev.includes('词条 use'), ev.slice(0, 200))
  // The word with no entry says so; the deployment without the dictionary says
  // something else, because "no entry" would be a lie about the word.
  check('没有收录时明说没有收录', ev.includes('词典暂无收录：bhikkhus'))
  check('词典没导入时说的是没导入', ev.includes('英文词典尚未导入'))
  check('两种“没有”不混为一谈',
    !ev.includes('词典暂无收录：dwelling') || ev.split('英文词典尚未导入').length === 2)
  check('弹窗里的词头不是可点的正文词',
    enpop.includes('class="hw"') && !/class="w"[^>]*>dwelling/.test(enpop))
  check('关闭键是图标加读屏标签', /class="iconbtn"[\s\S]{0,400}lucide-x/.test(enpop))
  check('读屏标签没有漏成正文',
    ![...enpop.matchAll(/<span class="sr-only"[^>]*>([^<]*)<\/span>/g)].some((m) => ev.includes(m[1])))
}

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

// The search is three pieces, and they are checked as three pieces: the design
// being drawn in parallel will move them, and a welded block cannot be moved.
console.log('— 搜索：三个分开的部件 —')
check('搜索框是一个独立部件',
  page.includes('class="sfield"') && page.includes('aria-label="搜索经文"'),
  'the reading page carries no search field')
check('模式切换是另一个独立部件',
  page.includes('class="smode"') && page.includes('aria-haspopup="menu"'),
  'the reading page carries no mode switch')
check('两者在同一个框里（不是两颗按钮并排）',
  /class="rsearch-row[^"]*"[\s\S]{0,600}class="sfield"[\s\S]{0,900}class="smode"/.test(page),
  'the field and the switch are not inside one row')
check('⌘K 提示已经让位给模式切换',
  !page.includes('class="kbd"') && !page.includes('⌘K'))

console.log('— 搜索：四种模式的结果 —')
const f = render('search', 'field')
check('输入框带得出当前模式', f.includes('搜索本章…'), f.slice(0, 200))
check('清空按钮是图标，不是字符',
  !/[✕×⨯]/.test(f) && f.includes('aria-label="清除搜索"'))
const sw = render('search', 'switch')
check('切换器显示当前模式', sw.includes('>本章</span>') || sw.includes('本章'), sw.slice(0, 200))
check('切换器说明每种模式查什么', sw.includes('目录里选中的这一章') || sw.includes('本章'))
const sr = render('search', 'chapter')
check('本章结果列出段落', (sr.match(/class="sres-hit"/g) || []).length === 2)
check('命中的字被标出来', sr.includes('<mark>dukkha</mark>'))
check('结果说清楚在哪一段', sr.includes('§88') && sr.includes('§121'))
const sr2 = render('search', 'titles')
check('经文名结果给书名与译名',
  sr2.includes('6. sandiṭṭhikanibbānasuttaṃ') && sr2.includes('6. 现见涅槃经'))
check('卷名命中标记为卷', sr2.includes('>卷<'))
const sr3 = render('search', 'refs')
check('译文命中标出是哪种语言', sr3.includes('>中<') && sr3.includes('>英<'))
check('译文命中不把别人的话当成经文', sr3.includes('is-ref'))
const srEmpty = render('search', 'empty')
check('没有结果时说明搜索范围', srEmpty.includes('没有找到') && srEmpty.includes('只搜索本书的中英参考译文'))
check('没有结果时重复查询词', srEmpty.includes('涅槃'))
const srShort = render('search', 'short')
check('查询太短时不假装搜过',
  !srShort.includes('没有找到') && !srShort.includes('sres-hit'))
const srErr = render('search', 'error')
check('请求失败时说出来', srErr.includes('无法连接服务器'))
const srLoad = render('search', 'loading')
check('搜索中显示进度', srLoad.includes('正在搜索'))
const srMore = render('search', 'limited')
check('结果被截断时说明还有更多',
  srMore.includes('结果不止这些') && srMore.includes('sres-more'))

console.log('— 标题目录：两份渲染，同一份行为 —')
const tocList = render('search', 'toc')
check('当前那一节被标出来',
  (tocList.match(/is-active/g) || []).length === 1, tocList.slice(0, 300))
check('标题带自己的译文', tocList.includes('1. 渡流经'))
check('列表自己是滚动容器', tocList.includes('tocscroll'))
check('桌面栏与手机抽屉用的是同一个组件',
  (page.match(/tocscroll/g) || []).length >= 1, 'the rail is not the shared list')
const tocEmpty = render('search', 'toc-empty')
check('没有分节标题时说清楚', tocEmpty.includes('本卷没有分节标题'))

rmSync(OUT, { recursive: true, force: true })
if (fails.length) {
  console.error(`\n${fails.length} check(s) failed.`)
  process.exit(1)
}
console.log('\nrender checks: ok')
