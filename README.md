# 巴利三藏阅读器 · Pāḷi Tipiṭaka Reader

面向「大量阅读巴利原典」的中文读者：点击经文中的任一单词即可查词、拆解复合词、展开变格表，
选定释义与词性后写在原文之下，并为每一段留下批注、翻译与参考译文。

前端 Vue 3，后端 Go，数据 MySQL，缓存 Redis。部署在 `http://pali.bigubuntu.internal/`。

---

## 开发

```bash
cd frontend && npm run check    # 组件引用审计 + 服务端渲染断言（不需要浏览器）
```

## 阅读设置

顶栏齿轮图标打开，全部即时生效（数据两种语言都已随段落返回，切换不发请求），
并保存在浏览器本地；登录后同步到账号。

| 设置 | 选项 | 作用 |
| --- | --- | --- |
| 字号 | 小 / 标准 / 大 / 特大 | 正文、释义、译文、词典面板一起缩放，界面骨架不缩放 |
| 参考译文 | 隐藏 / 中文 / 英文 / 全部 | ePitaka 的中译与英译，逐段对齐后才显示 |
| 词典释义 | 全部 / 中先 / 英先 / 仅中 / 仅英 | 查词面板默认选中哪部词典 |
| 目录与书名 | 中文 / 巴利语 / 对照 | 典籍与分部的名称；章节标题无通行中译，一律保留巴利语 |
| 正文字体 | 衬线 / 无衬线 | 只影响正文 |
| 显示异读 | 关 / 开 | 第六次结集本行间校勘记，如 `[suriyaggāho (sī. syā. kaṃ. pī.)]` |

## 它解决什么问题

学了一点语法之后，读原典的瓶颈不是「不懂」，而是**每一次停顿都要花很久**：查一个词要换三个
地方——词形还原、看释义、找它属于哪个词干、看变格表、再判断这里的格。这个阅读器把这些压到
一次点击里，并且把你查过的结论**写回原文下方**，这样第二次读同一段时不用再查。

因此本项目的重点是三件事，其余都是围绕它们的配套设施：

偈颂按品分条、编号取自经文本身，注疏里被加粗的词头也照原样呈现。目录按
tipitaka-pali-reader 的做法呈现：**只写标题，不加自己的编号**，层级靠缩进与
字号表达。经文自己编号（`1. brahmajālasuttaṃ`），再补一个 `§` 就会变成两个数字并排。

1. **一次点击给出全部答案** —— 面板的顺序刻意如此：先回答「这个词可能是什么」，
   再回答「这个词条是什么意思」。
   - **词形分析表**：`pos | 性 | 格 | 数 | of | word`，一行一种读法，超过 5 行可展开全部
   - **拆解**：复合词拆成可点的部分
   - **词条**：一条一行、可展开；展开后是释义、可切换的其他词典、词条信息、
     **变格表**（词干正常字重 + 语尾墨蓝，命中格用底色标出）
   全部在一次请求里返回。
2. **结论可以留在原文里** —— 选中的释义、词性、拆解方式锚定在「第几卷第几段的第几个词」，
   以浅色显示在经文下方。
3. **批注与翻译** —— 逐段的批注、自己写的翻译，以及可对照的参考译文。

---

## 技术栈

| 层 | 技术 |
| --- | --- |
| 前端 | Vue 3 + Vue Router + Pinia + Vite，图标用 lucide |
| 后端 | Go 1.22，chi 路由，标准 `net/http` |
| ORM | GORM（模型即 schema，`AutoMigrate` 建表） |
| 数据库 | MySQL 5.7（语料、词典、用户数据） |
| 缓存 | Redis（语料段落、词条查询、会话） |
| 设计 | OpenDesign 的 **kami（紙／纸）** 设计系统：羊皮纸底、墨蓝单色、衬线主导 |

## 依赖的其他项目

这一节写给第一次接手的人：**哪些东西不是本仓库的**，从哪里来，以及——更重要的——
**哪些设计是从别人那里学来的**。最后一点比数据的出处更容易被忽略，却更容易走错。

### 一、参考实现：`tipitaka-pali-reader`

仓库 `bksubhuti/tipitaka-pali-reader`（Flutter，桌面与安卓同一套代码）。
**它不是数据源，是参考实现。** 本项目的阅读模型、数据模型和若干设计决定直接来自读它的源码，
不是从别处推出来的。凡是「为什么要这样做」的问题，先去读它，读了再改。

学到的东西，以及对应的证据：

