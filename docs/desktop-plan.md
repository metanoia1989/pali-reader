# 桌面版方案（未开工 — 交给 agent 执行）

这份文件是一份**可以直接执行的方案**。写它的原因：2026-10 量过一轮打包体积，结论已经清楚了，
但当时决定不做。等要做的时候，把这个文件交给 agent 即可，不需要重新调研。

**所有数字都标了来源**：`实测` = 当时真跑出来的，`估` = 推算。重新做之前不要把这些当成承诺，
但也不要重新量一遍——量过的东西在下面。

---

## 一、目标

一个本地的 Pāḷi 三藏阅读器：双击打开，**不依赖任何服务器**，语料随程序走。
今天它是一个 Vue 前端 + 服务器上的 Go/MySQL/Redis（`pali.bigubuntu.internal`）。

## 二、已经存在的东西

`pali-reader/desktop/` —— **一个能跑的 Electron 应用**，上一轮建的，别从零开始：

```
desktop/
  package.json          electron 34.5.8（devDependency），npm start / smoke / measure / offline-demo / pack
  src/main.js           主进程、窗口、生命周期
  src/server.js         本机 loopback 静态服务 + /api 反向代理
  src/cache.js          主进程内 TTL+LRU（10 分钟 / 64MB）
  src/config.js         后端地址：PALI_API 环境变量 > userData/config.json > 默认值
  src/pages/offline.html  后端不可达时的可读错误页
  scripts/start.js      清掉 ELECTRON_RUN_AS_NODE 再拉起
  scripts/pack.js       不依赖 electron-builder 的打包
  tools/ measure.js  cdp-probe.js
  test/smoke.js         41 项服务级检查，不需要 Electron
  proof/                截图与测量 JSON
```

它的做法（**保留，不要推翻**）：主进程起**一个 loopback HTTP 服务**（`listen(0)`，只绑 127.0.0.1，
拒绝非 loopback 的 Host），既从磁盘供 `frontend/dist`，又把 `/api/*` 反向代理到后端。
**同源，所以 `frontend/src/api/index.js:40` 的相对路径 `'/api' + path` 一行都不用改。**

要改成自带语料的版本，就是把「代理到远端」换成「查本地 SQLite」，其余结构不动。

---

## 三、体积（已量，别重量）

| 项 | 磁盘 | 压缩后 | 来源 |
|---|---|---|---|
| Electron 外壳（现状，**不裁剪**） | 233 MB | 91.1 MB | 实测 |
| `frontend/dist` —— 真正的应用 | **272 KB** | | 实测 |
| 语料（SQLite，自带） | ~1,020 MB | ~250 MB | 混合 |
| **全包安装** | | **~341 MB** | 混合 |

应用自己的文件只有 **412 KB**（其中 `frontend/dist` 344 KB）。
**包里没有 `node_modules`** —— 上一轮的报告说「大部分是拷进去的 node_modules」，那是错的，
`pack.js` 只拷 Electron 自己的预编译 `Electron.app`、`frontend/dist`、`desktop/src`。
`desktop/` 目录显示 595 MB 是开发残留（`node_modules` 251 + Electron 压缩包缓存 103 + `build` 233），
**一样都不进包**。

### 明确决定：**不裁剪外壳**

裁掉语言包（55 → en/zh_CN/zh_TW，省 39.6 MB）、SwiftShader（15.8 MB）、crashpad（1.0 MB），
一共省 56.7 MB（233 → 177 MB）。**不做**，因为：

- 它会**破坏代码签名** —— 框架的 CodeDirectory 封了 37,560 个哈希，手工裁完必须重新签名
  （实测复现了 `pack.js` 里记录的 `--deep` 报错：*unsealed contents present in the root
  directory of an embedded framework*）
- 省下的 56.7 MB 相对 341 MB 的安装包是 17%，而下面第四节那个 `word_freq` 改动
  **几行代码就省 150 MB**

