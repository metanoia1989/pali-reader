'use strict'

// Measurement + screenshot harness. Run with `npm run measure` (or
// `npm run offline-demo`), or directly: `electron tools/measure.js`.
//
// It deliberately goes through src/app.js — the same bootstrap, the same
// security policy, the same BrowserWindow options as `npm start` — so the
// numbers and the screenshots describe the real application, not a mock.
//
// The baseline it compares against is a second window of the same Electron
// binary loading the *deployed* site over the LAN. Same engine, same machine,
// same code: the only difference is where the shell comes from.

const { app, session } = require('electron')
const http = require('http')
const crypto = require('crypto')
const fs = require('fs')
const path = require('path')

const ARGS = process.argv.slice(2)
const OFFLINE = ARGS.includes('--offline')
const OUT_DIR = path.resolve(__dirname, '..', 'proof')
const REPORT = path.join(OUT_DIR, OFFLINE ? 'measure-offline.json' : 'measure.json')

// "Point it at a dead port": loopback discard port, refuses instantly, so the
// run stays fast and deterministic. Must be set before bootstrap() reads config.
if (OFFLINE) process.env.PALI_API = process.env.PALI_API || 'http://127.0.0.1:9'

const { bootstrap, configurePaths, createWindow, installSecurity, installWebContentsGuards, buildMenu } = require('../src/app')

const userDataDir = configurePaths()
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const sha = (buf) => crypto.createHash('sha256').update(buf).digest('hex')

function get (url, { agent } = {}) {
  return new Promise((resolve, reject) => {
    const started = process.hrtime.bigint()
    const req = http.get(url, { agent, headers: { accept: '*/*', 'accept-encoding': 'identity' } }, (res) => {
      const chunks = []
      res.on('data', (c) => chunks.push(c))
      res.on('end', () => {
        const body = Buffer.concat(chunks)
        resolve({
          url,
          status: res.statusCode,
          headers: res.headers,
          body,
          text: body.toString('utf8'),
          bytes: body.length,
          cache: res.headers['x-pali-cache'] || '',
          ms: Number(process.hrtime.bigint() - started) / 1e6,
        })
      })
    })
    req.setTimeout(20000, () => req.destroy(new Error('timeout')))
    req.on('error', reject)
  })
}

async function series (url, { agent, times = 3 } = {}) {
  const runs = []
  for (let i = 0; i < times; i++) runs.push(await get(url, { agent }))
  return runs
}

const median = (rs) => Number([...rs].sort((a, b) => a.ms - b.ms)[Math.floor(rs.length / 2)].ms.toFixed(2))

/** Asset URLs referenced by an index.html, as same-origin paths. */
function assetPaths (html) {
  const out = []
  const re = /(?:src|href)="(\/assets\/[^"]+)"/g
  let m
  while ((m = re.exec(html)) !== null) out.push(m[1])
  return out
}

/**
 * Loads `target` and reports two in-page milestones, both measured from just
 * before navigation:
 *   shellMs   — #app has children (the SPA mounted)
 *   contentMs — the first .seg[data-seq] exists (Pali text on screen)
 * A MutationObserver installed at dom-ready supplies the in-page timestamps; a
 * 30ms main-process poll is the fallback so a missed observer cannot hang.
 */
async function timeLoad (win, target, { contentSelector = '.seg[data-seq]', timeoutMs = 45000 } = {}) {
  const wc = win.webContents
  const t0 = Date.now()
  let domReadyMs = null

  const armed = new Promise((resolve) => {
    wc.once('dom-ready', async () => {
      domReadyMs = Date.now() - t0
      try {
        await wc.executeJavaScript(`
          window.__probe = new Promise((resolve) => {
            const seen = { shell: null, content: null }
            const done = () => seen.shell !== null && seen.content !== null
            const tick = () => {
              if (seen.shell === null) {
                const el = document.querySelector('#app')
                if (el && el.children.length > 0) seen.shell = Math.round(performance.now())
              }
              if (seen.content === null && document.querySelector(${JSON.stringify(contentSelector)})) {
                seen.content = Math.round(performance.now())
              }
              if (done()) { mo.disconnect(); resolve(seen) }
            }
            const mo = new MutationObserver(tick)
            mo.observe(document.documentElement, { childList: true, subtree: true })
            tick()
            setTimeout(() => { mo.disconnect(); resolve(seen) }, ${timeoutMs})
          })
        `, true)
      } catch { /* navigation replaced the context */ }
    })
    const poll = setInterval(async () => {
      try {
        const n = await wc.executeJavaScript(`document.querySelectorAll(${JSON.stringify(contentSelector)}).length`, true)
        if (n > 0) { clearInterval(poll); resolve(Date.now()) }
      } catch { /* mid-navigation */ }
      if (Date.now() - t0 > timeoutMs) { clearInterval(poll); resolve(0) }
    }, 30)
  })

  await wc.loadURL(target)
  const fallbackAt = await armed
  const seen = await wc.executeJavaScript('window.__probe || null', true).catch(() => null)
  const segments = await wc.executeJavaScript("document.querySelectorAll('.seg[data-seq]').length", true).catch(() => 0)
  return {
    target,
    domReadyMs,
    shellMs: seen ? seen.shell : null,
    contentMs: seen && seen.content !== null ? seen.content : (fallbackAt ? fallbackAt - t0 : null),
    segments,
  }
}

