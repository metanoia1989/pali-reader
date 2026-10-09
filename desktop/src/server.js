'use strict'

// The whole "backend" of the desktop app: one loopback HTTP server that
//   (a) serves ../frontend/dist from disk, and
//   (b) reverse-proxies /api/* to the LAN backend.
//
// Serving the shell locally is what makes this feel unlike a browser tab: the
// HTML, JS and CSS never touch the network, so the only thing left to wait for
// is the API. Because the shell is served from the same origin as /api, the
// frontend's relative '/api' URLs keep working with no change to frontend/.

const http = require('http')
const fs = require('fs')
const fsp = require('fs/promises')
const path = require('path')
const crypto = require('crypto')
const { URL } = require('url')
const { ResponseCache } = require('./cache')

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.map': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.avif': 'image/avif',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.otf': 'font/otf',
  '.txt': 'text/plain; charset=utf-8',
  '.wasm': 'application/wasm',
  '.mp3': 'audio/mpeg',
  '.mp4': 'video/mp4',
  '.webm': 'video/webm',
  '.pdf': 'application/pdf',
}

// Headers that describe a single hop and must not be relayed (RFC 9110 §7.6.1).
const HOP_BY_HOP = new Set([
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'proxy-connection',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
])

// Only corpus data: identical for every reader and only changed by re-running
// the importer. Everything else (/api/work/*, /api/auth/*, /api/dict/*) is
// either mutable or per-reader, so it goes straight through.
const CACHEABLE = [
  /^\/api\/catalog$/,
  /^\/api\/books\/[^/]+$/,
  /^\/api\/books\/[^/]+\/segments$/,
]
// /api/books/<id>/marks is the reader's own annotations and /search is a query;
// both match the "/api/books/*" shape but must never be served from a cache.
const NEVER_CACHE = [/\/marks$/, /\/search$/]

function isCacheablePath (pathname) {
  if (NEVER_CACHE.some((re) => re.test(pathname))) return false
  return CACHEABLE.some((re) => re.test(pathname))
}