另两条常见的裁剪在这**省不到东西**，别白费力气：`--strip` 已经做过（只剩 5,202 个动态符号），
架构瘦身也没用（已经是 arm64-only）。asar 只值 ~0.1 MB，不值得。

---

## 四、先做这个：`word_freq` 无损瘦身（几行代码，省 150 MB）

**与桌面版无关，对现在的网页版立刻有用。**

`word_freq` 现在 **159 MB**，只用在三处：

- 「语料 N 次」小标签（`frontend/src/components/WordLookup.vue:318`）
- 生词本的罕见度排序（`frontend/src/views/VocabView.vue:30`）
- 首页的语料词数（`backend/internal/server/catalog.go:315`）

**879,160 个词里有 494,679 个（56%）只出现一次。** 所以：

> **只存 count ≥ 2 的行，查不到就当作 1 —— 每个计数仍然精确，完全无损。**

实测结果：**159 MB → 8.4 MB**（19 倍），压缩后 2.9 MB。

顺带：`rank` 列是纯冗余（它就是按 count 排序后的位置），去掉；两个二级索引也去掉。

改完要重导 `freq` 步并清缓存（见第七节）。

---

## 五、自带语料：装 **derived 数据**，不装源库

服务器上有两套：

| | 大小 |
|---|---|
| MySQL 里的 derived 数据（`pali_reading` 库） | **1,764 MB** |
| `sources/*.db` 源库（SQLite） | 2,565 MB |

**装 derived 数据，不装源库。** 源库是导入器的输入（`dpd.db`、`epitaka*.db`、`tipitaka_pali.db`），
装它就意味着后端要去读别人的 schema —— 那**既是重写又更大**。装 derived 数据只需让现有的
`cmd/importer` 对着一个 SQLite 驱动跑一遍：**GORM 本来就支持 SQLite，`AutoMigrate` 也已经定义了
schema**，所以这是构建步骤的改动，不是重写。

### SQLite 比 MySQL 小得多（实测）

同一份数据：`text_segments` 654.6 → **415.0 MB**（**0.63×**）；`ref_translations` 每行
289.7 → 178.8 字节（**1.6×**）。省的是 InnoDB 页开销和两个冗余索引。
所以本地那份**不是把 MySQL 的数字换个标签**。

### 分层与体积

| 层 | SQLite | 压缩后 | 装进安装包还是首次下载 |
|---|---|---|---|
| 经文本体 `text_segments` | ~450–500 MB | **74.1 MB** | **安装包**（不可减） |
| 目录 `text_toc` | ~6 MB | ~1.5 MB | 安装包 |
| 词典（lookup + headwords + entries） | ~220 MB（估） | ~55–70 MB（估） | **首次下载**，可只给子集 |
| 参考译文（中 + 英） | ~330 MB | **101.1 MB** | **首次下载，按语言分开** |
| `word_freq`（第四节之后） | **8.4 MB** | **2.9 MB** | 安装包 |
| **合计** | **~1,020 MB** | **~250 MB** | |

`text_segments` 那一行：415 MB 是在 **342,121 行**上实测的，线上是 **1,040,017 行**
（句子 / 偈颂切分之后），所以 ~450–500 MB 是估的。文本字节完全相同，多出来的是行 / 索引开销。
实测的载荷构成：Text 98.0 + HTML 108.1 + Tokens 88.3 + Markers 1.6 + Variants 0.9 + Bold 3.1
= **300.0 MB**，9,250,177 个 token。
（顺带：整部三藏纯文本是 116.0 MB，zstd → 29.5 MB，xz -9 → 17.5 MB。）

**如果嫌 `HTML` 那 108 MB 大**：它是可以从 Text + Variants + Bold 重新生成的。
阅读器运行期只读 `text` / `tokens` / `bold` / `variants`，**从不读 `html`**（这一点已确认），
所以本地版可以整个不存 `html` 列。

