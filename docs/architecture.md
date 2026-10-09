# 架构说明

## 一句话

语料与词典在**导入期**被压成三张可索引的表；运行期只做「按卷取一段正文」和「按词形取一份
词典答案」两件事，两件都走 Redis。用户自己的批注与翻译另走一条索引路径，永不缓存。

---

## 请求路径

### 1. 打开一卷经

```
GET /api/books/mula_di_01            → 卷信息 + 标题目录           （Redis 24h）
GET /api/books/mula_di_01/segments?from=1&count=60
                                     → 60 段正文，含分词三元组    （Redis 24h）
GET /api/books/mula_di_01/marks?from=1&to=60
                                     → 我的选词 / 批注 / 翻译
                                     → 该区间的参考译文
```

前两个请求与用户无关，全体共用一个 Redis 键；第三个带 token，按索引直查 MySQL。
首屏因此只有三次往返，且第二次之后的每次翻页都命中缓存。

滚动到末尾时前端再取下一批 60 段（`IntersectionObserver`，rootMargin 600px），
没有虚拟列表——段落高度差异太大，虚拟化带来的抖动比它省下的渲染更贵。

### 2. 点击一个词

```
GET /api/dict/lookup?word=buddhassa
```

一次请求返回面板需要的全部内容：

```jsonc
{
  "query": "buddhassa", "key": "buddhassa", "found": true,
  "headwords": [ { "lemma": "buddha", "pos": "masc", "phonetic": "/bʊd̪ʰə/",
                   "meaning1": "Buddha; Awakened One",
                   "construction": "√budh + ta",
                   "declension": { "columns": ["sg","pl"], "rows": [...],
                                   "hit": {"row":5,"column":0,"form":"buddhassa"} } } ],
  "analyses": [ {"pos":"noun","grammar":"masc dat sg","lemma":"buddha"},
                {"pos":"pp","grammar":"masc gen sg","lemma":"buddha"} ],
  "meanings": [ {"source":"dpd","name":"DPD","lang":"en","text":"..."},
                {"source":"mingfa","name":"巴漢詞典 · 明法尊者增訂","lang":"zh","text":"..."} ],
  "splits":   [ {"parts":["dhamma","cakka"],"resolved":true} ],
  "roots":    [ {"root":"√budh","rootMeaning":"know, wake up"} ],
  "freq": 14231, "rank": 412
}
```

结果按 `key` 缓存在 Redis 12 小时。查一个从未见过的词形约 8–15 ms（MySQL 数次单行查），
命中缓存后 < 1 ms。

---

## 语料是如何被拆成段的

`tipitaka_pali.db` 的 `pages` 表是**印刷页**，一页一段 HTML。阅读器不能用页做单位，于是
`internal/corpus/ParseBook` 把它重排成段：

| CST class | 变成 |
| --- | --- |
| `bodytext` `noindentbodytext` `unindented` `indent` | `prose` 段 |
| `gatha1/2/3/gathalast`（连续若干行） | 合并为一个 `verse` 段，行内以换行保留 |
| `nikaya` `book` `chapter` `title` `subhead` `subsubhead` | `heading` 段，同时生成一条目录项 |
| `centered` | `center` 段（礼敬语、结颂） |
| `hangnum` `paranum` | 丢弃（只是段号本身） |

同时收集三种信息：

- **段号** `<a name="para24">` → `para_no`。它会被后随的段落继承，因此在阅读器里可以显示
  「§24」，与学术引用一致。变体形式 `para24_vin1` 是同一段在别的版本里的锚点，忽略。
- **版本页码** `<a name="M1.0010">`（M 缅、T 泰、V VRI、P PTS、S 锡兰）→ `markers`。
- **异读** `<span class="note">[suriyaggāho (sī. syā. kaṃ. pī.)]</span>` → HTML 里保留为
  `<span class="v">`，纯文本里剔除。默认不显示，需要校勘时展开。

全藏结果：**179 卷、约 16 万段、6800 万字符**。

---

## 分词与「哪些词可以点」

`internal/tokenize` 是唯一实现，导入期与运行期共用，因此偏移量永远一致。

- 词字符 = Unicode 字母或附加符号，加上 `'` `’` `ʼ`（巴利语的省略号，如
  `buddhassā'ti`）与连字符（`evarūpāya-tiracchānavijjāya`）。
- 词首词尾的撇号与连字符会被剥掉再做归一化，但 token 的 offset/length 仍指向**可见形式**。
- 归一化 = 转小写。DPD 的查词键就是小写词形。

每段的 `tokens` 存成 `[[offset, length, flags], ...]`：
`flags & 1` 表示词典里有这个词（前端据此画虚线下划线），`flags & 2` 表示原文首字母大写。

导入时对每一卷做一次批量 `SELECT lookup_key ... WHERE lookup_key IN (...)`（每 5000 个一批），
所以标记「哪些词可查」不是逐词往返。

---

## 查词为什么一次就能给全

`internal/dict.Service.Lookup` 串起四步，每步都只查一次：

1. `dict_lookup`（主键查）→ 词目 id 列表、词形分析、拆解候选、拼写建议。
2. `dict_headwords`（主键 `IN`）→ 词目本体，按 id 列表顺序重排（DPD 的顺序就是相关度）。
3. `dict_entries`（联合唯一键查）→ 其他词典的释义，用 `dict_sources.sort` 排成面板顺序。
4. `word_freq`（主键查）→ 语料频次。

