#!/usr/bin/env node
'use strict'

// Independent verification that the *shipping* app (src/main.js, i.e. `npm start`)
// really renders Pali — not just that its HTTP server answers.
//
// It attaches to the Chromium DevTools port the app was started with, asks the
// page what is in its DOM, and saves a screenshot of the live renderer. Nothing
// in the app cooperates with this: it is the same debugging interface a browser
// exposes, and it is the only way to see into the window on a machine where
// macOS Screen Recording permission is not granted (screencapture returns the
// wallpaper and nothing else).
//
//   node tools/cdp-probe.js --port 9333 --out proof/shipping-app.png
//
// The WebSocket client is hand-rolled for the same reason frontend/tests/cdp.mjs
// hand-rolls one: this Node has no global WebSocket and the app has no `ws`
// dependency to borrow.

const net = require('net')
const http = require('http')
const crypto = require('crypto')
const fs = require('fs')
const path = require('path')

const GUID = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11'

function arg (name, fallback) {
  const i = process.argv.indexOf('--' + name)
  return i >= 0 && process.argv[i + 1] ? process.argv[i + 1] : fallback
}

function httpJson (url) {
  return new Promise((resolve, reject) => {
    http.get(url, (res) => {
      let b = ''
      res.on('data', (c) => { b += c })
      res.on('end', () => {
        try { resolve(JSON.parse(b)) } catch (e) { reject(new Error(`bad JSON from ${url}: ${b.slice(0, 200)}`)) }
      })
    }).on('error', reject)
  })
}

class Ws {
  constructor (socket) {
    this.socket = socket
    this.buf = Buffer.alloc(0)
    this.pending = new Map()
    this.nextId = 1
    socket.on('data', (chunk) => {
      this.buf = Buffer.concat([this.buf, chunk])
      this.drain()
    })
    socket.on('error', () => {})
  }

  static connect (wsUrl) {
    const u = new URL(wsUrl)
    return new Promise((resolve, reject) => {
      const key = crypto.randomBytes(16).toString('base64')
      const socket = net.connect(Number(u.port), u.hostname, () => {
        socket.write(
          `GET ${u.pathname} HTTP/1.1\r\n` +
          `Host: ${u.host}\r\n` +
          'Upgrade: websocket\r\n' +
          'Connection: Upgrade\r\n' +
          `Sec-WebSocket-Key: ${key}\r\n` +
          'Sec-WebSocket-Version: 13\r\n\r\n'
        )
      })
      socket.once('error', reject)
      let handshake = Buffer.alloc(0)
      const onData = (chunk) => {
        handshake = Buffer.concat([handshake, chunk])
        const end = handshake.indexOf('\r\n\r\n')
        if (end < 0) return
        const head = handshake.slice(0, end).toString('utf8')
        const accept = crypto.createHash('sha1').update(key + GUID).digest('base64')
        if (!head.includes('101') || !head.includes(accept)) {
          reject(new Error('websocket handshake rejected: ' + head.split('\r\n')[0]))
          return
        }
        socket.removeListener('data', onData)
        const ws = new Ws(socket)
        const rest = handshake.slice(end + 4)
        if (rest.length) {
          ws.buf = rest
          ws.drain()
        }
        resolve(ws)
      }
      socket.on('data', onData)
    })
  }

  drain () {
    for (;;) {
      const frame = this.readFrame()
      if (!frame) return
      if (frame.opcode === 1) {
        let msg
        try { msg = JSON.parse(frame.payload.toString('utf8')) } catch { continue }
        if (msg.id && this.pending.has(msg.id)) {
          const { resolve, reject } = this.pending.get(msg.id)
          this.pending.delete(msg.id)
          if (msg.error) reject(new Error(msg.error.message))
          else resolve(msg.result)
        }
      } else if (frame.opcode === 8) {
        this.socket.end()
      }
    }
  }