---

## 六、参考译文按语言分包下载 —— 参考实现已经这么做了，照搬

`tipitaka-pali-reader`（`bksubhuti/tipitaka-pali-reader`，`master`）的做法**已读源码确认**：

- `lib/ui/screens/settings/download_service.dart` / `language_installer.dart`
- `releaseUrl = 'https://github.com/dhammanana/epitaka_app/releases/download/latest'`
- `archiveName => 'epitaka_$code.zip'`，`fileName => 'lang_$code.db'`
- 流程：下载 zip → 解包出完整的 ePitaka 库 → 跑 `_copySentences` 把**只有句子**的部分
  写进 `lang_<code>.db`（`WITHOUT ROWID`，主键 `(book_id, para_id, line_id)`，只留非空译文）
  → **删掉原文件**
- 运行时 `ATTACH` 成 schema `lang_<code>`（`database_helper.dart:297`），
  按 `_key(paraId, lineId)` join（`sentence_page_content_repo.dart:211`）
- 提供 14 种语言，发布 56 个资产

**用本地文件原样重跑过它的 `_copySentences`**：

```
lang_en.db = 181.1 MB     （与它自报的 181 MB 一致）
lang_zh.db = 149.1 MB
压缩后 50.3 + 50.8 = 101.1 MB
```

它发布的其他资产（从 release API 实测）：`epitaka_en.zip` 118 MB、`epitaka_zh.zip` 66.9 MB、
`epitaka.zip`（巴利）166 MB、`dpd-dictionary.zip` 49.9 MB、`embeddings.zip` 390 MB；
Linux 安装包 177 MB —— 也就是**装完中英要下载 528 MB**。我们的格式同样内容更小，
因为只装句子文本、不装整个 ePitaka 库。

### 首次下载量

| 下载 | 我们 | 100 Mbps | 20 Mbps |
|---|---|---|---|
| 中文译文 | 50.8 MB | ~5 秒 | ~25 秒 |
| 英文译文 | 50.3 MB | ~5 秒 | ~25 秒 |
| 词典 | ~55–70 MB | ~6 秒 | ~30 秒 |
| **可选部分合计** | **~156–171 MB** | **~15 秒** | **~70 秒** |

**压缩率**（zstd -3，逐文件实测）：SQLite 文本类 **3.6–5.6:1**；
Electron 二进制只有 **2.6:1**（这就是外壳在下载量里的占比比在磁盘上更显眼的原因）。

---

## 七、动手步骤

1. **先做第四节** —— `word_freq` 只存 count ≥ 2、去掉 `rank` 和二级索引。
   改 `backend/internal/importer/freq.go` 与 `internal/store` 里的模型，
   重导 `freq`、清缓存，确认那三处显示不变。**这一步对网页版也立刻有用，先做、先发。**
2. **让 `cmd/importer` 支持 SQLite 目标** —— GORM 已支持，`AutoMigrate` 已有 schema，
   主要是构建步骤加一个驱动和一条 `-driver sqlite -dsn <path>` 路径。
   可选：本地版不建 `html` 列。
3. **改 `desktop/src/server.js`** —— `/api/*` 不再代理到远端，改为查本地 SQLite。
   保持 loopback + 同源的形状，**前端一行不改**（`frontend/src/api/index.js:40` 是相对路径）。
4. **做语言包** —— 导入时按语言导出 `lang_<code>.db`（只有句子，键
   `(book_id, para_id, line_id)`），zstd 压缩，放到一个可下载的位置。
5. **首次运行流程** —— 装完只有经文 + 目录 + `word_freq`；词典与各语言译文按需下载，
   下载完 `ATTACH` 或等效地挂进本地库。参考实现就是这么做的，照着走。
6. **保留 `desktop/` 已有的东西** —— 离线错误页、单实例锁、`ELECTRON_RUN_AS_NODE` 的清理、
   41 项 smoke 检查、`cdp-probe.js` 的验证手法。