变格表由 `dict_templates.data` 现场渲染。DPD 把变格网格存成矩阵：第 0 行在奇数位放列标题，
之后每行在 `1+2c` 放词形、`2+2c` 放该列的完整标签。按位置读，形容词的六列网格与名词的两列
网格就能用同一段代码渲染出来。命中格由词形反查得到。

**「词形分析」来自哪里。** DPD 的 `lookup.grammar` 在部分构建里是空的，于是导入期改用
`tipitaka_pali.db` 的 `dpd_grammar` 表——那是 DPD 自己渲染出来的每词形语法表，用
`ParseGrammar` 反向解析回 `{pos, grammar, lemma}` 结构。约 16.3 万个词形由此拿到分析。

**「词典里没有这个词」怎么办。** 按顺序兜底：DPD 的 `deconstructor` → `dpd_word_split`
的替代切分 → 词目自身的 `compound_construction`。三者都空就如实说词典未收录，不猜。

---

## 用户数据为什么不会错位

锚点是 `(book_id, segment, word_index)`。

- `segment` 是导入期在**卷内**的顺序号，重导语料时会重算，但同一份上游数据重算结果相同。
- `word_index` 是段内第几个 token，由导入期冻结的分词决定。

两者都不引用 `text_segments.id`。因此：

- 重导语料不会让任何人的笔记指向错误的段落——最坏情况是整卷的段号全变，那时笔记会落在
  另一段上，这一点由「重新导入只发生在语料升级时」来控制。
- 参考译文表 `ref_translations` 同样按 `(book_id, segment)` 存，与用户数据同构。

---

## 参考译文是怎么对齐的

ePitaka 发布了全藏中译，但它用的是自己的分段计数，与本项目按 CST § 号切出来的段**不是同一
把尺子**，无法按编号 join。两边共有的是巴利原文本身，所以对齐做在文本上：

1. 由 tipitaka-pali-reader 的 `book_map_report.txt` 得到卷级对应（176/179 卷有对应）。
2. 把两边的正文都降成词流（只保留字母数字、转小写）。这里刻意不重用 `tokenize`：两个版本
   放省略号撇号的位置不同（`atthasamhita'nti` vs `atthasamhitan ' ti`），按撇号切会让两条
   词流长度不一致。
3. 每个 ePitaka 段落用开头 6 个词做锚，在本项目的词流索引里查找**离游标最近**的位置。
   游标单调前进，因此反复出现的套语不会把对齐带偏。
4. 锚不上的段落归给游标所在段（它本来就是同一 § 的续文）；偏离游标超过 400 词的判定为
   锚定失败，如实计为未覆盖。
5. 覆盖率低于 90% 的卷**整卷跳过**——把译文放在错误的段落下面，比没有译文更糟。

---

## 缓存策略

| 键 | 内容 | TTL | 失效 |
| --- | --- | --- | --- |
| `catalog:v1` | 目录树 | 24h | 重导语料后手工清 |
| `b:<bookId>` | 卷信息 + 标题目录 | 24h | 同上 |
| `seg:<bookId>:<from>:<count>` | 一段正文（含分词） | 24h | 同上 |
| `d:lk:<key>` | 一次查词的完整答案 | 12h | 同上 |
| `d:dc:<pattern>:<form>` | 变格表 | 12h | 同上 |
| `s:<token>` | 会话（用户 JSON） | 随会话 | 登出时删除 |

Redis 不可用时全部降级为直查 MySQL：`cache.Cache` 的零值所有方法都是空操作，服务照常启动，
只是慢一些。这一点在 `main.go` 里是显式的——连不上 Redis 只打一行日志。

---

## 两个曾经踩进去的坑

这两处都不会报错，只会安静地给出错误的答案，所以写下来。

### 1. MySQL 的默认排序规则会把 ḍ 折成 d

`utf8mb4_general_ci` 把许多带附加符号的拉丁字母与它们的基本字母视为相等，于是
`buḍḍhassa` 与 `buddhassa` 在**主键上冲突**——导入成功、行数正常，但佛陀的词条被形容词
「老的」覆盖了。巴利语里 `ḍ ṭ ṇ ḷ ṃ` 是独立的字母，不是装饰。

凡是承载词语的列都必须在类型里写明 `CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`。
注意 **GORM 1.25 与其 MySQL 驱动并不实现 `collate:` 标签**：写了不报错，然后被丢掉，
列仍然落在库的默认排序规则上。`internal/store/collation_test.go` 守住这条。

### 2. 词元偏移量必须是 UTF-16 码元，不是字节

分词器在 Go 里按字节工作，但阅读器用 `String.prototype.slice` 切**JavaScript 字符串**，
那里的下标是 UTF-16 码元。每个拉丁字母以外的巴利语附加符号（ā ī ū ṃ ṭ ḍ ṇ ḷ ṅ ñ）占
2–3 个字节却只占 1 个码元，于是偏移量每遇到一个带符号的字母就错开一点，下划线落在错误
的单词上。`encodeTokens` 因此把字节偏移换算成 UTF-16 偏移再落库。

---

## 安全

- 密码 bcrypt；会话是 32 字节随机 token，`users`/`sessions` 分离。
- 登录失败对「账号不存在」与「密码错误」返回同一句话，避免枚举邮箱。
- 注册与登录按 IP 限流（Redis 计数窗口）。
- 搜索的 SQL 里 `LIKE` 通配符被转义；返回的片段走 HTML 转义，只有 `<mark>` 是服务端加的。
- 批注、选词、翻译的更新都带 `user_id` 条件，改不到别人的行。
- 前端不渲染来自语料的 HTML，只渲染 `<span class="v">` 与 `<span class="b">` 两种白名单标记
  （在导入期就把其他标签剥掉了）。
