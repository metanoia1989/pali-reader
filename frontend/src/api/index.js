// One place that knows how to talk to the API.
//
// Every call goes through request(), which attaches the session token, turns a
// non-2xx response into a thrown Error carrying the server's own message, and
// signals a lost session once so the app can react in a single place.

const TOKEN_KEY = 'pali.token'

export function getToken() {
  try {
    return localStorage.getItem(TOKEN_KEY) || ''
  } catch {
    return ''
  }
}

export function setToken(t) {
  try {
    if (t) localStorage.setItem(TOKEN_KEY, t)
    else localStorage.removeItem(TOKEN_KEY)
  } catch {
    /* private mode: the session simply does not survive a reload */
  }
}

let onUnauthorized = null
export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn
}

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message || code || 'request failed')
    this.status = status
    this.code = code
  }
}

async function request(method, path, { body, query, raw } = {}) {
  let url = '/api' + path
  if (query) {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(query)) {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
    }
    const s = qs.toString()
    if (s) url += '?' + s
  }

  const headers = {}
  const token = getToken()
  if (token) headers.Authorization = 'Bearer ' + token
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  let res
  try {
    res = await fetch(url, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (e) {
    throw new ApiError(0, 'network', '无法连接服务器，请检查网络')
  }

  if (res.status === 401) {
    setToken('')
    if (onUnauthorized) onUnauthorized()
    throw new ApiError(401, 'unauthorized', '登录已过期，请重新登录')
  }

  if (!res.ok) {
    let code = 'http_' + res.status
    let msg = '请求失败'
    try {
      const j = await res.json()
      if (j && j.error) code = j.error
      if (j && j.msg) msg = j.msg
    } catch {
      /* a non-JSON error body is still an error */
    }
    if (res.status === 429) msg = msg || '操作过于频繁，请稍后再试'
    throw new ApiError(res.status, code, msg)
  }

  if (raw) return res
  if (res.status === 204) return null
  const text = await res.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

const get = (p, query) => request('GET', p, { query })
const post = (p, body) => request('POST', p, { body })
const put = (p, body) => request('PUT', p, { body })
const patch = (p, body) => request('PATCH', p, { body })
const del = (p, query) => request('DELETE', p, { query })

export const api = {
  // --- session ---------------------------------------------------------
  register: (email, password, name) => post('/auth/register', { email, password, name }),
  verify: (email, code) => post('/auth/verify', { email, code }),
  resend: (email) => post('/auth/resend', { email }),
  login: (email, password) => post('/auth/login', { email, password }),
  logout: () => post('/auth/logout'),
  me: () => get('/auth/me'),
  updateMe: (payload) => patch('/auth/me', payload),

  // --- corpus ----------------------------------------------------------
  catalog: () => get('/catalog'),
  stats: () => get('/stats'),
  book: (id) => get(`/books/${encodeURIComponent(id)}`),
  segments: (id, from, count) => get(`/books/${encodeURIComponent(id)}/segments`, { from, count }),
  marks: (id, from, to) => get(`/books/${encodeURIComponent(id)}/marks`, { from, to }),
  search: (q, opts = {}) => get('/search', { q, ...opts }),
  searchBook: (id, q, opts = {}) =>
    get(`/books/${encodeURIComponent(id)}/search`, { q, ...opts }),
  // Names rather than text: volumes, and the suttas and chapters inside them,
  // each in Pāḷi and in whatever languages the reader has translations for.
  searchTitles: (q, opts = {}) => get('/search/titles', { q, ...opts }),
  // The published translations of one book. Scoped to the book because that is
  // what the key on ref_translations can answer quickly; see the handler.
  searchRefs: (id, q, opts = {}) =>
    get(`/books/${encodeURIComponent(id)}/refs/search`, { q, ...opts }),

  // --- dictionary ------------------------------------------------------
  lookup: (word) => get('/dict/lookup', { word }),
  // The English dictionary behind the 参考译文 popup: meanings of one English
  // word, and nothing the Pāḷi panel needs. A separate endpoint so the popup
  // never pays for an analysis it does not draw — see the handler.
  enLookup: (word) => get('/dict/en/lookup', { word }),
  suggest: (q, limit) => get('/dict/suggest', { q, limit }),
  declension: (pattern, form) => get('/dict/declension', { pattern, form }),

  // --- the reader's own work -------------------------------------------
  addPick: (payload) => post('/work/picks', payload),
  deletePick: (bookId, segment, wordIndex, key) =>
    del('/work/picks', key ? { bookId, segment, wordIndex, key } : { bookId, segment, wordIndex }),
  putNote: (payload) => put('/work/notes', payload),
  deleteNote: (id) => del(`/work/notes/${id}`),
  putTranslation: (payload) => put('/work/translations', payload),
  putProgress: (bookId, segment) => put('/work/progress', { bookId, segment }),
  progress: () => get('/work/progress'),
  vocab: (q) => get('/work/vocab', { q }),
  addVocab: (payload) => post('/work/vocab', payload),
  removeVocab: (lemma) => del(`/work/vocab/${encodeURIComponent(lemma)}`),
  bookmarks: () => get('/work/bookmarks'),
  putBookmark: (payload) => put('/work/bookmarks', payload),
  deleteBookmark: (id) => del(`/work/bookmarks/${id}`),
  settings: () => get('/work/settings'),
  putSettings: (payload) => put('/work/settings', payload),
  exportUrl: '/api/work/export',
}
