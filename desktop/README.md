# 巴利三藏阅读器 · 桌面版（macOS）

把已经部署在 `http://pali.bigubuntu.internal/` 的阅读器装进一个 Electron 应用，
不用开浏览器。

它**不构建任何东西**：`npm start` 只是把已经构建好的 `../frontend/dist` 从本地磁盘端出来，
再把 `/api/*` 转发给后端。前端一行没改，`/api` 仍然是相对路径。

```
desktop/
├── package.json           仅一个依赖：electron（devDependency）
├── src/
│   ├── main.js            出品入口：单实例、生命周期、启动路由参数
│   ├── app.js             两边共用的引导：路径、配置、安全策略、窗口、菜单
│   ├── server.js          回环 HTTP 服务：静态端盘 + /api 反向代理 + 语料缓存
│   ├── cache.js           TTL + LRU 的响应缓存
│   ├── config.js          后端地址的解析与落盘
│   └── pages/
│       ├── offline.html   连不上后端时显示的页面
│       └── inject.js      注入到 index.html 的「后端掉线」横幅
├── scripts/
│   ├── start.js           npm start 的入口：先检查，再拉起 Electron
│   └── pack.js            打成 .app（不引入 electron-builder）
├── tools/
│   ├── measure.js         计时 + 截图（走的是和出品完全相同的引导代码）
│   └── cdp-probe.js       从 Chromium 调试口独立验证窗口里真的渲染了巴利语
├── test/smoke.js          29 项纯 Node 检查：路由、头部、缓存规则、故障路径
└── proof/                 measure / cdp-probe 产出的截图与 JSON
```

## 跑起来

```bash
cd pali-reader/frontend && npm install && npm run build   # 只需要一次（或前端有更新时）
cd ../desktop && npm install
npm start
```

直接打开某部经：

```bash
npm start -- /read/mula_vi_01
```

`frontend/dist` 不存在时 `npm start` 会直接说清楚缺哪个目录、怎么生成，而不是抛一堆
Electron 的栈。

其他命令：

| 命令 | 作用 |
| --- | --- |
| `npm run smoke` | 29 项服务层检查（不需要 Electron） |
| `npm run measure` | 计时 + 截图，结果写进 `proof/` |
| `npm run offline-demo` | 把后端指向死端口，验证错误页 |
| `npm run pack` | 打成 `build/Pāḷi Reader.app` |

## 各部分的接法

**谁端什么。** 主进程里起一个只监听 `127.0.0.1`、端口随机（`listen(0)`）的 `http` 服务：

- `/`、`/assets/*`、`/read/xxx`…… → 从 `../frontend/dist` 读盘。`/assets/*` 是 Vite
  加了哈希的，带 `max-age=31536000, immutable`；`index.html` 带 `no-cache`，所以前端
  重新构建后一定能拿到新的。认不出的路径按 SPA 回退到 `index.html`。
- `/api/*` → 反向代理到后端，请求体与响应体都是流式转发，不整体缓冲。
- `/__desktop/*` → 状态、健康探测、错误页、注入脚本。

**关键一点**：外壳和 `/api` 同源，所以前端里那句 `let url = '/api' + path`
（`frontend/src/api/index.js:40`）原样可用。

**代理为什么必须是 direct。** 这台机器的系统代理（`~/.curlrc` 里的
`http://127.0.0.1:7897`）进不了 `*.internal`。窗口加载前会在
`session.defaultSession` 上执行 `setProxy({ mode: 'direct' })`，绕开系统代理直连。
`PALI_PROXY=system` 可以退回系统代理。

**缓存住在哪。** 三处，互相不冲突：

1. **主进程内的响应缓存**（`src/cache.js`）：只针对 `GET /api/catalog`、
   `GET /api/books/<id>`、`GET /api/books/<id>/segments` —— 语料数据，对所有读者都一样、
   只有重导语料才会变。默认 TTL 10 分钟，上限 64MB / 512 条，LRU 淘汰。
   `/api/books/<id>/marks`（读者自己的批注）和 `/search` **明确排除**，绝不缓存；
   `Authorization` 参与缓存键，登录用户不会读到别人的响应。
   为了缓存里的字节可以原样回放给任何客户端，这几个请求强制
   `accept-encoding: identity`，不会出现「gzip 的字节配 identity 的头」。
   命中时响应带 `x-pali-cache: hit`。