---

## 八、已经踩过的坑（别重踩）

- **`deploy/deploy.sh backend` 不发 `pali-importer`。** 它只发 API 二进制。改导入器必须单独
  `go build` + `scp` + `install`，并核对两边的 MD5 —— 这一点已经让人白跑过两次二十分钟的导入。
- **`pkill -f pali-importer` 会杀掉自己那条命令**（远程命令行里含这个字符串），
  导致 ssh 返回 255 而导入确实停了。用 `pgrep -x` 或 `[p]ali-importer` 这种不自匹配的写法。
- **重导之后必须清缓存**：`./pali-reader -flush-cache`。段落号会变，陈旧窗口会让读者看到上一版的分段。
- **这个构建沙箱里 Chromium 的沙箱起不来**，验证要用 `PALI_NO_SANDBOX=1`（默认仍是 `sandbox: true`）。
- **`ELECTRON_RUN_AS_NODE=1`** 在这个 shell 环境里是导出的，会让 Electron 当纯 Node 跑并崩在
  `Cannot read properties of undefined (reading 'setPath')`。`scripts/start.js` 会把它从子进程环境里删掉。
- **随机端口导致 localStorage 按 origin 分开**，所以登录态**跨启动不保留**。要持久登录得固定端口
  或把 token 放进 `userData`。
- **本机测不了** —— 没有 MySQL/Redis，也不要在本机起服务。所有运行期验证在 `bigubuntu` 上做，
  或者用 `frontend/tests/cdp.mjs`（手写的 CDP 客户端，`--no-proxy-server`，端口随机）。

## 九、明确**不建议**做的

**不要移植到 Tauri。** 它能省 ~83 MB 压缩体积（外壳 91.1 → ~8 MB），代价是：

- Rust 重写 `desktop/src/server.js`（静态服务 + 代理 + 缓存）
- Rust 工具链、分平台构建、分平台签名
- `setProxy({mode:'direct'})` 那个绕开本机代理的技巧要换一个等效 API（存在，但不是照抄）

而**同样的 150 MB 就在数据里躺着，几行代码就能拿到**（第四节）。
外壳占全包下载量的 **27%**，不是零头，但也远不是主导项。

（当时量过：无依赖的 Rust 版静态服务 + 代理**编译出来 320 KB**，7 秒编完，对着真实 dist 验证通过。
所以真要移植，最难的部分没有想象中大 —— 但收益仍然不划算。）

## 十、没量准的

1. **148 MB 那个框架二进制里 Chromium / Node 各占多少** —— Mach-O 不带符号大小，
   `~120 / ~25 MB` 是拿本机独立 Node 二进制（78.3 MB）推的。
2. **Tauri 运行时的真实大小** —— 沙箱里 `~/.cargo` 不可写，`~8 MB` 是估的。
   但**应用代码那部分 320 KB 是实测的**。
3. **裁掉之后的应用能不能启动** —— 没验证成功，因为**未裁剪的原包在当时的 shell 里也是立刻退出**
   （没有 GUI session）。当时把这个当作对照如实报了，没有当成通过。**不裁剪的话这一条不重要。**
4. **词典那一层（~220 MB）** —— 唯一没有在本地重建的层，是估的，与参考实现实测的
   `dpd-dictionary.zip` 49.9 MB 互相印证过。
5. **`text_segments` 在线上行数下的体积** —— 415 MB 是 342,121 行上实测的，
   线上 1,040,017 行下 ~450–500 MB 是估的。

---

## 附：一句话总结

**不裁外壳、不换框架；先把 `word_freq` 无损瘦身（省 150 MB，网页版也受益）；
自带语料装 SQLite 里的 derived 数据（~1 GB 磁盘 / ~250 MB 压缩）；译文与词典按语言/按需下载
（参考实现已验证这条路的可行性）。**
