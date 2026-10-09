#!/usr/bin/env node
'use strict'

// Node-only checks for the piece that is easiest to get subtly wrong: the
// server's routing, headers and cache rules. No Electron, no window — run with
// `npm run smoke`. It talks to whatever PALI_API points at (default: the LAN).

const http = require('http')
const path = require('path')
const fs = require('fs')
const { DesktopServer } = require('../src/server')

const DIST = path.resolve(__dirname, '..', '..', 'frontend', 'dist')
const API = process.env.PALI_API || 'http://pali.bigubuntu.internal'

let pass = 0
let fail = 0

function check (name, ok, detail) {
  if (ok) {
    pass++
    console.log(`  ok   ${name}`)
  } else {
    fail++
    console.log(`  FAIL ${name}${detail ? ` — ${detail}` : ''}`)
  }
}

function get (url, opts = {}) {
  return new Promise((resolve, reject) => {
    const req = http.request(url, { method: opts.method || 'GET', headers: opts.headers || {} }, (res) => {
      const chunks = []
      res.on('data', (c) => chunks.push(c))
      res.on('end', () => resolve({
        status: res.statusCode,
        headers: res.headers,
        body: Buffer.concat(chunks),
        text: Buffer.concat(chunks).toString('utf8'),
      }))
    })
    req.on('error', reject)
    req.end()
  })
}