  readFrame () {
    const b = this.buf
    if (b.length < 2) return null
    const opcode = b[0] & 0x0f
    const masked = (b[1] & 0x80) !== 0
    let len = b[1] & 0x7f
    let at = 2
    if (len === 126) {
      if (b.length < at + 2) return null
      len = b.readUInt16BE(at); at += 2
    } else if (len === 127) {
      if (b.length < at + 8) return null
      len = Number(b.readBigUInt64BE(at)); at += 8
    }
    let mask = null
    if (masked) {
      if (b.length < at + 4) return null
      mask = b.slice(at, at + 4); at += 4
    }
    if (b.length < at + len) return null
    const payload = Buffer.from(b.slice(at, at + len))
    if (mask) for (let i = 0; i < payload.length; i++) payload[i] ^= mask[i % 4]
    this.buf = b.slice(at + len)
    return { opcode, payload }
  }

  send (method, params = {}) {
    const id = this.nextId++
    const payload = Buffer.from(JSON.stringify({ id, method, params }), 'utf8')
    const mask = crypto.randomBytes(4)
    const n = payload.length
    let header
    if (n < 126) {
      header = Buffer.alloc(6)
      header[1] = 0x80 | n
      mask.copy(header, 2)
    } else if (n < 65536) {
      header = Buffer.alloc(8)
      header[1] = 0x80 | 126
      header.writeUInt16BE(n, 2)
      mask.copy(header, 4)
    } else {
      header = Buffer.alloc(14)
      header[1] = 0x80 | 127
      header.writeBigUInt64BE(BigInt(n), 2)
      mask.copy(header, 10)
    }
    header[0] = 0x81
    for (let i = 0; i < n; i++) payload[i] ^= mask[i % 4]
    this.socket.write(Buffer.concat([header, payload]))
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      setTimeout(() => {
        if (this.pending.delete(id)) reject(new Error(`${method} timed out`))
      }, 20000)
    })
  }
}

async function main () {
  const port = Number(arg('port', '9333'))
  const out = path.resolve(arg('out', 'proof/shipping-app.png'))
  const targets = await httpJson(`http://127.0.0.1:${port}/json/list`)
  const page = targets.find((t) => t.type === 'page')
  if (!page) throw new Error('no page target on the debug port: ' + JSON.stringify(targets).slice(0, 300))

  const ws = await Ws.connect(page.webSocketDebuggerUrl)
  const evaluate = async (expr) => {
    const r = await ws.send('Runtime.evaluate', { expression: expr, returnByValue: true })
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.text)
    return r.result.value
  }

  const report = {
    debugPort: port,
    targetUrl: page.url,
    targetTitle: page.title,
    dom: await evaluate(`(() => ({
      segments: document.querySelectorAll('.seg[data-seq]').length,
      paliLines: document.querySelectorAll('.seg-pali').length,
      bookTitle: (document.querySelector('.booktitle') || {}).textContent || '',
      clickableWords: document.querySelectorAll('.seg-pali .w').length,
      firstPali: (document.querySelector('.seg-pali') || {}).innerText || '',
      appChildren: document.querySelector('#app') ? document.querySelector('#app').children.length : 0,
      location: location.href,
    }))()`),
  }
  report.dom.diacriticsSeen = Array.from(new Set((report.dom.firstPali.match(/[āīūṭḍṇṃḷ]/g) || []))).join('')
  report.dom.sample = await evaluate(
    "Array.from(document.querySelectorAll('.seg-pali')).slice(0,3).map(e=>e.innerText).join('\\n---\\n')"
  )

  const shot = await ws.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
  fs.mkdirSync(path.dirname(out), { recursive: true })
  fs.writeFileSync(out, Buffer.from(shot.data, 'base64'))
  report.screenshot = { file: out, bytes: fs.statSync(out).size }

  console.log(JSON.stringify(report, null, 2))
  ws.socket.end()
}

main().catch((err) => {
  console.error('cdp-probe failed:', err.message)
  process.exit(1)
})
