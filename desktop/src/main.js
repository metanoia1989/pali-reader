'use strict'

// The shipping entry point. Everything substantial lives in app.js so that
// tools/measure.js can drive exactly the same code path.

const { app, BrowserWindow, dialog, session } = require('electron')

const { bootstrap, buildMenu, configurePaths, createWindow, installSecurity, installWebContentsGuards, resolveDistDir } = require('./app')

// Must happen before the app is ready: userData decides where config.json and
// the Chromium HTTP cache live.
const userDataDir = configurePaths()

// A second instance would start a second loopback server and a second window
// against the same corpus, which is only ever confusing. Focus the first one.
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
} else {
  let mainWindow = null
  let server = null

  app.on('second-instance', () => {
    if (!mainWindow) return
    if (mainWindow.isMinimized()) mainWindow.restore()
    mainWindow.focus()
  })

  app.whenReady().then(async () => {
    buildMenu()

    const { distDir, exists } = resolveDistDir()
    if (!exists) {
      // Failing obscurely here is the difference between "the app is broken"
      // and "you forgot to build the frontend".
      dialog.showErrorBox(
        '找不到前端产物',
        `期望的位置：${distDir}\n\n先在 pali-reader/frontend 执行 npm run build 生成 dist，再启动桌面应用。`
      )
    }

    const boot = await bootstrap({ userDataDir })
    server = boot.server

    await installSecurity(session.defaultSession, server.origin)
    installWebContentsGuards(server)

    mainWindow = createWindow({ server })
    mainWindow.on('closed', () => { mainWindow = null })

    // Ask the backend once before painting anything, so "the LAN is down" is a
    // readable page with the configured address on it instead of a reader that
    // renders an empty catalogue.
    const probe = await server.probeUpstream(3500)
    console.log(`[desktop] backend probe: ${probe.ok ? 'ok' : 'unreachable'} (${probe.ms}ms) ${probe.error || ''}`)

    // `npm start -- /read/mula_vi_01` opens straight into a book. Purely a
    // shortcut: with no argument the app opens the home screen as usual.
    //
    // process.argv still contains Chromium switches and the app directory, and
    // a macOS path passes a naive "starts with /" test — an earlier version
    // happily tried to open /Users/…/desktop as a route. Take the arguments that
    // come after the app path and require them to look like a route.
    const argv = process.argv.slice(1)
    const appPathIndex = argv.indexOf(app.getAppPath())
    const positional = appPathIndex >= 0 ? argv.slice(appPathIndex + 1) : argv.filter((a) => !a.startsWith('-'))
    const routeArg = positional.find((a) => /^\/[A-Za-z0-9][A-Za-z0-9/_-]*$/.test(a))
    const startPath = probe.ok ? (routeArg || '/') : '/__desktop/offline'
    if (routeArg) console.log(`[desktop] opening ${routeArg}`)

    try {
      await mainWindow.loadURL(server.url(startPath))
    } catch (err) {
      console.error('[desktop] loadURL failed:', err.message)
    }

    // Free win: the catalogue is on every screen and is immutable, so fetch it
    // while the owner is still reading the first paragraph.
    if (probe.ok) server.warm(['/api/catalog'])

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) {
        mainWindow = createWindow({ server })
        mainWindow.on('closed', () => { mainWindow = null })
        mainWindow.loadURL(server.url('/')).catch(() => {})
      }
    })
  }).catch((err) => {
    console.error('[desktop] startup failed:', err)
    dialog.showErrorBox('启动失败', String(err && err.stack ? err.stack : err))
    app.quit()
  })

  app.on('window-all-closed', () => {
    // macOS convention: the app stays in the dock and `activate` reopens it.
    // The loopback server is cheap and holding it keeps the warm cache alive.
    if (process.platform !== 'darwin') app.quit()
  })

  app.on('before-quit', () => {
    if (server) {
      const stats = server.cache.stats()
      console.log(`[desktop] cache: ${stats.hits} hits / ${stats.misses} misses, ${stats.entries} entries, ${Math.round(stats.bytes / 1024)}KB`)
      server.close().catch(() => {})
    }
  })
}