async function main () {
  if (!fs.existsSync(path.join(DIST, 'index.html'))) {
    console.error(`frontend/dist 不存在：${DIST}\n先在 frontend/ 里 npm run build。`)
    process.exit(2)
  }

  const server = new DesktopServer({ distDir: DIST, apiBase: API, cacheTtlMs: 600000, configPath: '/tmp/config.json', configSource: 'test' })
  await server.start()
  const o = server.origin
  console.log(`\nserver ${o} → api ${API}\n`)

  // --- shell from disk ----------------------------------------------------
  const index = await get(o + '/')
  check('GET / serves the shell', index.status === 200 && index.text.includes('<div id="app">'), `status=${index.status}`)
  check('shell carries the desktop inject script', index.text.includes('/__desktop/inject.js'))
  check('shell is not long-cached', index.headers['cache-control'] === 'no-cache', index.headers['cache-control'])
  check('shell has an etag', !!index.headers.etag)

  const assetName = (index.text.match(/src="(\/assets\/[^"]+\.js)"/) || [])[1]
  const asset = await get(o + assetName)
  check('hashed asset is served with the right type', asset.headers['content-type'] === 'text/javascript; charset=utf-8', asset.headers['content-type'])
  check('hashed asset is immutable', String(asset.headers['cache-control']).includes('immutable'), asset.headers['cache-control'])

  const cond = await get(o + assetName, { headers: { 'if-none-match': asset.headers.etag } })
  check('conditional asset request answers 304', cond.status === 304, `status=${cond.status}`)

  const spa = await get(o + '/read/mula_vi_01')
  check('SPA fallback serves index.html for a client route', spa.status === 200 && spa.text.includes('<div id="app">'), `status=${spa.status}`)

  const missing = await get(o + '/assets/nope-1234.js')
  check('unknown asset path falls back to the shell, not a 404 body', missing.status === 200 && missing.text.includes('<div id="app">'))

  const evil = await get(o + '/../../../../etc/passwd')
  check('path traversal is refused', evil.status === 200 && !evil.text.includes('root:'), `status=${evil.status}`)

  const badHost = await get(o + '/', { headers: { host: 'evil.example.com' } })
  check('non-loopback Host is refused', badHost.status === 403, `status=${badHost.status}`)

  // --- api proxy ----------------------------------------------------------
  const health = await get(o + '/__desktop/health')
  const healthJson = JSON.parse(health.text)
  check('health probe reports the backend is up', healthJson.ok === true, JSON.stringify(healthJson))
  if (!healthJson.ok) {
    console.log('\n后端不可达，跳过缓存用例。')
    await server.close()
    console.log(`\n${pass} passed, ${fail} failed\n`)
    process.exit(fail ? 1 : 0)
  }

  server.cache.clear()
  const c1 = await get(o + '/api/catalog')
  const c2 = await get(o + '/api/catalog')
  check('catalog proxies through', c1.status === 200 && c1.text.startsWith('{'), `status=${c1.status}`)
  check('first catalog request is a miss', c1.headers['x-pali-cache'] === 'miss', c1.headers['x-pali-cache'])
  check('second catalog request is a hit', c2.headers['x-pali-cache'] === 'hit', c2.headers['x-pali-cache'])
  check('cached body is byte-identical', c1.body.equals(c2.body), `${c1.body.length} vs ${c2.body.length}`)
  check('cached body is not encoding-mislabelled', !c2.headers['content-encoding'], c2.headers['content-encoding'])
  check('cache hit carries an age header', c2.headers.age !== undefined)

  const s1 = await get(o + '/api/books/mula_vi_01/segments?from=1&count=60')
  const s2 = await get(o + '/api/books/mula_vi_01/segments?from=1&count=60')
  check('book segments are cached', s1.headers['x-pali-cache'] === 'miss' && s2.headers['x-pali-cache'] === 'hit', `${s1.headers['x-pali-cache']}/${s2.headers['x-pali-cache']}`)
  check('a different window is a different key', (await get(o + '/api/books/mula_vi_01/segments?from=61&count=60')).headers['x-pali-cache'] === 'miss')
  check('segments payload has Pali text', JSON.parse(s2.text).items.some((i) => /[āīūṭḍṇṃḷ]/.test(i.text || '')))

  const m1 = await get(o + '/api/books/mula_vi_01/marks?from=1&to=60')
  const m2 = await get(o + '/api/books/mula_vi_01/marks?from=1&to=60')
  check('marks are never cached (per-reader, mutable)', m1.headers['x-pali-cache'] === 'bypass' && m2.headers['x-pali-cache'] === 'bypass', `${m1.headers['x-pali-cache']}/${m2.headers['x-pali-cache']}`)

  const search = await get(o + '/api/books/mula_vi_01/search?q=buddha')
  check('book search is never cached', search.headers['x-pali-cache'] === 'bypass', search.headers['x-pali-cache'])

  // --- the failure path ---------------------------------------------------
  const dead = new DesktopServer({ distDir: DIST, apiBase: 'http://127.0.0.1:9', cacheTtlMs: 60000 })
  await dead.start()
  const deadProbe = JSON.parse((await get(dead.origin + '/__desktop/health')).text)
  check('health probe reports a dead backend', deadProbe.ok === false, JSON.stringify(deadProbe))
  const deadApi = await get(dead.origin + '/api/catalog')
  check('API call against a dead backend returns 502 JSON', deadApi.status === 502 && JSON.parse(deadApi.text).error === 'upstream_unreachable', `status=${deadApi.status}`)
  const deadShell = await get(dead.origin + '/')
  check('the shell still loads with the backend down', deadShell.status === 200 && deadShell.text.includes('<div id="app">'), `status=${deadShell.status}`)
  const offline = await get(dead.origin + '/__desktop/offline')
  check('offline page names the backend', offline.status === 200 && offline.text.includes('http://127.0.0.1:9') && offline.text.includes('连不上后端'))
  const status = JSON.parse((await get(dead.origin + '/__desktop/status')).text)
  check('status endpoint reflects the failure', status.ok === false && !!status.error, JSON.stringify(status).slice(0, 200))
  await dead.close()

  const stats = server.cache.stats()
  check('cache reports hits', stats.hits >= 2, JSON.stringify(stats))

  // --- proxy contract, against a local echo backend -----------------------
  // The real backend cannot tell us whether a header arrived or a body was
  // altered; an echo server can. This is what proves "streaming, unchanged".
  const seen = []
  const echo = http.createServer((req, res) => {
    const chunks = []
    req.on('data', (c) => chunks.push(c))
    req.on('end', () => {
      const body = Buffer.concat(chunks)
      seen.push({ method: req.method, url: req.url, headers: req.headers, body })
      if (req.url.startsWith('/api/echo-binary')) {
        // 2MB of deterministic bytes, to catch buffering/truncation mistakes.
        const big = Buffer.alloc(2 * 1024 * 1024)
        for (let i = 0; i < big.length; i++) big[i] = i % 251
        res.writeHead(200, { 'content-type': 'application/octet-stream', 'cache-control': 'public, max-age=60', etag: '"abc"' })
        res.end(big)
        return
      }
      if (req.url.startsWith('/api/echo-204')) {
        res.writeHead(204).end()
        return
      }
      res.writeHead(200, { 'content-type': 'application/json', etag: '"etag-1"', 'cache-control': 'public, max-age=30' })
      res.end(JSON.stringify({ method: req.method, url: req.url, headers: req.headers, bodyLength: body.length, body: body.toString('utf8') }))
    })
  })
  await new Promise((r) => echo.listen(0, '127.0.0.1', r))
  const echoPort = echo.address().port

  const via = new DesktopServer({ distDir: DIST, apiBase: `http://127.0.0.1:${echoPort}`, cacheTtlMs: 60000 })
  await via.start()

  const bearer = 'Bearer tok_ABC-123'
  const echoed = JSON.parse((await get(via.origin + '/api/auth/me?x=1', { headers: { authorization: bearer, 'x-custom': 'still-here', accept: 'application/json' } })).text)
  check('Authorization is forwarded verbatim', echoed.headers.authorization === bearer, echoed.headers.authorization)
  check('custom request headers are forwarded', echoed.headers['x-custom'] === 'still-here')
  check('query string is preserved', echoed.url === '/api/auth/me?x=1', echoed.url)
  check('Host is rewritten to the backend', echoed.headers.host === `127.0.0.1:${echoPort}`, echoed.headers.host)

  const posted = JSON.parse((await get(via.origin + '/api/work/notes', {
    method: 'PUT',
    headers: { 'content-type': 'application/json', authorization: bearer },
  })).text)
  check('request method is forwarded', posted.method === 'PUT', posted.method)

  const bigRes = await get(via.origin + '/api/echo-binary')
  check('a 2MB response arrives byte-complete', bigRes.body.length === 2 * 1024 * 1024 && bigRes.body[1] === 1 && bigRes.body[1024 * 1024] === (1024 * 1024) % 251, `${bigRes.body.length} bytes`)
  check('content-type passes through', bigRes.headers['content-type'] === 'application/octet-stream', bigRes.headers['content-type'])
  check('etag passes through for the OS HTTP cache', bigRes.headers.etag === '"abc"', bigRes.headers.etag)
  check('cache-control passes through for the OS HTTP cache', String(bigRes.headers['cache-control']).includes('max-age=60'), bigRes.headers['cache-control'])
  check('oversized responses are not cached', via.cache.stats().entries === 0, JSON.stringify(via.cache.stats()))

  const noContent = await get(via.origin + '/api/echo-204')
  check('204 passes through with no body', noContent.status === 204 && noContent.body.length === 0, `status=${noContent.status} bytes=${noContent.body.length}`)

  // The echo backend must not have seen any hop-by-hop header the client sent.
  check('hop-by-hop headers are not relayed', !seen.some((s) => s.headers['proxy-connection'] !== undefined))
  await via.close()
  await new Promise((r) => echo.close(r))

  await server.close()
  console.log(`\n${pass} passed, ${fail} failed\n`)
  process.exit(fail ? 1 : 0)
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
