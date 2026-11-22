// A Vael plugin in JavaScript: the same work as the Rust one, and it can
// also say something while it works, through the global the editor
// installs.
//
// The counting follows the same rules as the Rust plugin, so that both
// answer the same thing for the same text: lines are counted as Rust's
// `str::lines` counts them, and a trailing newline adds no line.
export function run(request) {
  const { command, text } = JSON.parse(request);
  if (command === "loop") {
    for (;;) {}
  }
  const words = text.trim().split(/\s+/).filter(Boolean).length;
  const parts = text.split(/\r?\n/);
  const lines = parts.filter((line, i) => i < parts.length - 1 || line !== "").length;
  const chars = [...text].length;
  return JSON.stringify({ notify: `${words} 词 · ${lines} 行 · ${chars} 字符` });
}