// A Vael plugin in JavaScript: it is called with the file that is open,
// and answers with what the editor should do.
//
//   vael.notify(text)   说话（需要 notify 能力）
//   vael.replace(text)  把选区换成这段文字（需要 editor 能力）
//   vael.storage.get(key) / vael.storage.set(key, value)  记住什么（需要 storage 能力）
//
// 把它整个目录复制到 ~/.config/vael/plugins/ 里，「统计字数」就会出现在命令面板。

export function run(request) {
  const { command, text } = JSON.parse(request);
  if (command !== "wordcount") {
    return JSON.stringify({ error: "不认识的命令：" + command });
  }
  const words = text.trim().split(/\s+/).filter(Boolean).length;
  const parts = text.split(/\r?\n/);
  const lines = parts.filter((line, i) => i < parts.length - 1 || line !== "").length;
  const chars = [...text].length;
  return JSON.stringify({ notify: `${words} 词 · ${lines} 行 · ${chars} 字符` });
}