async function shoot (win, name) {
  const image = await win.webContents.capturePage()
  const file = path.join(OUT_DIR, name)
  fs.writeFileSync(file, image.toPNG())
  const size = image.getSize()
  return { file, width: size.width, height: size.height, bytes: fs.statSync(file).size }
}

async function runOffline (report, server, win) {
  report.note = '后端指向死端口 http://127.0.0.1:9，验证的是错误状态而不是白屏'
  await win.webContents.loadURL(server.url('/__desktop/offline'))
  await sleep(1500)
  report.errorPage = {
    url: win.webContents.getURL(),
    title: await win.webContents.executeJavaScript('document.title'),
    heading: await win.webContents.executeJavaScript("(document.querySelector('h1')||{}).textContent || ''"),
    apiShown: await win.webContents.executeJavaScript("(document.querySelector('dd')||{}).textContent || ''"),
    hasRetry: await win.webContents.executeJavaScript("!!document.getElementById('retry')"),
    bodyText: (await win.webContents.executeJavaScript('document.body.innerText')).slice(0, 700),
    blank: (await win.webContents.executeJavaScript('document.body.innerText.trim().length')) < 20,
  }
  report.shots = [await shoot(win, 'offline.png')]

  // The other half: the shell itself loads fine (it is on disk), only the API is
  // missing, and inject.js says so in a banner.
  await win.webContents.loadURL(server.url('/'))
  await sleep(7000)
  report.offlineBanner = {
    present: await win.webContents.executeJavaScript("!!document.getElementById('pali-desktop-offline-banner')"),
    text: await win.webContents.executeJavaScript("(document.getElementById('pali-desktop-offline-banner')||{}).innerText || ''"),
    appMounted: await win.webContents.executeJavaScript("!!document.querySelector('#app') && document.querySelector('#app').children.length > 0"),
    appBlank: await win.webContents.executeJavaScript("document.querySelector('#app') ? document.querySelector('#app').children.length === 0 : true"),
  }
  report.shots.push(await shoot(win, 'offline-banner.png'))
}

