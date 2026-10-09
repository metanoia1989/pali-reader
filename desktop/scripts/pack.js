#!/usr/bin/env node
'use strict'

// Assembles a double-clickable .app bundle from the Electron distribution that
// is already in node_modules — no electron-builder, no @electron/packager, no
// extra dependency.
//
//   npm run pack
//
// What it does, in the order electron-packager does it:
//   1. copies node_modules/electron/dist/Electron.app
//   2. stages ../frontend/dist into desktop/dist and puts the app source in
//      Contents/Resources/app
//   3. rewrites the bundle identity in Info.plist
//   4. ad-hoc signs it — macOS on Apple Silicon refuses to launch a bundle whose
//      signature does not match its contents, and editing Info.plist breaks it
//
// The bundle keeps Electron's internal executable names. Renaming those means
// rewriting four helper bundles too, and that is the one part worth handing to
// electron-builder if a fully branded build is ever needed; the display name,
// bundle id and dock name all come from Info.plist and are set here.

const fs = require('fs')
const path = require('path')
const { execFileSync } = require('child_process')

const APP_ROOT = path.resolve(__dirname, '..')
const REPO_ROOT = path.resolve(APP_ROOT, '..')
const ELECTRON_APP = path.join(APP_ROOT, 'node_modules', 'electron', 'dist', 'Electron.app')
const OUT_DIR = path.join(APP_ROOT, 'build')
const BUNDLE_NAME = 'Pāḷi Reader.app'

const pkg = require('../package.json')
const BUNDLE_ID = 'internal.bigubuntu.pali-reader.desktop'

function fail (msg) {
  console.error('\n' + msg + '\n')
  process.exit(1)
}

function copyDir (from, to, { skip = [] } = {}) {
  fs.cpSync(from, to, {
    recursive: true,
    dereference: false,
    filter: (src) => !skip.some((s) => path.basename(src) === s),
  })
}

const frontendDist = path.join(REPO_ROOT, 'frontend', 'dist', 'index.html')
if (!fs.existsSync(frontendDist)) fail(`找不到前端产物：${frontendDist}\n先在 frontend/ 里 npm run build。`)
if (!fs.existsSync(ELECTRON_APP)) fail(`找不到 Electron.app：${ELECTRON_APP}\n先 npm install。`)

const app = path.join(OUT_DIR, BUNDLE_NAME)
if (fs.existsSync(app)) {
  console.log(`清理上一次的 ${path.relative(APP_ROOT, app)}`)
  fs.rmSync(app, { recursive: true, force: true })
}
fs.mkdirSync(OUT_DIR, { recursive: true })

// A leftover desktop/dist would silently win over ../frontend/dist for
// `npm start` (resolveDistDir prefers it), so a rebuild would look like it had
// had no effect. Only the bundle may contain a copy of the shell.
const strayStage = path.join(APP_ROOT, 'dist')
if (fs.existsSync(strayStage)) {
  console.log(`清理会遮蔽 ../frontend/dist 的 ${path.relative(APP_ROOT, strayStage)}`)
  fs.rmSync(strayStage, { recursive: true, force: true })
}

console.log('1/4  复制 Electron.app …')
copyDir(ELECTRON_APP, app)

console.log('2/4  放入应用代码与前端产物 …')
const resources = path.join(app, 'Contents', 'Resources')
const appDir = path.join(resources, 'app')
fs.mkdirSync(appDir, { recursive: true })
// ResolveDistDir() prefers <appRoot>/dist, so the shell travels inside the
// bundle and the .app is self-contained instead of reaching back into the repo.
// Note it goes *in the bundle*, not to desktop/dist: a staged copy there would
// silently win over ../frontend/dist for `npm start` too, and then a rebuild
// would appear to have no effect.
copyDir(path.join(REPO_ROOT, 'frontend', 'dist'), path.join(appDir, 'dist'))
for (const entry of ['src', 'package.json']) {
  copyDir(path.join(APP_ROOT, entry), path.join(appDir, entry))
}
fs.rmSync(path.join(resources, 'default_app.asar'), { force: true })

console.log('3/4  改写 Info.plist …')
const plistPath = path.join(app, 'Contents', 'Info.plist')
let plist = fs.readFileSync(plistPath, 'utf8')
const setKey = (key, value) => {
  const re = new RegExp(`(<key>${key}</key>\\s*<string>)[^<]*(</string>)`)
  if (re.test(plist)) plist = plist.replace(re, `$1${value}$2`)
  else plist = plist.replace('</dict>', `\t<key>${key}</key>\n\t<string>${value}</string>\n</dict>`)
}
setKey('CFBundleName', pkg.productName)
setKey('CFBundleDisplayName', pkg.productName)
setKey('CFBundleIdentifier', BUNDLE_ID)
setKey('CFBundleShortVersionString', pkg.version)
setKey('CFBundleVersion', pkg.version)
setKey('NSHumanReadableCopyright', '')
fs.writeFileSync(plistPath, plist)

console.log('4/4  ad-hoc 签名 …')
// Not --deep: that tries to re-seal Electron's bundled frameworks and fails
// with "unsealed contents present in the root directory of an embedded
// framework" (Mantle.framework). Signing the outer bundle is enough, and
// --verify still descends into the helpers, which keep Electron's own
// signature because their bytes were never touched.
try {
  execFileSync('codesign', ['--force', '--sign', '-', app], { stdio: 'pipe' })
} catch (err) {
  fail('codesign 失败：' + (err.stderr ? err.stderr.toString() : err.message))
}
try {
  execFileSync('codesign', ['--verify', '--verbose=2', app], { stdio: 'pipe' })
  console.log('     签名校验通过')
} catch (err) {
  console.warn('     签名校验未通过（应用可能仍能启动）：' + (err.stderr ? err.stderr.toString().trim() : err.message))
}

const size = execFileSync('du', ['-sh', app]).toString().split('\t')[0]
console.log(`\n完成：${app}  (${size})`)
console.log(`启动：open ${JSON.stringify(app)}`)
console.log(`带参数启动：${JSON.stringify(path.join(app, 'Contents', 'MacOS', 'Electron'))} /read/mula_vi_01`)
console.log('\n配置仍在 ~/Library/Application Support/ 下，与 npm start 用的是同一份。')