function escapeHtml (s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

function etagFor (stat) {
  return `W/"${stat.size.toString(16)}-${Math.floor(stat.mtimeMs).toString(16)}"`
}

/** Short, stable digest used to keep per-reader responses out of each other's cache slots. */
function shortHash (s) {
  return crypto.createHash('sha1').update(s).digest('hex').slice(0, 12)
}

class DesktopServer {
  constructor ({ distDir, apiBase, cacheTtlMs = 0, log = () => {}, version = '', configPath = '', configSource = '' } = {}) {
    this.distDir = distDir
    this.apiBase = apiBase
    this.log = log
    this.version = version
    this.cache = new ResponseCache({ ttlMs: cacheTtlMs })
    this.indexHtml = null
    this.indexHtmlStamp = ''
    this.server = null
    this.port = 0
    this.origin = ''
    this.upstream = new URL(apiBase)
    this.agent = this.upstream.protocol === 'https:'
      ? new (require('https').Agent)({ keepAlive: true, maxSockets: 24, keepAliveMsecs: 15000 })
      : new http.Agent({ keepAlive: true, maxSockets: 24, keepAliveMsecs: 15000 })
    // Kept up to date by every proxied request, so the in-page banner can ask
    // "is the backend reachable right now?" without doing its own probe.
    this.status = {
      ok: null,
      checkedAt: 0,
      error: '',
      apiBase,
      configPath,
      configSource,
      lastSuccessAt: 0,
      lastFailureAt: 0,
    }
    this.counters = { proxies: 0, cacheHits: 0, cacheMisses: 0, cacheBypass: 0, upstreamErrors: 0, staticHits: 0 }
    this.pages = {
      offline: fs.readFileSync(path.join(__dirname, 'pages', 'offline.html'), 'utf8'),
      inject: fs.readFileSync(path.join(__dirname, 'pages', 'inject.js'), 'utf8'),
    }
  }

  get cacheKeyPrefix () {
    return `api ${this.apiBase}`
  }

  /**
   * index.html is rewritten once per build so the page loads /__desktop/inject.js,
   * which is what surfaces "the backend just went away" without touching any
   * file in frontend/.
   */
  getIndexHtml () {
    const file = path.join(this.distDir, 'index.html')
    const stat = fs.statSync(file)
    const stamp = `${stat.size}:${stat.mtimeMs}`
    if (this.indexHtml && this.indexHtmlStamp === stamp) return this.indexHtml
    let html = fs.readFileSync(file, 'utf8')
    const tag = '<script src="/__desktop/inject.js" defer></script>'
    html = html.includes('</body>') ? html.replace('</body>', `  ${tag}\n  </body>`) : html + tag
    this.indexHtml = html
    this.indexHtmlStamp = stamp
    return html
  }

  async start () {
    this.server = http.createServer((req, res) => {
      this.handle(req, res).catch((err) => {
        this.log(`request failed: ${req.method} ${req.url} — ${err.message}`)
        if (!res.headersSent) {
          res.writeHead(500, { 'content-type': 'application/json; charset=utf-8' })
        }
        res.end(JSON.stringify({ error: 'desktop_shell_error', msg: err.message }))
      })
    })
    this.server.keepAliveTimeout = 20000
    await new Promise((resolve, reject) => {
      this.server.once('error', reject)
      // Port 0 = let the OS pick a free loopback port. Nothing is exposed off
      // the machine, which is also why binding 127.0.0.1 (not 0.0.0.0) matters.
      this.server.listen(0, '127.0.0.1', () => {
        this.port = this.server.address().port
        this.origin = `http://127.0.0.1:${this.port}`
        resolve()
      })
    })
    return { port: this.port, origin: this.origin }
  }

  async close () {
    this.agent.destroy()
    if (!this.server) return
    await new Promise((resolve) => this.server.close(resolve))
  }

  url (p = '/') {
    return this.origin + p
  }

  async handle (req, res) {
    const url = new URL(req.url, this.origin || 'http://127.0.0.1')
    // The server is loopback-only; refusing a non-loopback Host closes the
    // DNS-rebinding hole where a web page points its own hostname at us.
    const host = (req.headers.host || '').replace(/:\d+$/, '')
    if (host !== '127.0.0.1' && host !== 'localhost' && host !== '[::1]') {
      res.writeHead(403, { 'content-type': 'text/plain; charset=utf-8' })
      res.end('forbidden host')
      return
    }
    if (url.pathname.startsWith('/__desktop/')) {
      await this.handleDesktop(req, res, url)
      return
    }
    if (url.pathname === '/api' || url.pathname.startsWith('/api/')) {
      this.proxyApi(req, res, url)
      return
    }
    await this.serveStatic(req, res, url)
  }

  // --- static shell -------------------------------------------------------

  /** Maps a URL path to a file inside distDir, or null if it escapes/does not exist. */
  resolveStatic (pathname) {
    let rel
    try {
      rel = decodeURIComponent(pathname)
    } catch {
      return null
    }
    const resolved = path.resolve(this.distDir, '.' + path.posix.normalize(rel))
    if (resolved !== this.distDir && !resolved.startsWith(this.distDir + path.sep)) return null
    return resolved
  }

  async serveStatic (req, res, url) {
    if (req.method !== 'GET' && req.method !== 'HEAD') {
      res.writeHead(405, { allow: 'GET, HEAD' })
      res.end()
      return
    }
    const isShell = url.pathname === '/' || url.pathname === '/index.html'
    let file = isShell ? null : this.resolveStatic(url.pathname)
    let stat = null
    if (file) {
      try {
        stat = await fsp.stat(file)
        if (stat.isDirectory()) stat = null
      } catch {
        stat = null
      }
    }
    if (!stat) {
      // SPA fallback: /read/mula_vi_01, /catalog, /vocab … are client routes,
      // so anything that is not a real file gets the shell.
      file = path.join(this.distDir, 'index.html')
      try {
        stat = await fsp.stat(file)
      } catch {
        this.sendMissingShell(res)
        return
      }
    }

    const ext = path.extname(file).toLowerCase()
    const isIndex = path.basename(file) === 'index.html'
    const etag = etagFor(stat)
    const headers = {
      'content-type': MIME[ext] || 'application/octet-stream',
      // Vite fingerprints everything under /assets, so those bytes can never
      // change under the same name — cache them for a year. index.html must
      // revalidate, otherwise a rebuilt frontend would never be picked up.
      'cache-control': isIndex
        ? 'no-cache'
        : file.includes(`${path.sep}assets${path.sep}`)
          ? 'public, max-age=31536000, immutable'
          : 'public, max-age=3600',
      etag,
      'last-modified': new Date(stat.mtimeMs).toUTCString(),
      'x-pali-shell': 'disk',
    }

    if (this.isNotModified(req, etag, stat.mtimeMs)) {
      res.writeHead(304, headers)
      res.end()
      return
    }

    this.counters.staticHits++
    if (isIndex) {
      let html
      try {
        html = this.getIndexHtml()
      } catch (err) {
        this.sendMissingShell(res, err.message)
        return
      }
      const body = Buffer.from(html, 'utf8')
      headers['content-type'] = MIME['.html']
      headers['content-length'] = String(body.length)
      res.writeHead(200, headers)
      res.end(req.method === 'HEAD' ? undefined : body)
      return
    }

    headers['content-length'] = String(stat.size)
    res.writeHead(200, headers)
    if (req.method === 'HEAD') {
      res.end()
      return
    }
    const stream = fs.createReadStream(file)
    stream.on('error', () => res.destroy())
    stream.pipe(res)
  }

  sendMissingShell (res, detail) {
    const msg = [
      '找不到前端产物：' + path.join(this.distDir, 'index.html'),
      '',
      '先在 pali-reader/frontend 里执行 npm run build 生成 dist，再启动桌面应用。',
      detail ? `（${detail}）` : '',
    ].join('\n')
    res.writeHead(500, { 'content-type': 'text/plain; charset=utf-8' })
    res.end(msg)
  }

  isNotModified (req, etag, mtimeMs) {
    const inm = req.headers['if-none-match']
    if (inm && etag && inm.split(',').some((t) => t.trim() === etag)) return true
    const ims = req.headers['if-modified-since']
    // mtimeMs === 0 marks a response with no file behind it (the API cache),
    // where a bare If-Modified-Since must never be read as "still fresh".
    if (!inm && ims && mtimeMs > 0) {
      const since = Date.parse(ims)
      if (Number.isFinite(since) && Math.floor(mtimeMs / 1000) * 1000 <= since) return true
    }
    return false
  }

  // --- /api proxy ---------------------------------------------------------

  proxyApi (req, res, url) {
    this.counters.proxies++
    const pathname = url.pathname
    const cacheable = req.method === 'GET' && isCacheablePath(pathname) && this.cache.enabled
    const auth = req.headers.authorization || ''
    // Authorization participates in the key so a signed-in reader can never be
    // served another reader's response, even for corpus data.
    const key = `${this.cacheKeyPrefix} ${req.method} ${url.pathname}${url.search} ${shortHash(auth)}`

    if (cacheable) {
      const hit = this.cache.get(key)
      if (hit) {
        this.counters.cacheHits++
        this.status.ok = true
        this.status.checkedAt = Date.now()
        const headers = { ...hit.headers, 'x-pali-cache': 'hit', age: String(Math.floor((Date.now() - hit.storedAt) / 1000)) }
        if (this.isNotModified(req, headers.etag, 0)) {
          res.writeHead(304, headers)
          res.end()
          return
        }
        res.writeHead(hit.status, headers)
        res.end(hit.body)
        return
      }
      this.counters.cacheMisses++
    } else {
      this.counters.cacheBypass++
    }

    const upstreamHeaders = {}
    for (const [name, value] of Object.entries(req.headers)) {
      const lower = name.toLowerCase()
      if (lower === 'host' || HOP_BY_HOP.has(lower)) continue
      upstreamHeaders[lower] = value
    }
    upstreamHeaders.host = this.upstream.host
    if (cacheable) {
      // Force identity so the cached bytes are self-describing: whatever a
      // given client asked for, what we store can be replayed to any other
      // client without mislabelling the encoding.
      upstreamHeaders['accept-encoding'] = 'identity'
    }

    const options = {
      protocol: this.upstream.protocol,
      hostname: this.upstream.hostname,
      port: this.upstream.port || (this.upstream.protocol === 'https:' ? 443 : 80),
      method: req.method,
      path: this.upstream.pathname.replace(/\/$/, '') + url.pathname + url.search,
      headers: upstreamHeaders,
      agent: this.agent,
    }

    const timeoutMs = Number(process.env.PALI_PROXY_TIMEOUT_MS) > 0 ? Number(process.env.PALI_PROXY_TIMEOUT_MS) : 20000
    const upstreamReq = http.request(options, (upstreamRes) => {
      this.status.ok = true
      this.status.checkedAt = Date.now()
      this.status.lastSuccessAt = Date.now()
      this.status.error = ''

      const responseHeaders = {}
      for (const [name, value] of Object.entries(upstreamRes.headers)) {
        const lower = name.toLowerCase()
        if (HOP_BY_HOP.has(lower)) continue
        if (cacheable && (lower === 'content-encoding' || lower === 'content-length' || lower === 'vary')) continue
        responseHeaders[lower] = value
      }
      responseHeaders['x-pali-cache'] = cacheable ? 'miss' : 'bypass'
      res.writeHead(upstreamRes.statusCode, responseHeaders)

      if (cacheable && upstreamRes.statusCode === 200) {
        const chunks = []
        let size = 0
        let recording = true
        upstreamRes.on('data', (chunk) => {
          if (!recording) return
          size += chunk.length
          // Past the cap, stop recording and let the rest stream: correctness
          // first, the cache is an optimisation and may simply decline.
          if (size > 8 * 1024 * 1024) {
            recording = false
            chunks.length = 0
            return
          }
          chunks.push(chunk)
        })
        upstreamRes.on('end', () => {
          if (!recording) return
          this.cache.set(key, {
            status: 200,
            headers: { ...responseHeaders, 'content-length': String(size) },
            body: Buffer.concat(chunks),
          })
        })
      }

      upstreamRes.pipe(res)
    })

    upstreamReq.setTimeout(timeoutMs, () => {
      upstreamReq.destroy(new Error(`上游 ${timeoutMs}ms 无响应`))
    })

    upstreamReq.on('error', (err) => {
      this.counters.upstreamErrors++
      this.status.ok = false
      this.status.checkedAt = Date.now()
      this.status.lastFailureAt = Date.now()
      this.status.error = err.message
      this.log(`upstream error: ${req.method} ${url.pathname} → ${this.apiBase}: ${err.message}`)
      if (!res.headersSent) {
        res.writeHead(502, {
          'content-type': 'application/json; charset=utf-8',
          'cache-control': 'no-store',
          'x-pali-upstream': this.apiBase,
        })
      }
      res.end(JSON.stringify({
        error: 'upstream_unreachable',
        msg: `无法连接后端 ${this.apiBase}（${err.message}）`,
        apiBase: this.apiBase,
      }))
    })

    // Stream the request body straight through: JSON writes are small, but
    // buffering them would be a needless copy and would break large exports.
    req.pipe(upstreamReq)
  }

  // --- desktop control surface -------------------------------------------

  async handleDesktop (req, res, url) {
    switch (url.pathname) {
      case '/__desktop/status':
        this.sendJson(res, 200, {
          ok: this.status.ok,
          apiBase: this.apiBase,
          error: this.status.error,
          checkedAt: this.status.checkedAt,
          cache: this.cache.stats(),
        })
        return

      case '/__desktop/health': {
        const probe = await this.probeUpstream()
        this.sendJson(res, probe.ok ? 200 : 503, { ...probe, apiBase: this.apiBase })
        return
      }

      case '/__desktop/retry': {
        const probe = await this.probeUpstream()
        if (probe.ok) {
          res.writeHead(302, { location: '/', 'cache-control': 'no-store' })
          res.end()
        } else {
          res.writeHead(302, { location: '/__desktop/offline', 'cache-control': 'no-store' })
          res.end()
        }
        return
      }

      case '/__desktop/offline': {
        const body = Buffer.from(this.renderOffline(this.status.error || '尚未连接'), 'utf8')
        res.writeHead(200, {
          'content-type': 'text/html; charset=utf-8',
          'content-length': String(body.length),
          'cache-control': 'no-store',
        })
        res.end(req.method === 'HEAD' ? undefined : body)
        return
      }

      case '/__desktop/inject.js': {
        const body = Buffer.from(this.pages.inject, 'utf8')
        res.writeHead(200, {
          'content-type': MIME['.js'],
          'content-length': String(body.length),
          'cache-control': 'no-store',
        })
        res.end(req.method === 'HEAD' ? undefined : body)
        return
      }

      default:
        this.sendJson(res, 404, { error: 'not_found' })
    }
  }

  sendJson (res, status, payload) {
    const body = Buffer.from(JSON.stringify(payload), 'utf8')
    res.writeHead(status, {
      'content-type': 'application/json; charset=utf-8',
      'content-length': String(body.length),
      'cache-control': 'no-store',
    })
    res.end(body)
  }

  renderOffline (error) {
    const css = this.pages.offline
    return css
      .replaceAll('{{API_BASE}}', escapeHtml(this.apiBase))
      .replaceAll('{{ERROR}}', escapeHtml(error))
      .replaceAll('{{CONFIG_PATH}}', escapeHtml(this.status.configPath || ''))
      .replaceAll('{{CONFIG_SOURCE}}', escapeHtml(this.status.configSource || ''))
      .replaceAll('{{TUNNEL}}', escapeHtml(this.apiBase.replace(/:\d+$/, ':8099')))
  }

  /** One GET against the backend, used before the window loads and by the retry button. */
  probeUpstream (timeoutMs) {
    const ms = timeoutMs || 3500
    return new Promise((resolve) => {
      const started = Date.now()
      const target = this.upstream.pathname.replace(/\/$/, '') + '/api/health'
      const probeReq = http.request({
        protocol: this.upstream.protocol,
        hostname: this.upstream.hostname,
        port: this.upstream.port || (this.upstream.protocol === 'https:' ? 443 : 80),
        method: 'GET',
        path: target,
        headers: { host: this.upstream.host, accept: 'application/json', 'accept-encoding': 'identity' },
        agent: this.agent,
      }, (r) => {
        r.resume()
        const ok = r.statusCode >= 200 && r.statusCode < 400
        const result = { ok, status: r.statusCode, ms: Date.now() - started, error: ok ? '' : `HTTP ${r.statusCode}` }
        this.status.ok = ok
        this.status.checkedAt = Date.now()
        this.status.error = result.error
        resolve(result)
      })
      probeReq.setTimeout(ms, () => probeReq.destroy(new Error(`${ms}ms 超时`)))
      probeReq.on('error', (err) => {
        this.status.ok = false
        this.status.checkedAt = Date.now()
        this.status.error = err.message
        resolve({ ok: false, status: 0, ms: Date.now() - started, error: err.message })
      })
      probeReq.end()
    })
  }

  /** Best-effort warm-up so the first screen does not pay for /api/catalog. */
  warm (paths = ['/api/catalog']) {
    for (const p of paths) {
      const warmReq = http.request({
        hostname: '127.0.0.1',
        port: this.port,
        method: 'GET',
        path: p,
        headers: { host: `127.0.0.1:${this.port}`, accept: 'application/json', 'accept-encoding': 'identity' },
      }, (r) => r.resume())
      warmReq.on('error', () => {})
      warmReq.end()
    }
  }
}

module.exports = { DesktopServer, isCacheablePath, MIME }
