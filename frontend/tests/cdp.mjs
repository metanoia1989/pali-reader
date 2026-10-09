// A minimal Chrome DevTools Protocol client.
//
// Written rather than installed: the browser extension that normally drives a
// browser for this session is not connected, and Node 18 has neither a global
// WebSocket nor a `ws` dependency here. CDP is a JSON protocol over one
// WebSocket, and a client for it is a handshake plus frame encoding — small
// enough to keep in the repository and far more useful than no browser at all.
//
// Usage:
//   const chrome = await launch()          // starts headless Chrome
//   const page = await chrome.page()       // a tab with the debugger attached
//   await page.goto(url)
//   await page.click(sel)
//   await page.shot('/tmp/x.png')
//   await chrome.close()
import { createHash, randomBytes } from 'node:crypto'
import { spawn } from 'node:child_process'
import { connect } from 'node:net'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const GUID = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11'

/** One WebSocket connection, framing done by hand. */
class Socket {
  constructor(net) {
    this.net = net
    this.buf = Buffer.alloc(0)
    this.frames = []
    this.waiter = null
    net.on('data', (chunk) => {
      this.buf = Buffer.concat([this.buf, chunk])
      this.drain()
    })
    net.on('error', (e) => this.reject(e))
    net.on('close', () => this.reject(new Error('socket closed')))
  }

  reject(err) {
    if (this.waiter) {
      const w = this.waiter
      this.waiter = null
      w.reject(err)
    }
  }

  drain() {
    for (;;) {
      const frame = this.readFrame()
      if (!frame) return
      if (frame.opcode === 1) this.frames.push(frame.payload.toString('utf8'))
      if (this.waiter && this.frames.length) {
        const w = this.waiter
        this.waiter = null
        w.resolve(this.frames.shift())
      }
    }
  }

  readFrame() {
    const b = this.buf
    if (b.length < 2) return null
    const opcode = b[0] & 0x0f
    const masked = (b[1] & 0x80) !== 0
    let len = b[1] & 0x7f
    let at = 2
    if (len === 126) {
      if (b.length < at + 2) return null
      len = b.readUInt16BE(at)
      at += 2
    } else if (len === 127) {
      if (b.length < at + 8) return null
      len = Number(b.readBigUInt64BE(at))
      at += 8
    }
    if (masked) {
      if (b.length < at + 4) return null
      at += 4
    }
    if (b.length < at + len) return null
    const payload = b.subarray(at, at + len)
    this.buf = b.subarray(at + len)
    return { opcode, payload }
  }

  next(timeoutMs = 20000) {
    if (this.frames.length) return Promise.resolve(this.frames.shift())
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.waiter = null
        reject(new Error('CDP response timed out'))
      }, timeoutMs)
      this.waiter = {
        resolve: (v) => {
          clearTimeout(timer)
          resolve(v)
        },
        reject: (e) => {
          clearTimeout(timer)
          reject(e)
        },
      }
    })
  }

  send(text) {
    const payload = Buffer.from(text, 'utf8')
    const mask = randomBytes(4)
    let header
    if (payload.length < 126) {
      header = Buffer.alloc(2)
      header[1] = 0x80 | payload.length
    } else if (payload.length < 65536) {
      header = Buffer.alloc(4)
      header[1] = 0x80 | 126
      header.writeUInt16BE(payload.length, 2)
    } else {
      header = Buffer.alloc(10)
      header[1] = 0x80 | 127
      header.writeBigUInt64BE(BigInt(payload.length), 2)
    }
    header[0] = 0x81
    const masked = Buffer.alloc(payload.length)
    for (let i = 0; i < payload.length; i++) masked[i] = payload[i] ^ mask[i % 4]
    this.net.write(Buffer.concat([header, mask, masked]))
  }
}

function handshake(net, wsPath) {
  return new Promise((resolve, reject) => {
    const key = randomBytes(16).toString('base64')
    const expect = createHash('sha1').update(key + GUID).digest('base64')
    let head = ''
    const onData = (chunk) => {
      head += chunk.toString('latin1')
      const end = head.indexOf('\r\n\r\n')
      if (end < 0) return
      net.off('data', onData)
      const status = head.split('\r\n')[0]
      const got = /sec-websocket-accept:\s*(\S+)/i.exec(head)?.[1]
      if (!/101/.test(status)) return reject(new Error(`upgrade failed: ${status}`))
      if (got !== expect) return reject(new Error('bad Sec-WebSocket-Accept'))
      // Anything past the header belongs to the frame stream.
      const rest = Buffer.from(head.slice(end + 4), 'latin1')
      const sock = new Socket(net)
      if (rest.length) {
        sock.buf = rest
        sock.drain()
      }
      resolve(sock)
    }
    net.on('data', onData)
    net.on('error', reject)
    net.write(
      `GET ${wsPath} HTTP/1.1\r\nHost: 127.0.0.1\r\nUpgrade: websocket\r\n` +
        `Connection: Upgrade\r\nSec-WebSocket-Key: ${key}\r\nSec-WebSocket-Version: 13\r\n\r\n`,
    )
  })
}

