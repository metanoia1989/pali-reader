// Injected into index.html by the desktop server.
//
// Why this exists at all: the startup probe catches "the backend was down when
// the app opened". This catches the other half — the reader is halfway through
// a book and the LAN drops. The SPA would show its own per-request error inside
// whatever panel happened to ask, which is easy to miss; a banner at the top
// says the whole backend is gone.
//
// It only ever adds one element of its own and never reads or writes app state.

;(function () {
  var BANNER_ID = 'pali-desktop-offline-banner'
  var POLL_MS = 8000
  var last = null

  function mount (apiBase, error) {
    if (document.getElementById(BANNER_ID)) return
    var bar = document.createElement('div')
    bar.id = BANNER_ID
    bar.setAttribute('role', 'status')
    bar.style.cssText = [
      'position:fixed', 'top:0', 'left:0', 'right:0', 'z-index:2147483000',
      'display:flex', 'align-items:center', 'gap:12px', 'justify-content:center',
      'padding:7px 14px', 'background:#fdf6e6', 'border-bottom:1px solid #e6d9b8',
      'color:#6b5320', 'font-size:12.5px', 'line-height:1.5',
      'font-family:"Helvetica Neue",Helvetica,Arial,"PingFang SC","Hiragino Sans GB","Microsoft YaHei","Noto Sans CJK SC",sans-serif',
    ].join(';')
    var text = document.createElement('span')
    text.textContent = '后端 ' + apiBase + ' 连不上（' + error + '），正在自动重试…'
    var button = document.createElement('button')
    button.type = 'button'
    button.textContent = '立即重试'
    button.style.cssText = [
      'border:1px solid #d9c48c', 'background:#fff', 'color:#6b5320',
      'border-radius:4px', 'padding:2px 10px', 'font-size:12px', 'cursor:pointer',
      'font-family:inherit',
    ].join(';')
    button.addEventListener('click', function () {
      button.disabled = true
      button.textContent = '重试中…'
      fetch('/__desktop/health', { cache: 'no-store' })
        .then(function (r) { return r.json() })
        .then(function (j) { apply(j.ok ? { ok: true } : { ok: false, error: j.error, apiBase: j.apiBase }) })
        .catch(function () { button.disabled = false; button.textContent = '立即重试' })
    })
    bar.appendChild(text)
    bar.appendChild(button)
    document.body.appendChild(bar)
  }

  function unmount () {
    var el = document.getElementById(BANNER_ID)
    if (el) el.remove()
  }

  function apply (status) {
    if (!status || status.ok !== false) {
      unmount()
      return
    }
    mount(status.apiBase || '', status.error || '无响应')
    var el = document.getElementById(BANNER_ID)
    if (el) {
      el.lastChild.disabled = false
      el.lastChild.textContent = '立即重试'
    }
  }

  function poll () {
    fetch('/__desktop/status', { cache: 'no-store' })
      .then(function (r) { return r.json() })
      .then(function (j) {
        last = j
        apply(j)
      })
      .catch(function () { /* the shell server itself is gone; nothing to say */ })
  }

  // Let the first screen finish before adding a poller to the same origin.
  if (document.readyState === 'complete') setTimeout(poll, 2500)
  else window.addEventListener('load', function () { setTimeout(poll, 2500) })
  setInterval(poll, POLL_MS)
  window.__paliDesktopStatus = function () { return last }
})()
