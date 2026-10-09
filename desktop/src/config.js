'use strict'

// Where the backend lives, and how the owner changes it without a rebuild.
//
// Resolution order, highest priority first:
//   1. PALI_API environment variable   — one-off override, e.g. the SSH tunnel
//   2. apiBase in <userData>/config.json — the durable knob the owner edits
//   3. DEFAULT_API_BASE                 — the deployed LAN site
//
// The file is written on first run so that "how do I point this somewhere
// else?" has a visible answer on disk instead of only existing in a README.

const fs = require('fs')
const path = require('path')

const DEFAULT_API_BASE = 'http://pali.bigubuntu.internal'
const DEFAULT_CACHE_TTL_MS = 10 * 60 * 1000
const CONFIG_FILENAME = 'config.json'

/**
 * Accepts what a human would actually type ("pali.example.org", a URL with a
 * trailing slash, a URL with a path prefix) and returns a canonical
 * scheme://host[:port][/prefix] with no trailing slash, or null if unusable.
 */
function normalizeApiBase (raw) {
  if (typeof raw !== 'string') return null
  let s = raw.trim()
  if (!s) return null
  if (!/^https?:\/\//i.test(s)) s = 'http://' + s
  let u
  try {
    u = new URL(s)
  } catch {
    return null
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null
  if (!u.hostname) return null
  const pathname = u.pathname.replace(/\/+$/, '')
  return `${u.protocol}//${u.host}${pathname}`
}

function readConfigFile (configPath) {
  try {
    const raw = fs.readFileSync(configPath, 'utf8')
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') return { value: parsed, error: null }
    return { value: null, error: 'config.json 不是一个 JSON 对象' }
  } catch (err) {
    if (err.code === 'ENOENT') return { value: null, error: null }
    return { value: null, error: `无法读取 config.json：${err.message}` }
  }
}

function writeDefaultConfig (configPath, apiBase, cacheTtlMs) {
  try {
    fs.mkdirSync(path.dirname(configPath), { recursive: true })
    fs.writeFileSync(
      configPath,
      JSON.stringify(
        {
          _readme: [
            '把 apiBase 改成别的后端地址即可，改完重启应用生效。',
            '环境变量 PALI_API 优先级更高，适合临时指向 SSH 隧道。',
            `默认值：${DEFAULT_API_BASE}（局域网）或 http://localhost:8099（SSH 隧道）`,
          ],
          apiBase,
          cacheTtlMs,
        },
        null,
        2
      ) + '\n',
      'utf8'
    )
    return null
  } catch (err) {
    // A read-only home directory should degrade to the defaults, not crash the
    // app: the reader still works, it just cannot be reconfigured.
    return `无法写入 ${configPath}：${err.message}`
  }
}

/**
 * @returns {{apiBase:string, cacheTtlMs:number, source:string, configPath:string,
 *            requested:string|null, warnings:string[]}}
 */
function resolveConfig ({ userDataDir, env = process.env } = {}) {
  const warnings = []
  const configPath = path.join(userDataDir, CONFIG_FILENAME)
  const file = readConfigFile(configPath)
  if (file.error) warnings.push(file.error)

  let apiBase = null
  let source = ''
  let requested = null

  const fromEnv = env.PALI_API
  if (fromEnv != null && String(fromEnv).trim() !== '') {
    requested = String(fromEnv).trim()
    apiBase = normalizeApiBase(fromEnv)
    source = 'PALI_API 环境变量'
    if (!apiBase) warnings.push(`PALI_API 不是可用的地址，已忽略：${requested}`)
  }

  if (!apiBase && file.value && file.value.apiBase != null) {
    requested = String(file.value.apiBase).trim()
    apiBase = normalizeApiBase(requested)
    source = `配置文件 ${configPath}`
    if (!apiBase) warnings.push(`config.json 里的 apiBase 不是可用的地址，已回退到默认值：${requested}`)
  }

  if (!apiBase) {
    apiBase = DEFAULT_API_BASE
    source = '内置默认值'
    requested = null
  }

  let cacheTtlMs = DEFAULT_CACHE_TTL_MS
  const ttlFromEnv = Number(env.PALI_CACHE_TTL_MS)
  if (Number.isFinite(ttlFromEnv) && ttlFromEnv >= 0) {
    cacheTtlMs = ttlFromEnv
  } else if (file.value && Number.isFinite(Number(file.value.cacheTtlMs)) && Number(file.value.cacheTtlMs) >= 0) {
    cacheTtlMs = Number(file.value.cacheTtlMs)
  }

  return { apiBase, cacheTtlMs, source, configPath, requested, warnings, configFileExisted: fs.existsSync(configPath) }
}

module.exports = {
  DEFAULT_API_BASE,
  DEFAULT_CACHE_TTL_MS,
  CONFIG_FILENAME,
  normalizeApiBase,
  resolveConfig,
  writeDefaultConfig,
}
