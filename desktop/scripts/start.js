#!/usr/bin/env node
'use strict'

// `npm start` lands here. It builds nothing: the app always runs against the
// frontend/dist that already exists. The only job is to fail loudly and
// usefully when something is missing, instead of handing you an obscure
// Electron stack trace.

const { spawn } = require('child_process')
const fs = require('fs')
const path = require('path')

const APP_ROOT = path.resolve(__dirname, '..')
const REPO_ROOT = path.resolve(APP_ROOT, '..')

function fail (lines) {
  console.error('\n' + lines.join('\n') + '\n')
  process.exit(1)
}

const argv = process.argv.slice(2)
const measure = argv.includes('--measure')
const offlineDemo = argv.includes('--offline-demo')
const passthrough = argv.filter((a) => a !== '--measure' && a !== '--offline-demo')

// 1. The frontend must already be built. Say which directory and how to build it.
const distIndex = path.join(REPO_ROOT, 'frontend', 'dist', 'index.html')
if (!fs.existsSync(distIndex)) {
  fail([
    `找不到前端产物：${distIndex}`,
    '',
    '这个应用不负责构建前端，它只是把已经构建好的 frontend/dist 从本地磁盘端出来。',
    '先生成它，再回来启动：',
    '',
    '    cd ' + path.join(REPO_ROOT, 'frontend'),
    '    npm install',
    '    npm run build',
    '',
  ])
}

// 2. Electron's own dependency must be installed (it is a devDependency).
let electronPath
try {
  electronPath = require('electron')
} catch {
  fail([
    '没装 Electron。在 desktop/ 里执行：',
    '',
    '    cd ' + APP_ROOT,
    '    npm install',
    '',
  ])
}
if (typeof electronPath !== 'string' || !fs.existsSync(electronPath)) {
  fail([
    'Electron 的二进制不在（node_modules/electron/dist 缺失）。',
    '多半是 npm install 时二进制下载失败。重试：',
    '',
    '    cd ' + APP_ROOT,
    '    node node_modules/electron/install.js',
    '',
  ])
}

const entry = measure || offlineDemo ? path.join(APP_ROOT, 'tools', 'measure.js') : APP_ROOT
const args = [entry, ...passthrough, ...(offlineDemo ? ['--offline'] : [])]

// ELECTRON_RUN_AS_NODE turns the Electron binary into a plain Node interpreter:
// the app then dies with "Cannot read properties of undefined (reading
// 'setPath')" because require('electron') hands back the npm shim's path string
// instead of the API. Some shells and CI images export it globally, so drop it
// rather than inheriting a confusing failure.
const env = { ...process.env }
delete env.ELECTRON_RUN_AS_NODE

// Chromium's own sandbox cannot initialise inside another sandbox (seatbelt
// under seatbelt, most container images, some CI runners) — the renderer exits
// with "sandbox initialization failed: Operation not permitted". The app keeps
// sandbox:true by default; this is the documented way out for those hosts.
if (process.env.PALI_NO_SANDBOX === '1') args.unshift('--no-sandbox')

const child = spawn(electronPath, args, {
  cwd: APP_ROOT,
  stdio: 'inherit',
  env,
})
child.on('exit', (code, signal) => {
  if (signal) process.kill(process.pid, signal)
  else process.exit(code == null ? 0 : code)
})
