# 插件方案探索

Vael 要能扩展：加命令、改文本、读项目、连语言服务器。这是把两种运行时都真正跑起来之后的结论——**主插件语言用 JavaScript（moejs），同一套宿主 API 也开给 wasm（wazero）**，让需要编译语言或更强隔离的插件有去处。

原型在 [`experiments/plugins`](../experiments/plugins)：同一份「统计字数」插件分别用 Rust→wasm 与 JavaScript 实现，两种宿主都用同一份请求/响应 ABI 调用，测试断言两者答案一致、都能被中断，并测了加载与调用的开销。

## 已经落地的部分（M1）

M1 已在编辑器里跑起来了，代码在 [`plugin.go`](../plugin.go) 与 [`internal/workspace/plugins.go`](../internal/workspace/plugins.go)：

- **插件目录**：`~/.config/vael/plugins/<名字>/plugin.json` + 插件代码；清单声明 `api`（当前 1）、`runtime`（当前 `js`）、`main`、`capabilities` 与 `contributes.commands`。版本、运行时、命令不全的清单一律不加载，错误显示在设置里。
- **运行时**：JavaScript 用 [moejs](https://github.com/MoeForge/moejs)（纯 Go、可中断、带内存限额）；每次调用限时 2 秒、限内存 64 MiB，超时的插件被中断，编辑器不会被卡住。
- **宿主 API**（按能力授权，没声明的能力调用即报错）：
  | API | 能力 | 做什么 |
  | --- | --- | --- |
  | `vael.notify(text)` | `notify` | 以提示条说一句 |
  | `vael.replace(text)` | `editor` | 把选区换成这段文字（进撤销栈） |
  | `vael.storage.get(key)` / `vael.storage.set(key, value)` | `storage` | 每个插件自己的键值存储（`plugin-storage.json`） |
- **调用约定**：编辑器给插件 `{command, text, content, path, selection}`，插件回 `{notify, replace, error, view}`；`text` 是选区，没有选区时是整个文件。
- **面板**：插件在答案里可以带一棵**声明式视图树**（`view`），编辑器用 [`pluginui.go`](../pluginui.go) 的那套部件画成右侧面板——插件只说结构与语义，字号、颜色、间距、图标都由宿主定；树里的按钮回到同一个 ABI（`command`），于是面板能自己刷新。
- **命令**：清单里的命令进命令面板，标题前缀「插件：」；插件自己画界面的声明式视图树仍是原型（见下文）。
- **启停**：设置里的「插件」一栏可以启用／停用、看版本、能力与错误，也能打开插件目录；状态记在 `plugins.json`。
- **两个运行时**：清单里 `runtime` 写 `js`（默认，moejs）或 `wasm`（wazero）。两者是**同一套 ABI**：编辑器写一份 JSON 请求，插件回一份 JSON 答案，答案里说编辑器该做什么（说话、替换文本、给面板）。
  - **JavaScript 插件**：`export function run(request)`，宿主把 `vael.notify`／`vael.replace`／`vael.storage` 装成全局函数，插件可以在计算时说话。
  - **wasm 插件**：模块导出 `vael_alloc(size)`（给宿主一块地方写请求）、`vael_run(ptr, size)`（跑命令，答案的高 32 位是位置、低 32 位是长度）与 `memory`；宿主把 JSON 写进它的内存，读回它的答案。wasm 插件没有宿主函数，能力通过答案里的动作体现。沙箱限内存 64 MiB，超时（默认 2 秒）即关闭沙箱，死循环的插件也停得下。Rust 的例子见 [`experiments/plugins/plugin`](../experiments/plugins/plugin)。
- **主题与图标主题也是插件**：像 Zed 的扩展一样，主题只是数据，不需要运行代码。清单里写 `contributes.themes`（`id`／`name`／`family`／`dark`／`colors`，颜色用 `#rrggbb`，键是 `wash`、`card`、`text`、`muted`、`edge`、`accent`、`selected`、`warn`、`success`、`danger`）和 `contributes.iconThemes`（`id`／`name`／`marks`，mark 按扩展名给 `text` 或 `icon` 名，加 `tone`：`muted`／`accent`／`success`／`warn`／`danger`）。同一个 `family` 的深浅主题会被 `⌘/Ctrl+Shift+L` 一起切换；主题随会话保留。示例见 [`examples/plugins/midnight`](../examples/plugins/midnight)。
- **示例**：[`examples/plugins/wordcount`](../examples/plugins/wordcount)，复制到插件目录即可用。
- **安装与卸载**（M4 的一部分）：插件包是 zip 或 tar.gz，里面有 `plugin.json` 与插件的文件；可以从文件或网址安装（网址可带 SHA-256 校验），安装时先解到临时目录、读清单、核对接口版本与运行时，再放进插件目录；写到包外的路径一律拒绝；同名插件会被替换，插件可以在设置里卸载。
- **插件商店**（M4 的另一部分）：设置里填一个索引地址（JSON，`{"plugins":[{"id","name","version","description","url","sha256"}]}`，也接受直接是数组），编辑器读入并缓存 6 小时（`plugin-store.json`），列出插件；装过的显示版本，索引里更新就给出「更新到 x.y.z」。包按索引里的 SHA-256 校验。**还没有**的是官方公共索引的托管、自动更新与依赖解析。

与原计划的不同：M1 先只做 JavaScript 运行时（wasm 留到 M3），插件的自注册命令（`activate(api)`）改为清单里的 `contributes.commands`，权限确认弹窗与插件市场留到 M4。

## 目标与约束

- UI 是 Go（MyGo 原生），插件**不自己画界面**：插件只能贡献命令、文本变换、面板内容的数据、状态栏文字等，由 Vael 负责渲染。
- 纯 Go、无 cgo、可交叉编译（与核心、MyGo 一致）。
- 插件可能来自网络，必须**能被中断、能限内存、能限权限**。
- 插件生态要低门槛：写插件不该先装一套工具链。

## 备选一：moejs（纯 Go 的 JavaScript 运行时）

[moejs](https://github.com/MoeForge/moejs) 用 Go 实现 JS 引擎，专为「在 Go 程序里跑很多小插件沙箱」而做。

- 完整 ES（含模块、async/await、Proxy、BigInt），通过 test262；还能直接跑 TypeScript（剥类型）。
- 宿主函数就是 Go 函数（`moejs.NativeFunc` + `SetGlobal`），插件通过 `import` 也回到 Go 提供的函数——**宿主 API 的边界天然可控**。
- 可中断：任意 goroutine 调 `Interrupt`，死循环也能停下；`MemoryLimit` 按请求限内存；`Stats` 给计数器。
- 生态设施现成：`eventloop`（setTimeout/事件循环）、`webapi`（fetch/Streams/AbortController，带 allow 钩子）、`node`（`node:fs` 被 `os.Root` 关在一个目录里，默认只读）。
- 体积：无运行时依赖（纯 Go），每个 runtime 约几十 KiB；调用约几微秒。

## 备选二：wasm（wazero）

[wazero](https://wazero.io) 是纯 Go 的 WebAssembly 运行时（无 cgo，可交叉编译），Extism 之类的框架就建在它上面。

- 强隔离：线性内存 + 能力式导入，插件只能碰到宿主显式给的东西。
- 语言自由：Rust、C/C++、Zig、AssemblyScript、（TinyGo 编译的）Go 都行，产物是单个 `.wasm`。
- `CloseOnContextDone` 让一次调用可被 context 取消（原型里用它停掉死循环）。
- 源码级 ABI：跨语言内存（`vael_alloc`/`vael_run` + 线性内存读写），或者用 Extism PDK 省掉这层。
- 代价：作者要装工具链；Go 自己的 `GOOS=wasip1` 是**命令模型**（`_start` 跑完就退出），做常驻插件得用 TinyGo 或 Rust；加载要编译（毫秒级），小插件的调用开销与 JS 相当甚至更高。

## 实测（本机，Apple Silicon，Go 1.27）

同一份「字数统计」插件，同样的 JSON 请求，各 2000 次：

| | 加载 | 一次调用 | 常驻堆 | 插件产物 |
| --- | --:| --:| --:| --:|
| wasm（Rust，wazero） | ~8.7 ms | ~8.1 µs | ~1 MiB（含编译产物） | 36 KiB `.wasm` |
| JS（moejs） | ~0.15 ms | ~4.2 µs | ~48 KiB | 1 KiB `.js`（源码即产物） |

结论很清楚：**对小插件（编辑器插件绝大多数都是），JS 的加载快约 60 倍、内存小一个数量级、单次调用还更快**——调用开销都由 ABI 的 JSON 编解码主导，wasm 的「接近原生」在这类工作量上没有优势。wasm 的优势在重计算或需要长期驻留的状态机上（语言服务器代理、语法分析器），以及需要真正内存隔离的不可信插件。

两者都能满足「可中断」（原型里两边都在 100 ms 内被停掉），这是编辑器插件的硬要求。

## 推荐：一套 ABI，两个运行时

```
                    ┌────────────── Vael（Go / MyGo）
   命令面板 ───────▶│ plugin.Host
                    │   ├─ 权限检查（能力表）
                    │   ├─ 超时 / 中断 / 内存限额
                    │   └─ 宿主 API（读写编辑区、通知、状态栏、存储…）
                    └───┬───────────────┬─────────────────
                        │ moejs         │ wazero
                        │ plugin.js     │ plugin.wasm
```

- **默认运行时是 moejs**：插件就是 `plugin.js`（或 `.ts`），放进插件目录即用，不需要构建。
- **wasm 运行时可选**：同一份宿主 API 以导入的形式给 wasm（Extism PDK 风格的 `vael_call` 或自定义 ABI），给重计算、强隔离、或作者偏好的编译语言。
- 两者都调用同一个 `Host`，权限、超时、审计、错误呈现只有一份实现。

## ABI：一次调用，一问一答

请求是 JSON，响应也是 JSON；不做回调式长连接，宿主负责把响应变成动作。

```jsonc
// 宿主 → 插件
{ "command": "wordcount", "text": "你好 世界", "selection": [0, 10], "path": "src/main.go" }

// 插件 → 宿主
{ "notify": "2 词 · 1 行" }                  // 状态栏/提示条说一句
{ "replace": "…新文本…" }                     // 替换传入的文本（一次撤销）
{ "commands": [{ "id": "hello", "title": "Hello" }] }   // 注册命令（插件加载时）
{ "error": "…" }
```

- 加载时插件可导出 `activate(api)`：注册命令、事件订阅、状态栏项；`deactivate()` 清理。
- 编辑区操作一律经过宿主回传的文本/选区，而不是给插件文件句柄——既能撤销，也便于审计。
- 响应式设计天然适配「命令 -> 结果」这类插件（格式化、转换、插入片段、生成提交信息）；需要流式或后台工作的插件走宿主提供的事件与任务 API（见下）。

## 自绘 UI：插件描述「是什么」，编辑器决定「长什么样」

插件要画界面时，**不给它像素**，而是让它交一棵**声明式视图树**：节点只说结构和语义（列、行、标题、徽标、列表、按钮、提示……），由编辑器用自己的主题、字号、间距渲染。原型见 [`pluginui.go`](../pluginui.go)（命令面板里「视图：插件面板（原型）」可打开），界面长这样：

```
┌ 插件面板（原型）──────────────┐   ← 宿主外壳：与其它面板同高、同标题栏
│ 提交摘要              [3 项]  │   ← title + badge(accent)
│ 工作区有 1 个文件改动…         │   ← meta，宿主定字号与颜色
│ ────────────────────────────  │
│ 📄 main.go        M  +12 −3   │   ← list 行：图标、标题、编码、详情
│ 📄 store.go       A  +40      │
│ 提交信息                       │   ← meta
│ feat(store): 补上计数器        │   ← code 块
│ ⚠ 提交前会先跑 go test ./...   │   ← notice(warn)，宿主的警告样式
│ [提交]  查看差异               │   ← primary / quiet 按钮
└──────────────────────────────┘
```

```jsonc
{"type":"column","gap":5,"children":[
  {"type":"row","gap":3,"align":"center","children":[
    {"type":"text","text":"提交摘要","role":"title"},
    {"type":"spacer"},
    {"type":"badge","text":"3 项","tone":"accent"}
  ]},
  {"type":"list","items":[
    {"title":"main.go","code":"M","detail":"+12 −3","icon":"file","selected":true}
  ]},
  {"type":"button","text":"提交","command":"commit","primary":true}
]}
```

### 三层，按表达力从紧到松

| 层 | 插件给什么 | 谁来画 | 适用 |
| --- | --- | --- | --- |
| 1. 声明式视图树（默认） | JSON 节点树 | 编辑器 | 面板、表单、列表、状态、结果 |
| 2. 贡献数据 | 命令、状态栏项、列表行的数据 | 宿主的既有视图 | 状态栏、提交列表、搜索结果 |
| 3. 受限画布（逃生口） | 像素或 web 内容，画在宿主给的卡片里 | 插件，但受限于宿主给的画框 | 图表、diff 预览、可视化 |

### 靠什么保证统一

- **组件目录固定**：`column`、`row`、`text`、`code`、`badge`、`notice`、`button`、`buttons`、`input`、`list`、`markdown`、`spacer`、`divider`。不认识的组件**就地显示「不支持」**，不是一片空白（有测试）。
- **度量由宿主定，插件只选档位**：`gap` 是间距档位（0/2/4/6/8/10/12），不是像素；字号按角色（`title` 12.5/650、`body` 11.5、`meta` 10、`mono` 10.5 等宽）；行高 26/28、圆角 4/5/6、图标只能用名录里的名字（`file`、`git`、`check`……）。
- **颜色只能按语义**：`tone` 取 `default`／`muted`／`accent`／`success`／`warn`／`danger`，由当前主题解析——同一棵树在明暗两种主题下自动正确，插件拿不到 `#rrggbb`。
- **外壳归宿主**：插件面板永远在宿主的标题栏下（与其它面板同为 36 高），插件画不了窗口边框、菜单、标题栏、缩放。
- **交互归宿主**：按钮只发**命令**（`command`，注册进命令面板），焦点环、悬停、选中、滚动条、上下文菜单、提示都由宿主提供；快捷键必须经宿主注册（冲突检测 + 用户可改键）。
- **无障碍与本地化**：视图树的每个节点带可读标签，宿主转成无障碍节点；文案由插件给，但字号、对比度由角色决定。

### 校验、工具与降级

- **解析即校验**：树的深度 ≤ 12、元素 ≤ 400，超了就拒绝并说明（有测试）。
- **开发模式**：一个「组件画廊」面板把每个组件在明暗两种主题下各渲染一遍，配 golden 截图测试——样式改动一眼可见；`vael plugin lint` 检查 gap 档位、tone 名、图标名、重复命令。
- **版本与降级**：视图树带 `api` 版本与组件名；宿主对未知组件显示占位提示，旧插件照常工作；新组件按需增加，不破坏已有插件。

### 逃生口：受限画布

只有声明了 `canvas` 能力、且用户批准的插件可以用：宿主给一张与面板同底色、同圆角的卡片，把已解析的主题令牌（颜色、字号、行高、圆角）作为参数传给插件，并带一个「插件」角标；画布之外的一切仍然由宿主画。样式漂移靠三件事约束——令牌先行、角标提示、安装时的能力审查。

## 宿主 API（两个运行时同名）

| 能力 | 说明 | 默认 |
| --- | --- | --- |
| `view.render(tree)` | 交一棵声明式视图树给宿主渲染 | 允许 |
| `editor.text(event)` / `editor.selection()` | 读当前文件文本与选区（只读快照） | 允许 |
| `editor.replace(range, text)` | 替换，进撤销栈 | 允许 |
| `editor.open(path)` / `workspace.files()` | 打开与列项目文件 | 允许 |
| `notify.message(text)` | 状态栏/提示条 | 允许 |
| `status.item(id, text)` | 状态栏徽标 | 允许 |
| `commands.register(...)` | 注册命令进命令面板 | 允许 |
| `storage.get/set(key)` | 每个插件独立的键值存储 | 允许（限额） |
| `http.fetch(url, opts)` | 网络 | **需声明 + 用户批准** |
| `fs.read/write(path)` | 项目内文件 | **需声明 + 目录白名单** |
| `process.run(cmd)` | 执行命令 | **需声明 + 用户批准**（同任务面板的权限） |

清单 `plugin.json`：

```jsonc
{
  "name": "wordcount",
  "version": "0.1.0",
  "api": 1,
  "runtime": "js",              // js | wasm
  "main": "plugin.js",          // 或 plugin.wasm
  "capabilities": ["editor", "notify"],   // 声明的能力，加载时核对
  "contributes": { "commands": [{ "id": "wordcount", "title": "统计字数" }] }
}
```

- 与核心协议一样，**加载时核对 `api` 版本与能力表**，缺一即拒绝并说明原因（沿用现有核心握手的思路）。
- 未声明的能力在调用时直接报错，而不是静默失败。

## 权限与安全

- **能力声明**：清单里写，界面上展示；涉及 `http`/`fs`/`process` 的首次使用弹窗确认，结果记入 `plugins/permissions.json`。
- **文件**：moejs 的 `node:fs` 天然被 `os.Root` 关进插件目录；若要读项目文件，走宿主 API（路径规范化后检查是否在项目内）。
- **网络**：`webapi` 的 allow 钩子按域名白名单放行，未批准直接失败。
- **资源**：每次调用带超时（默认 2 s，命令可声明更长）、`MemoryLimit`（默认 64 MiB/请求）、单插件调用频率上限。
- **隔离**：wasm 插件天然进程内沙箱；JS 插件与 Vael 同进程，靠引擎的中断与内存限额兜底——这也是为什么 JS 必须是「一问一答 + 可中断」，而不是让插件长期跑循环。

## 路线图

| 里程碑 | 内容 |
| --- | --- |
| M1 | moejs 宿主落地：声明式视图树（与 `pluginui.go` 原型同一套节点）`~/.config/vael/plugins/<name>/plugin.json` + `plugin.js`，命令注册进命令面板，`editor.text/selection/replace` 与 `notify`；超时与内存限额；设置里的插件列表（启用/禁用/打开目录）——**已完成**，见上 |
| M2 | 事件与贡献点：保存前/保存后、打开文件、命令面板、状态栏项；`storage`；插件错误在面板里显示堆栈 |
| M3 | wasm 运行时（wazero + 同一 Host）：清单 `runtime: "wasm"`，Extism 兼容的 PDK 让 Rust/Zig 插件少写胶水——**运行时已完成**（`vael_alloc`／`vael_run`／`memory` 这套 ABI），PDK 与宿主函数还没做 |
| M4 | 权限模型与生态：能力确认弹窗、目录白名单、插件市场（从 Git 仓库/单体 `.zip` 安装、校验和、版本升级）——**索引、校验和、安装、版本升级已完成**；能力确认弹窗与公共索引托管还没做 |

## 风险与取舍

- **moejs 还年轻**（v0.1.x）：接口可能变，跟着升级；好处是纯 Go、可中断、带 Node/web 兼容层，正好覆盖编辑器插件的需求。
- **JS 与 Go 同进程**：插件崩溃/死循环需要靠中断与限额兜底；不可信插件建议走 wasm（M3）。
- **双运行时 = 双份 ABI 维护**：所以先定 ABI 与 Host，两边只是「怎么把 JSON 送进去、怎么取回来」的差别（原型里分别是线性内存读写与 `NativeFunc`）。
- **不为插件开放自绘 UI**：保持 Vael 的界面语言统一，插件只能提供数据与命令；需要面板时用声明式的列表/表单，由 Vael 渲染。

## 运行原型

```sh
cd experiments/plugins
go test ./...                    # 两种宿主：答案一致、可中断、加载/调用开销
go test -bench . -benchtime 2000x
```

wasm 插件源码在 `plugin/`，用 Rust 构建（产物已提交，测试不需要工具链）：

```sh
cd plugin
cargo build --release --target wasm32-unknown-unknown
cp target/wasm32-unknown-unknown/release/vael_wordcount.wasm wordcount.wasm
```