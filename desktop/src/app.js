'use strict'

// Everything both entry points share: the shipping app (src/main.js) and the
// measurement harness (tools/measure.js). Keeping this in one place is what
// makes the numbers in the report numbers about the real app rather than about
// a look-alike.

const { app, BrowserWindow, Menu, session, shell } = require('electron')
const path = require('path')
const fs = require('fs')

const { resolveConfig, writeDefaultConfig, DEFAULT_API_BASE } = require('./config')
const { DesktopServer } = require('./server')

const APP_ROOT = path.resolve(__dirname, '..')

/** The staged copy wins when present so a packaged .app is self-contained. */
function resolveDistDir () {
  const candidates = [
    process.env.PALI_DIST,
    path.resolve(APP_ROOT, 'dist'),
    path.resolve(APP_ROOT, '..', 'frontend', 'dist'),
  ].filter(Boolean)
  for (const dir of candidates) {
    if (fs.existsSync(path.join(dir, 'index.html'))) return { distDir: dir, exists: true }
  }
  return { distDir: candidates[candidates.length - 1] || path.resolve(APP_ROOT, '..', 'frontend', 'dist'), exists: false }
}

/**
 * userData is where config.json, the Chromium HTTP cache and localStorage live.
 * PALI_USER_DATA exists so the app can run from a read-only or sandboxed home
 * directory (CI, this build environment) without behaving differently.
 *
 * sessionData is pinned under userData rather than left at ~/Library/Caches so
 * the HTTP cache travels with the app data — it is part of what makes the
 * second launch of a book instant. Must run before app.whenReady().
 */
function configurePaths () {
  const override = process.env.PALI_USER_DATA
  const userData = override ? path.resolve(override) : app.getPath('userData')
  try {
    fs.mkdirSync(userData, { recursive: true })
  } catch { /* handled below by the config writer's own error reporting */ }
  app.setPath('userData', userData)
  try {
    app.setPath('sessionData', path.join(userData, 'session'))
  } catch { /* older Electron: userData alone is enough */ }
  return userData
}

function makeLogger (scope) {
  const verbose = process.env.PALI_LOG === '1' || process.env.PALI_LOG === 'requests'
  return (...args) => {
    if (verbose) console.log(`[${scope}]`, ...args)
  }
}

/**
 * Boots config + the loopback server.
 *
 * `userDataDir` must come from configurePaths(), which has to run before
 * app.whenReady() — so entry points call it at module load and pass the result
 * in rather than letting bootstrap() set the path too late.
 *
 * @returns {Promise<{config:object, server:DesktopServer, userDataDir:string, distDir:string}>}
 */
async function bootstrap ({ userDataDir } = {}) {
  if (!userDataDir) userDataDir = configurePaths()
  const config = resolveConfig({ userDataDir })

  if (!config.configFileExisted) {
    const err = writeDefaultConfig(config.configPath, config.apiBase, config.cacheTtlMs)
    if (err) config.warnings.push(err)
  }

  if (process.env.PALI_API && !config.warnings.length) {
    config.warnings.push('PALI_API 覆盖了配置文件里的地址')
  }

  const { distDir, exists } = resolveDistDir()
  const log = makeLogger('pali')

  const server = new DesktopServer({
    distDir,
    apiBase: config.apiBase,
    cacheTtlMs: config.cacheTtlMs,
    configPath: config.configPath,
    configSource: config.source,
    log,
  })
  await server.start()

  console.log(
    `[desktop] shell=${server.origin} api=${config.apiBase} (${config.source}) ` +
    `dist=${distDir}${exists ? '' : ' [缺失]'} cache=${config.cacheTtlMs}ms userData=${userDataDir}`
  )
  for (const w of config.warnings) console.warn(`[desktop] ${w}`)

  return { config, server, userDataDir, distDir, distExists: exists }
}

/**
 * Nothing in this app is a browser: there is no reason to load remote code, open
 * popups, or hand out camera/microphone permissions.
 */
function installSecurity (ses, allowedOrigin) {
  const log = makeLogger('security')

  // The workstation runs a system proxy (see ~/.curlrc) that cannot resolve
  // *.internal hosts and answers 502 for them. Chromium would happily use it and
  // every API call would fail, so the app talks to the network directly.
  const proxyMode = process.env.PALI_PROXY === 'system' ? 'system' : 'direct'
  const proxyApplied = ses.setProxy(
    proxyMode === 'direct' ? { mode: 'direct' } : { mode: 'system' }
  )
  return Promise.resolve(proxyApplied)
    .then(() => {
      log(`proxy mode = ${proxyMode}`)
      if (proxyMode === 'direct') {
        ses.resolveProxy('http://pali.bigubuntu.internal/')
          .then((r) => log(`resolveProxy(pali.bigubuntu.internal) = ${r}`))
          .catch(() => {})
      }
    })
    .catch((err) => console.warn('[desktop] setProxy failed:', err.message))
    .then(() => {
      // The reader copies a word/definition with navigator.clipboard, which
      // needs this one grant; everything else is refused.
      ses.setPermissionRequestHandler((_wc, permission, callback) => {
        callback(permission === 'clipboard-sanitized-write')
      })
      ses.setPermissionCheckHandler((_wc, permission) => permission === 'clipboard-sanitized-write')
    })
}

