'use strict'

// A tiny TTL + LRU cache for whole HTTP responses held in the main process.
//
// Why not just let Chromium's disk cache do it? Because the point is to make
// "open the next book" instant even on the very first visit to that book in
// this session, and to answer before Chromium has had a chance to revalidate.
// The disk cache still runs underneath for everything we do not hold here.

class ResponseCache {
  /**
   * @param {{ttlMs?:number, maxBytes?:number, maxEntries?:number}} opts
   */
  constructor ({ ttlMs = 0, maxBytes = 64 * 1024 * 1024, maxEntries = 512 } = {}) {
    this.ttlMs = ttlMs
    this.maxBytes = maxBytes
    this.maxEntries = maxEntries
    this.map = new Map() // key -> {body, status, headers, storedAt, bytes}
    this.bytes = 0
    this.hits = 0
    this.misses = 0
    this.stores = 0
    this.evictions = 0
  }

  get enabled () {
    return this.ttlMs > 0
  }

  /** Returns the entry (and marks it most-recently-used) or null. */
  get (key) {
    const entry = this.map.get(key)
    if (!entry) {
      this.misses++
      return null
    }
    if (Date.now() - entry.storedAt > this.ttlMs) {
      this.delete(key)
      this.misses++
      return null
    }
    // Refresh recency: Map preserves insertion order, so re-insert.
    this.map.delete(key)
    this.map.set(key, entry)
    this.hits++
    return entry
  }

  set (key, { body, status, headers }) {
    if (!this.enabled) return
    const bytes = body.length
    // A single oversized response would blow the whole budget; skip it rather
    // than evicting everything else to make room.
    if (bytes > this.maxBytes) return
    const existing = this.map.get(key)
    if (existing) this.bytes -= existing.bytes
    this.map.delete(key)
    this.map.set(key, { body, status, headers, bytes, storedAt: Date.now() })
    this.bytes += bytes
    this.stores++
    this.evict()
  }

  delete (key) {
    const entry = this.map.get(key)
    if (!entry) return
    this.map.delete(key)
    this.bytes -= entry.bytes
  }

  evict () {
    while ((this.bytes > this.maxBytes || this.map.size > this.maxEntries) && this.map.size > 0) {
      const oldest = this.map.keys().next().value
      this.delete(oldest)
      this.evictions++
    }
  }

  clear () {
    this.map.clear()
    this.bytes = 0
  }

  stats () {
    return {
      enabled: this.enabled,
      ttlMs: this.ttlMs,
      entries: this.map.size,
      bytes: this.bytes,
      hits: this.hits,
      misses: this.misses,
      stores: this.stores,
      evictions: this.evictions,
    }
  }
}

module.exports = { ResponseCache }