2. **Chromium 自己的 HTTP 缓存**：没有特殊处理，一切照旧，仍在给没被上面缓存的东西干活。
   它的 `sessionData` 被放进 userData，这样缓存跟着应用数据走，重启后仍然有效。
3. **后端自己的 Redis 缓存**：没动。

**后端地址怎么定。** 优先级从高到低：

1. 环境变量 `PALI_API`
2. `<userData>/config.json` 里的 `apiBase`
3. 内置默认 `http://pali.bigubuntu.internal`

`config.json` 在第一次运行时自动写好，里面就有注释字段和两个候选地址，改完重启即可：

```json
{
  "apiBase": "http://pali.bigubuntu.internal",
  "cacheTtlMs": 600000
}
```

```bash
PALI_API=http://localhost:8099 npm start     # 临时走 SSH 隧道
```

`<userData>` 在 macOS 上是 `~/Library/Application Support/Pāḷi Reader/`
（`PALI_USER_DATA=<dir>` 可以改到别处）。

**后端掉线时。** 窗口显示之前先探一次 `/api/health`（3.5 秒超时）：

- 不通 → 直接加载 `/__desktop/offline`，页面上写清当前后端地址、这个地址是从哪来的、
  失败原因，有一个「重试」按钮，并且每 5 秒自动重试一次，后端一回来就自己进主界面。
- 通 → 正常加载。之后如果后端中途掉线，注入的 `inject.js` 会在页面顶部挂一条横幅
  （后端地址 + 失败原因 + 立即重试），而不是让读者面对一个沉默的页面。

**安全设置。** `contextIsolation: true`、`nodeIntegration: false`、`sandbox: true`、
没有 preload（页面和自己同源，不需要从 Node 拿任何东西）、`webSecurity` 保持默认。
`setWindowOpenHandler` 与 `will-navigate` 只放行自己的回环源，外链交给系统浏览器；
`will-attach-webview` 一律拒绝；权限请求只放行 `clipboard-sanitized-write`
（查词面板的复制要用）。HTTP 服务只绑 `127.0.0.1`，并拒绝非回环的 `Host` 头，
免得被 DNS rebinding 当成跳板。

顺带一个副作用：`http://127.0.0.1` 在 Chromium 里算安全上下文，所以局域网 http 页面上
用不了的 `navigator.clipboard`（见 `WordLookup.vue` 里的注释）在这里是可用的。

## 环境变量一览

| 变量 | 作用 |
| --- | --- |
| `PALI_API` | 覆盖后端地址（优先级最高） |
| `PALI_CACHE_TTL_MS` | 覆盖缓存 TTL，`0` 关闭缓存 |
| `PALI_DIST` | 覆盖前端产物目录 |
| `PALI_USER_DATA` | 覆盖 userData 目录（配置、缓存、localStorage） |
| `PALI_PROXY` | `system` 时使用系统代理，默认 `direct` |
| `PALI_PROXY_TIMEOUT_MS` | 上游超时，默认 20000 |
| `PALI_LOG` | `1` 打开日志，`requests` 更啰嗦 |
| `PALI_NO_SANDBOX` | `1` 时给 Electron 加 `--no-sandbox`（见下） |
| `PALI_DEVTOOLS` | `1` 时打开开发者工具 |

## 已知限制

- **Chromium 的沙箱在另一个沙箱里起不来。** 在本机某些受限 shell / CI / 容器里会看到
  `sandbox initialization failed: Operation not permitted`，渲染进程反复重启、窗口空白。
  这是宿主环境不让嵌套 seatbelt，不是应用的问题；`PALI_NO_SANDBOX=1 npm start` 可以绕开。
  默认不开：正常桌面环境应该保留沙箱。
- **`ELECTRON_RUN_AS_NODE` 会让 Electron 变成普通 Node。** 有些 shell 会全局导出它，
  症状是启动就报 `Cannot read properties of undefined (reading 'setPath')`。
  `scripts/start.js` 会主动把它从子进程环境里删掉。
- **`.app` 里保留的是 Electron 自己的可执行文件名。** 显示名、Dock 名、bundle id 都改好了，
  但改可执行文件名意味着连四个 Helper bundle 一起改，那部分交给 electron-builder 更合适。
- **签名是 ad-hoc 的。** 本机自用没问题；要分发得换开发者证书。
- `desktop/.cache/electron/` 里放着一份 Electron 34.5.8 的 darwin-arm64 压缩包（95MB），
  是本机 `npm install` 时二进制下载失败后留下的离线兜底。不需要可以删。