async function httpGet(url) {
  const res = await fetch(url)
  return res.json()
}

async function waitFor(fn, { tries = 60, gapMs = 250 } = {}) {
  for (let i = 0; i < tries; i++) {
    try {
      const v = await fn()
      if (v) return v
    } catch {
      /* not up yet */
    }
    await new Promise((r) => setTimeout(r, gapMs))
  }
  throw new Error('timed out waiting for Chrome')
}

export async function launch({ port = 0, width = 1600, height = 1000 } = {}) {
  // A fixed port collides with a previous run whose close() never happened,
  // and the symptom is a launch that hangs rather than one that fails.
  if (!port) port = 9400 + Math.floor(Math.random() * 400)
  const profile = mkdtempSync(join(tmpdir(), 'cdp-pali-'))
  const child = spawn(CHROME, [
    '--headless=new',
    '--no-sandbox',
    '--disable-gpu',
    '--disable-crash-reporter',
    `--crash-dumps-dir=${profile}/crash`,
    '--no-first-run',
    '--no-default-browser-check',
    // The workstation proxy cannot reach the LAN host; the tunnel is on
    // localhost, which must not go through it either.
    '--no-proxy-server',
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${profile}`,
    `--window-size=${width},${height}`,
    'about:blank',
  ], { stdio: 'ignore', detached: false })

  const version = await waitFor(() =>
    httpGet(`http://127.0.0.1:${port}/json/version`).catch(() => null),
  { tries: 80, gapMs: 250 }).catch((e) => {
    try {
      child.kill('SIGKILL')
    } catch {
      /* already gone */
    }
    throw e
  })

  let id = 0
  const pending = new Map()
  const events = []

  const wsUrl = version.webSocketDebuggerUrl
  const net = connect(Number(new URL(wsUrl).port), new URL(wsUrl).hostname)
  await new Promise((res, rej) => {
    net.once('connect', res)
    net.once('error', rej)
  })
  const browser = await handshake(net, new URL(wsUrl).pathname)
  browser.drain = (function (orig) {
    return function () {
      orig.call(this)
      while (this.frames.length) {
        const msg = JSON.parse(this.frames.shift())
        if (msg.id && pending.has(msg.id)) {
          const { resolve, reject } = pending.get(msg.id)
          pending.delete(msg.id)
          msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result)
        } else if (msg.method) {
          events.push(msg)
        }
      }
    }
  })(browser.drain)

  const call = (method, params = {}, sessionId) =>
    new Promise((resolve, reject) => {
      const msgId = ++id
      pending.set(msgId, { resolve, reject })
      browser.send(JSON.stringify({ id: msgId, method, params, ...(sessionId ? { sessionId } : {}) }))
    })

  const target = await call('Target.createTarget', { url: 'about:blank' })
  const attached = await call('Target.attachToTarget', { targetId: target.targetId, flatten: true })
  const session = attached.sessionId

  const page = {
    session,
    call: (m, p = {}) => call(m, p, session),
    events,
    async goto(url, settleMs = 1800) {
      await page.call('Page.enable')
      await page.call('Runtime.enable')
      await page.call('Page.navigate', { url })
      await new Promise((r) => setTimeout(r, settleMs))
      return page
    },
    async eval(expression) {
      const r = await page.call('Runtime.evaluate', {
        expression, returnByValue: true, awaitPromise: true,
      })
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.text)
      return r.result.value
    },
    async click(selector) {
      const ok = await page.eval(`(() => {
        const el = document.querySelector(${JSON.stringify(selector)})
        if (!el) return false
        el.scrollIntoView({ block: 'center' })
        el.click()
        return true
      })()`)
      if (!ok) throw new Error(`no element for ${selector}`)
      await new Promise((r) => setTimeout(r, 500))
    },
    async shot(path) {
      const { data } = await page.call('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
      writeFileSync(path, Buffer.from(data, 'base64'))
      return path
    },
    async setViewport(w, h, mobile = false) {
      await page.call('Emulation.setDeviceMetricsOverride', {
        width: w, height: h, deviceScaleFactor: 1, mobile,
      })
    },
  }

  return {
    page,
    call,
    async close() {
      try {
        child.kill('SIGKILL')
      } catch {
        /* already gone */
      }
    },
  }
}