function isAllowedNavigation (target, allowedOrigin) {
  try {
    return new URL(target).origin === allowedOrigin
  } catch {
    return false
  }
}

function createWindow ({ server, title = '巴利三藏阅读器', show = true, allowedOrigin } = {}) {
  // allowedOrigin defaults to the app's own loopback origin. tools/measure.js
  // passes the LAN origin for its "what would a browser tab cost?" baseline.
  const permitted = allowedOrigin || server.origin
  const win = new BrowserWindow({
    width: 1440,
    height: 960,
    minWidth: 960,
    minHeight: 640,
    show: false,
    title,
    backgroundColor: '#f5f4ed',
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
      // No preload: the renderer is same-origin with our own server and needs
      // nothing from Node. Leaving it out is one less surface to audit.
      spellcheck: false,
      devTools: !app.isPackaged || process.env.PALI_DEVTOOLS === '1',
    },
  })

  let shown = false
  const reveal = () => {
    if (shown || win.isDestroyed()) return
    shown = true
    if (show) win.show()
  }
  win.once('ready-to-show', reveal)
  // If the page never paints (a broken build, a hung server), still show the
  // window: an empty frame the owner can close beats an invisible process.
  const revealTimer = setTimeout(reveal, 4000)
  win.on('closed', () => clearTimeout(revealTimer))

  win.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:/i.test(url) && !isAllowedNavigation(url, server.origin)) shell.openExternal(url)
    return { action: 'deny' }
  })
  win.webContents.on('will-navigate', (event, url) => {
    if (isAllowedNavigation(url, server.origin)) return
    event.preventDefault()
    if (/^https?:/i.test(url)) shell.openExternal(url)
  })
  win.webContents.on('did-fail-load', (_e, code, desc, url) => {
    console.error(`[desktop] load failed ${code} ${desc} ${url}`)
  })

  if (process.env.PALI_DEVTOOLS === '1') win.webContents.openDevTools({ mode: 'detach' })

  return win
}

/** One listener for every webContents, including anything created later. */
function installWebContentsGuards (server) {
  app.on('web-contents-created', (_e, contents) => {
    contents.on('will-attach-webview', (event) => event.preventDefault())
  })
  app.on('web-contents-created', (_e, contents) => {
    contents.setWindowOpenHandler(({ url }) => {
      if (/^https?:/i.test(url) && !isAllowedNavigation(url, server.origin)) shell.openExternal(url)
      return { action: 'deny' }
    })
  })
}

/**
 * A deliberately small menu: the roles a Mac app is expected to have, plus
 * View → 重新载入 which is the fastest way out of a wedged renderer. No menus
 * for features that do not exist.
 */
function buildMenu () {
  const isMac = process.platform === 'darwin'
  const template = [
    ...(isMac
      ? [{
          label: app.name,
          submenu: [
            { role: 'about' },
            { type: 'separator' },
            { role: 'hide' },
            { role: 'hideOthers' },
            { role: 'unhide' },
            { type: 'separator' },
            { role: 'quit' },
          ],
        }]
      : []),
    {
      label: '编辑',
      submenu: [
        { role: 'undo', label: '撤销' },
        { role: 'redo', label: '重做' },
        { type: 'separator' },
        { role: 'cut', label: '剪切' },
        { role: 'copy', label: '拷贝' },
        { role: 'paste', label: '粘贴' },
        { role: 'selectAll', label: '全选' },
      ],
    },
    {
      label: '显示',
      submenu: [
        { role: 'reload', label: '重新载入' },
        { role: 'forceReload', label: '强制重新载入' },
        { type: 'separator' },
        { role: 'resetZoom', label: '实际大小' },
        { role: 'zoomIn', label: '放大' },
        { role: 'zoomOut', label: '缩小' },
        { type: 'separator' },
        { role: 'togglefullscreen', label: '全屏' },
      ],
    },
    {
      label: '窗口',
      submenu: [
        { role: 'minimize', label: '最小化' },
        { role: 'zoom', label: '缩放' },
        ...(isMac ? [{ type: 'separator' }, { role: 'front', label: '前置全部窗口' }] : []),
      ],
    },
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

module.exports = {
  APP_ROOT,
  DEFAULT_API_BASE,
  bootstrap,
  buildMenu,
  configurePaths,
  createWindow,
  installSecurity,
  installWebContentsGuards,
  resolveDistDir,
  makeLogger,
}
