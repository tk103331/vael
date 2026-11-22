# 插件方案探索

Vael 要能扩展：加命令、改文本、读项目、连语言服务器。这是把两种运行时都真正跑起来之后的结论——**主插件语言用 JavaScript（moejs），同一套宿主 API 也开给 wasm（wazero）**，让需要编译语言或更强隔离的插件有去处。

原型在 [`experiments/plugins`](../experiments/plugins)：同一份「统计字数」插件分别用 Rust→wasm 与 JavaScript 实现，两种宿主都用同一份请求/响应 ABI 调用，测试断言两者答案一致、都能被中断，并测了加载与调用的开销。

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

## 宿主 API（两个运行时同名）

| 能力 | 说明 | 默认 |
| --- | --- | --- |
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
| M1 | moejs 宿主落地：`~/.config/vael/plugins/<name>/plugin.json` + `plugin.js`，命令注册进命令面板，`editor.text/selection/replace` 与 `notify`；超时与内存限额；设置里的插件列表（启用/禁用/打开目录） |
| M2 | 事件与贡献点：保存前/保存后、打开文件、命令面板、状态栏项；`storage`；插件错误在面板里显示堆栈 |
| M3 | wasm 运行时（wazero + 同一 Host）：清单 `runtime: "wasm"`，Extism 兼容的 PDK 让 Rust/Zig 插件少写胶水 |
| M4 | 权限模型与生态：能力确认弹窗、目录白名单、插件市场（从 Git 仓库/单体 `.zip` 安装、校验和、版本升级） |

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