async function main () {
  fs.mkdirSync(OUT_DIR, { recursive: true })
  await app.whenReady()
  buildMenu()

  const report = {
    when: new Date().toISOString(),
    offline: OFFLINE,
    platform: process.platform,
    arch: process.arch,
    electron: process.versions.electron,
    chrome: process.versions.chrome,
    switches: process.argv.filter((a) => a.startsWith('--')),
  }
  const boot = await bootstrap({ userDataDir })
  const { server, config, distDir } = boot

  report.apiBase = config.apiBase
  report.apiBaseSource = config.source
  report.configPath = config.configPath
  report.cacheTtlMs = config.cacheTtlMs
  report.distDir = distDir
  report.shellOrigin = server.origin
  report.userData = userDataDir

  await installSecurity(session.defaultSession, server.origin)
  installWebContentsGuards(server)

  const win = createWindow({ server })
  win.setPosition(60, 60)
  report.probe = await server.probeUpstream(3500)

  if (OFFLINE) {
    await runOffline(report, server, win)
    fs.writeFileSync(REPORT, JSON.stringify(report, null, 2) + '\n', 'utf8')
    console.log(JSON.stringify(report, null, 2))
    console.log(`\nwrote ${REPORT}`)
    await server.close()
    app.exit(0)
    return
  }

  const lanOrigin = new URL(config.apiBase).origin

  // --- shell: disk vs the deployed site over the LAN ------------------------
  //
  // frontend/ may be rebuilt and redeployed by someone else while this runs, so
  // do not assume the two origins agree just because the paths came from one
  // build. Fetch the same paths from both and require the byte counts to match
  // before calling the comparison apples-to-apples; retry once if a deploy
  // landed mid-measurement.
  async function shellComparison () {
    const localIndex = await get(server.origin + '/')
    const lanIndex = await get(lanOrigin + '/')
    const paths = assetPaths(localIndex.text)
    const lanAgent = new http.Agent({ keepAlive: true })
    const localAgent = new http.Agent({ keepAlive: true })
    const lanFiles = [lanIndex]
    for (const p of paths) lanFiles.push(await get(lanOrigin + p, { agent: lanAgent }))
    const localFiles = [localIndex]
    for (const p of paths) localFiles.push(await get(server.origin + p, { agent: localAgent }))
    lanAgent.destroy()
    localAgent.destroy()

    const files = paths.map((p, i) => ({
      path: p,
      lanMs: Number(lanFiles[i + 1].ms.toFixed(2)),
      localMs: Number(localFiles[i + 1].ms.toFixed(2)),
      lanBytes: lanFiles[i + 1].bytes,
      localBytes: localFiles[i + 1].bytes,
      sameBytes: lanFiles[i + 1].bytes === localFiles[i + 1].bytes,
    }))
    const tot = (rs) => Number(rs.reduce((a, r) => a + r.ms, 0).toFixed(2))
    const bytes = (rs) => rs.reduce((a, r) => a + r.bytes, 0)
    const onDisk = fs.readFileSync(path.join(distDir, 'index.html'))
    return {
      note: '同一个已构建产物的 4 个关键文件，分别从局域网与本机磁盘取；字节数逐项核对',
      comparable: files.every((f) => f.sameBytes) && onDisk.equals(lanIndex.body),
      identicalIndexHtml: sha(onDisk) === sha(lanIndex.body),
      indexSha256: sha(onDisk),
      servedShellBytes: localIndex.bytes,
      assetCount: paths.length,
      lan: { totalMs: tot(lanFiles), bytes: bytes(lanFiles) },
      local: { totalMs: tot(localFiles), bytes: bytes(localFiles) },
      files,
    }
  }

  report.shell = await shellComparison()
  if (!report.shell.comparable) {
    // Someone deployed between the two halves of the comparison; take the
    // numbers again rather than reporting a difference that is not there.
    report.shellRetriedForConcurrentDeploy = report.shell.files.map((f) => f.path)
    await sleep(1500)
    report.shell = await shellComparison()
  }

  // --- API: upstream vs our proxy, cold and warm ---------------------------
  const apiPaths = {
    catalog: '/api/catalog',
    book: '/api/books/mula_vi_01',
    segments: '/api/books/mula_vi_01/segments?from=1&count=60',
  }
  const upAgent = new http.Agent({ keepAlive: true })
  const upstream = {}
  for (const [k, p] of Object.entries(apiPaths)) upstream[k] = await series(lanOrigin + p, { agent: upAgent })
  upAgent.destroy()

  server.cache.clear()
  server.cache.hits = 0
  server.cache.misses = 0
  const pxAgent = new http.Agent({ keepAlive: true })
  const proxied = {}
  for (const [k, p] of Object.entries(apiPaths)) proxied[k] = await series(server.origin + p, { agent: pxAgent })
  pxAgent.destroy()

  report.api = {}
  for (const k of Object.keys(apiPaths)) {
    const warm = median(proxied[k].slice(1))
    report.api[k] = {
      path: apiPaths[k],
      bytes: proxied[k][0].bytes,
      upstreamMs: median(upstream[k]),
      proxyColdMs: Number(proxied[k][0].ms.toFixed(2)),
      proxyWarmMs: warm,
      modes: proxied[k].map((r) => r.cache),
      speedup: Number((median(upstream[k]) / warm).toFixed(1)),
    }
  }

  // --- windows: browser tab vs the app -------------------------------------
  //
  // Two protocols, because they answer different questions and have very
  // different noise floors:
  //
  //  freshWindow — a brand-new BrowserWindow per sample. This is "first paint
  //    after launching", and its cost is dominated by spawning a Chromium
  //    renderer, which is identical for both sides and swamps the difference.
  //    Reported with its spread so it is not mistaken for a precise number.
  //
  //  revisit — one window, navigate to Home and back to the book, N times.
  //    This is "reopening a book you already visited", the renderer is warm on
  //    both sides, and what is left is exactly the thing being compared: where
  //    the shell comes from and whether the API is on the wire.
  //
  // Chromium's own HTTP cache is allowed to work throughout, exactly as it
  // would across a restart of the app.
  const bookId = 'mula_vi_01'
  const stat = (rs) => {
    const xs = rs.map((r) => r.contentMs).sort((a, b) => a - b)
    return {
      samples: rs.map((r) => r.contentMs),
      min: xs[0],
      median: xs[Math.floor(xs.length / 2)],
      max: xs[xs.length - 1],
      mean: Math.round(xs.reduce((a, b) => a + b, 0) / xs.length),
    }
  }

  async function probeNewWindow (target, { allowedOrigin, y = 60 } = {}) {
    const w = createWindow({ server, allowedOrigin })
    w.setPosition(60, y)
    const result = await timeLoad(w, target)
    w.close()
    return result
  }

  const FRESH = 3
  const fresh = { browserTab: [], appCold: [] }
  for (let i = 0; i < FRESH; i++) {
    fresh.browserTab.push(await probeNewWindow(`${lanOrigin}/read/${bookId}`, { allowedOrigin: lanOrigin }))
    server.cache.clear()
    fresh.appCold.push(await probeNewWindow(server.url(`/read/${bookId}`)))
  }

  // The worst realistic browser case: a profile that has never been to the site,
  // so Chromium's HTTP cache is empty and every asset is a fresh download. The
  // app cannot be deprived of the equivalent (its shell is on disk by
  // definition), which is the whole point of the comparison.
  const browserTabFreshProfile = []
  for (let i = 0; i < FRESH; i++) {
    await session.defaultSession.clearCache()
    browserTabFreshProfile.push(await probeNewWindow(`${lanOrigin}/read/${bookId}`, { allowedOrigin: lanOrigin }))
  }

  const REVISITS = 5
  async function revisit (target, { home, clearCache = false } = {}) {
    const w = createWindow({ server, allowedOrigin: new URL(target).origin })
    w.setPosition(70, 70)
    const out = []
    for (let i = 0; i < REVISITS; i++) {
      await w.webContents.loadURL(home)
      await sleep(500)
      if (clearCache) server.cache.clear()
      out.push(await timeLoad(w, target))
    }
    w.close()
    return out
  }

  const revisited = {
    browserTab: await revisit(`${lanOrigin}/read/${bookId}`, { home: `${lanOrigin}/` }),
    appReopen: await revisit(server.url(`/read/${bookId}`), { home: server.url('/') }),
    appReopenCacheCold: await revisit(server.url(`/read/${bookId}`), { home: server.url('/'), clearCache: true }),
  }

  report.window = {
    note: '时间从 loadURL 之前起算，到页内第一个 .seg[data-seq] 出现；单位毫秒',
    freshWindow: {
      note: `每次样本新建一个 BrowserWindow（含渲染进程启动），各 ${FRESH} 次`,
      browserTab: stat(fresh.browserTab),
      browserTabEmptyHttpCache: stat(browserTabFreshProfile),
      appCold: stat(fresh.appCold),
    },
    revisit: {
      note: `同一个窗口，往返首页再回到同一本书，各 ${REVISITS} 次；渲染进程已热`,
      browserTab: stat(revisited.browserTab),
      appReopen: stat(revisited.appReopen),
      appReopenCacheCold: stat(revisited.appReopenCacheCold),
    },
  }
  report.window.revisit.speedup = Number(
    (report.window.revisit.browserTab.median / report.window.revisit.appReopen.median).toFixed(2)
  )
  report.window.revisit.cacheValue = Number(
    (report.window.revisit.appReopenCacheCold.median / report.window.revisit.appReopen.median).toFixed(2)
  )

  // --- hero screenshots from a normal, visible window ----------------------
  await timeLoad(win, server.url(`/read/${bookId}`))
  await sleep(1200)
  await shoot(win, 'reader-cold.png')
  await timeLoad(win, server.url('/read/mula_di_01'))
  await sleep(900)
  await shoot(win, 'reader-second-book.png')
  await timeLoad(win, server.url(`/read/${bookId}`))
  await sleep(900)

  report.cacheAfter = server.cache.stats()
  report.counters = server.counters
  report.domEvidence = await win.webContents.executeJavaScript(`(() => ({
    segments: document.querySelectorAll('.seg[data-seq]').length,
    paliLines: document.querySelectorAll('.seg-pali').length,
    bookTitle: (document.querySelector('.booktitle') || {}).textContent || '',
    tocItems: document.querySelectorAll('.toc-item').length,
    firstPali: (document.querySelector('.seg-pali') || {}).innerText || '',
    clickableWords: document.querySelectorAll('.seg-pali .w').length,
  }))()`)
  report.domEvidence.sample = await win.webContents.executeJavaScript(
    "Array.from(document.querySelectorAll('.seg-pali')).slice(0, 3).map(e => e.innerText).join('\\n---\\n')"
  )
  // Diacritics are letters, not typography (repo rule 1): show that what is on
  // screen really is Pali orthography rather than a flat Latin transliteration.
  report.domEvidence.diacriticsSeen = Array.from(new Set(
    (report.domEvidence.sample.match(/[āīūṭḍṇṃḷĀĪŪṬḌṆṂḶ]/g) || [])
  )).join('')
  report.shots = [await shoot(win, 'reader-proof.png')]


  fs.writeFileSync(REPORT, JSON.stringify(report, null, 2) + '\n', 'utf8')
  console.log(JSON.stringify(report, null, 2))
  console.log(`\nwrote ${REPORT}`)

  await server.close()
  app.exit(0)
}

main().catch((err) => {
  console.error('measure failed:', err)
  app.exit(1)
})
