# 插件方案原型

[`docs/PLUGINS.md`](../../docs/PLUGINS.md) 的设计得到验证的地方：同一份「字数统计」插件，用 JavaScript（moejs）和 Rust→wasm（wazero）各写一遍，两种宿主共用一套 ABI。

```
wasm.go            wazero 宿主：vael_alloc / vael_run + 线性内存
js.go              moejs 宿主：Hook("run") + 宿主全局（vael.notify）
plugin/            Rust 版插件（构建产物 wordcount.wasm 已提交）
js/wordcount.js    JavaScript 版插件
plugins_test.go    两边答案一致、都能被 context 停掉、加载与调用开销
```

ABI 一次调用一问一答：

```jsonc
// 宿主 → 插件
{ "command": "wordcount", "text": "你好 世界" }
// 插件 → 宿主
{ "notify": "2 词 · 1 行 · 5 字符" }
```

## 跑起来

```sh
go test ./...
go test -bench . -benchtime 2000x
```

本机（Apple Silicon，Go 1.27）实测：

| | 加载 | 一次调用 | 常驻堆 |
| --- | --:| --:| --:|
| wasm（Rust） | ~8.7 ms | ~8.1 µs | ~1 MiB |
| JS（moejs） | ~0.15 ms | ~4.2 µs | ~48 KiB |

## 重新构建 wasm 插件

测试跑的是已提交的 `plugin/wordcount.wasm`，改了 Rust 源码后：

```sh
cd plugin
rustup target add wasm32-unknown-unknown
cargo build --release --target wasm32-unknown-unknown
cp target/wasm32-unknown-unknown/release/vael_wordcount.wasm wordcount.wasm
```