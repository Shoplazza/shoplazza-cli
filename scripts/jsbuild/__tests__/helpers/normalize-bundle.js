'use strict';
// rollup 产物文件名/内部引用含 [hash]（8 位十六进制）。比较前统一替换成 HASH，
// 使对拍只关注语义内容、忽略每次构建都会变的内容哈希。
function normalizeBundle(src) {
  return src.replace(/\.[a-f0-9]{8}\.(js|css|[a-z0-9]+)\b/g, '.HASH.$1');
}
module.exports = { normalizeBundle };