| 学到什么 | 在它源码里的位置 | 本项目落在哪 |
| --- | --- | --- |
| **译文不做对齐，是按主键 join 出来的。** 巴利语与每种译文用**同一组主键** `(book_id, para_id, line_id)` 分库存储，运行期只查一次 | `sentence_page_content_repo.dart`（`_translationsFor`，join 只有一条 SQL） | `backend/internal/importer/align.go` 建立一次映射，之后就是 join |
| **翻译按语言分包下载**，运行时 `ATTACH` 成 schema `lang_<code>` | `language_installer.dart`、`database_helper.dart` | 见 `docs/desktop-plan.md`，尚未实现 |
| 页面由**句子**组成，一条一行 | `page_composer.dart` 的 `_sentenceHtml` | 散文按句切、偈颂按行切 |
| 标题就是句子表里的一行，`headings` 只给它层级 | `sentence_page_content_repo.dart` | `text_toc.segment_h` 指向标题段 |
| 目录**只写标题、不加编号**，层级靠缩进 | — | `frontend/src/components/CatalogTree.vue` |
| 迁移工具用**整册词流全局比对**搬运标记，落在不同文本上就记 `exact=0` 并报出来 | `tools/epitaka_migration/match_markers.py` | `backend/internal/importer/diff.go` |

**读它的时候注意**：它的 `book_map` 里 `S-i [560-39680]` 那样的范围是**词流偏移**、只给报告看，
**没有任何代码读回它**（见 `map_books.py`）。本项目曾经把它当成段落号去用，白费了功夫。

### 二、数据源

都在**导入期**读取，运行期只读 MySQL 或本机 SQLite：

| 来源 | 文件 | 提供什么 |
| --- | --- | --- |
| [dpd-db](https://github.com/digitalpalidictionary/dpd-db) | `dpd.db` | 词目、词形索引、变格模板、词根、复合词构造 |
| tipitaka-pali-reader | `tipitaka_pali.db` | 三藏原文（CST 第六次结集本）、DPD 词形语法、复合词切分、英文词典 |
| `dhammanana/epitaka_app` 的 release | `epitaka.db` | 巴利语原文，**一行一句**，键 `(book_id, para_id, line_id)` |
| 同上 | `epitaka_zh.db` / `epitaka_en.db` | 参考译文，与上面**同一组主键** |
| 巴漢詞典 / 漢譯パーリ語辭典 / 水野弘元 | `zh/*.txt`（Tabfile） | 中文释义 |
| 社区整理 | `zh_supplement.json` | 中文词典补编 |

源文件**不入库**（几个 GB，见 `.gitignore`）。放哪、每份多大、**从哪个地址下载**，
都写在 `../pali-data/README.md` 里（与本仓库平级）。服务器上在
`/www/server/go_project/pali_reading/sources/`。

两个地址最常用：参考译文来自
`https://github.com/dhammanana/epitaka_app/releases/download/latest/epitaka_zh.zip`，
三藏原文来自 `bksubhuti/tipitaka-pali-reader` 的 release 资产 `tipitaka_pali.db.tar.bz2`。
注意 `dpd.db` **不是直接下载的文件**，是 `dpd-db` 构建出来的产物。

### 三、库与设计系统

| 依赖 | 用途 |
| --- | --- |
| Vue 3 / Vue Router / Pinia / Vite 5 | 前端 |
| `lucide-vue-next` | **唯一的图标来源。不要用 emoji，也不要用字符当图标** |
| Go 1.22 / chi / GORM | 后端 |
| MySQL 5.7 / Redis | 存储与缓存（本机版改用 SQLite，见 `docs/desktop-plan.md`） |
| Electron 34 | 桌面外壳，见 `pali-reader/desktop/` |
| [OpenDesign](https://opendesign.dev) 的 **kami（紙／纸）** 设计系统 | 羊皮纸底、墨蓝单色、衬线主导。设计稿在 `docs/design/` |

字体栈（`frontend/src/assets/base.css` 的 `--ui` / `--pali`）抄自 `WorkSpace/EnglishReading`
的做法：**用系统字体、不引 webfont**，阅读面不等待网络请求。

---

## 目录结构

```
backend/
  main.go                      # 启动：迁移 → 连 Redis → 监听
  cmd/importer/                # 语料与词典导入器（构建期工具，不是运行时）
  internal/
    config/                    # 环境变量 → 配置
    store/                     # GORM 模型（schema 唯一来源）、连接、迁移
    corpus/                    # CST 页面 HTML → 可读段落
    tokenize/                  # 巴利语分词（导入期与查词共用同一份实现）
    dict/                      # 查词：词形 → 释义 + 语法 + 拆解 + 变格
    cache/                     # Redis 封装（Redis 挂了自动降级为无缓存）
    importer/                  # 各数据源的导入逻辑
    server/                    # HTTP 路由与处理器
frontend/
  src/
    api/                       # 唯一的后端调用入口
    store/                     # auth / reader 两个 store
    components/                # TopBar、目录树、段落卡片、查词面板、变格表
    views/                     # 首页 / 目录 / 阅读器 / 搜索 / 生词本 / 登录注册
    assets/base.css            # kami 设计令牌与全部组件样式
deploy/
  deploy.sh                    # 交叉编译 → 上传 → 重启
  run-import.sh                # 服务器上的导入脚本
  nginx-pali.bigubuntu.internal.conf   # 站点伪静态（= 面板伪静态输入框）
docs/
  design/opendesign-prototype.html     # OpenDesign 产出的设计稿
  architecture.md
```

---

## 数据模型要点

**锚点不用行 id。** 用户的选词、批注、翻译锚定在 `(book_id, segment, word_index)`：
第几卷、正文里的第几段、段内第几个词。段落序号由导入期冻结，重新导入语料不会让任何人的
笔记错位；`word_choices` / `notes` / `user_translations` 都不引用 `text_segments.id`。

**段落即渲染单位。** CST 的一页 HTML 在导入期被拆成「段」：散文是一个 `<p>`，偈颂是连续
若干行的合并，标题单独成段。段落数约 16 万，全藏约 6800 万字符。

**分词在导入期完成。** 每段的 `tokens` 列存 `[offset, length, flags]` 三元组数组；flags 的第
0 位表示「词典里有这个词」。前端拿偏移量切 `text` 渲染，payload 与原文几乎等大。

**查词一次请求。** `GET /api/dict/lookup?word=` 返回词目、词形分析、全部词典释义、
复合词拆解、变格表预览与语料频次；结果按词形缓存在 Redis。

---

## 本地开发

```bash
# 后端（需要 MySQL 与 Redis）
cd backend
DB_DSN='user:pass@tcp(127.0.0.1:3306)/pali_reading' \
REDIS_URL='redis://127.0.0.1:6379/0' \
DEV=true go run .

# 前端（Vite 代理 /api 到 :8090）
cd frontend
npm install
npm run dev     # http://localhost:5173
```

Go 的模块缓存被固定在仓库外的 `../.gopath`，用 `./go.sh` 代替 `go` 即可：

```bash
cd backend && ../go.sh build ./...
```

## 导入语料

导入器是构建期工具，运行期服务不写语料与词典表。

```bash
# 1. 准备源文件目录（见 docs/architecture.md 的「源文件」一节）
#    sources/{dpd.db,tipitaka_pali.db,epitaka.db,epitaka_zh.db},
#    sources/zh/*.txt, sources/zh_supplement.json
# 2. 导入
cd backend
DB_DSN='...' ../go.sh run ./cmd/importer -sources ../data/sources -steps all
```

`-steps` 支持 `schema,dict,text,catalog,entries,ref,endict,freq,report`，每步幂等，失败后可从该步续跑。

`endict` 是**可选的附加项**：英文参考译文的点词查词用的 ECDICT 词典。源文件
`sources/dict_seed.json`（8.9 MB，构建期输入，不进服务二进制）不存在时该步跳过，阅读器
照常工作，只是英文词点开显示「词典暂无收录」。单独刷新它只要
`./run-import.sh endict`，不必重导语料、也不必重发服务二进制。

## 部署

```bash
./deploy/deploy.sh          # 交叉编译 + 上传 + 重启 + 自检
```

脚本会编译 `linux/amd64` 的静态二进制与前端产物，上传到
`/www/server/go_project/pali_reading` 与 `/www/wwwroot/pali.bigubuntu.internal`，
再调用宝塔的面板接口重启 Go 项目。

---

## 数据来源与版权

巴利原文为 **Chaṭṭha Saṅgāyana（第六次结集）** 本。词典数据来自
[DPD](https://github.com/digitalpalidictionary/dpd-db)（CC BY-NC-SA）、
PTS Pali-English Dictionary、Concise Pali-English Dictionary、Pali Proper Names，
以及《巴漢詞典》《漢譯パーリ語辭典》《パーリ語辞典》。参考译文来自 ePitaka 发布的中译。

各数据源版权归原作者所有，本项目仅供个人学习使用。
