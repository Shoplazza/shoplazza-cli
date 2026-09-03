#!/usr/bin/env bash
# E2E 验收脚本：themes block +edit / +get（AI 生成 block 文件的写入、落位与读取）
#
# 用法（脚本在 tests/e2e/，随仓库维护）：
#   bash theme_block.sh          # 安全模式：前置检查 + DryRun 轨道（零真实调用）
#   bash theme_block.sh --live   # 完整验收：写操作仅落在 SHOPLAZZA_TEST_THEME_ID 的编辑会话草稿
#
# 环境变量：
#   SHOPLAZZA_TEST_THEME_ID   一次性测试主题（never production），--live 必填
#   SHOPLAZZA_BIN             可选，缺省用 PATH 上的 shoplazza
#
# 硬性安全边界：
#   - 所有写场景显式 --theme "$TEST_THEME"，且该主题必须非上线
#   - 绝不 promote / publish：全部改动停留在编辑会话草稿
#   - 退出时对本次创建的每个 block 调 delete-gen 清理（连带移除实例）
set -u

BIN="${SHOPLAZZA_BIN:-shoplazza}"
LIVE=0
[[ "${1:-}" == "--live" ]] && LIVE=1
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FX="$HERE/fixtures"

PASS=0; FAIL=0; SKIP=0
declare -a FAILED_NAMES=()
declare -a CREATED_TYPES=()

log()  { printf '%s\n' "$*" >&2; }
result() {
  case "$1" in
    PASS) PASS=$((PASS+1)); log "  ✅ PASS  $2";;
    SKIP) SKIP=$((SKIP+1)); log "  ⏭️  SKIP  $2 —— ${3:-}";;
    *)    FAIL=$((FAIL+1)); FAILED_NAMES+=("$2"); log "  ❌ FAIL  $2 —— ${3:-}";;
  esac
}
OUT=""; ERR=""; CODE=0
run_cli() {
  local out_f err_f
  out_f=$(mktemp); err_f=$(mktemp)
  "$BIN" "$@" >"$out_f" 2>"$err_f"; CODE=$?
  OUT=$(cat "$out_f"); ERR=$(cat "$err_f")
  rm -f "$out_f" "$err_f"
}
jsonq() {
  printf '%s' "$1" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    v = eval(sys.argv[1])
    print(json.dumps(v, ensure_ascii=False) if isinstance(v, (dict, list)) else v)
except Exception:
    pass' "$2" 2>/dev/null
}
expect() { if [[ "$3" == "1" ]]; then return 0; fi; result FAIL "$1" "断言失败: $2"; return 1; }
is_true() { [[ "$1" == "True" || "$1" == "true" ]] && echo 1 || echo 0; }
# page_block <template> <section_id> <target> <python 表达式，b 为该 block 行>：经 themes +page 回读实例（不依赖 gen-blocks 读接口）
page_block() {
  run_cli themes +page --template "$1" --theme "$TEST_THEME" --session "$OSEID" --section "$2"
  jsonq "$OUT" "next(($4 for b in (d['data'].get('sections') or [d['data'].get('section')])[0]['blocks'] if b['target']=='$3'))"
}
# get_fail_reason：+get 非 0 退出时给出可读原因（服务端 500 单独点名，便于区分 CLI 缺陷与后端缺陷）
get_fail_reason() {
  local st rid; st=$(jsonq "$ERR" "d['error']['detail']['status_code']"); rid=$(jsonq "$ERR" "d['error']['detail'].get('request_id','')")
  if [[ "$st" == "500" ]]; then echo "后端 gen-blocks GET 500（request_id=${rid}）"; else echo "exit=$CODE $ERR"; fi
}

# ---------- 前置检查 ----------
log "== 前置检查 =="
command -v "$BIN" >/dev/null 2>&1 || { log "FATAL: 找不到 CLI 二进制 '$BIN'"; exit 10; }
command -v python3 >/dev/null 2>&1 || { log "FATAL: 需要 python3"; exit 10; }
BLOCK_HELP=$("$BIN" themes block --help 2>/dev/null)
grep -q '^\s*+edit\b' <<<"$BLOCK_HELP" || { log "FATAL: 'themes block +edit' 未实现"; exit 11; }
grep -q '^\s*+get\b'  <<<"$BLOCK_HELP" || { log "FATAL: 'themes block +get' 未实现"; exit 11; }
[[ -f "$FX/gen_block_min.liquid" && -f "$FX/gen_block_v2.liquid" && -f "$FX/gen_block_bad.liquid" ]] || { log "FATAL: 缺少 fixtures"; exit 10; }
log "  CLI 就绪：$("$BIN" --version 2>/dev/null | head -1)"

TEST_THEME="${SHOPLAZZA_TEST_THEME_ID:-}"
if [[ "$LIVE" == "1" ]]; then
  run_cli themes list --params '{"page_size":"1"}'
  [[ $CODE -eq 0 ]] || { log "FATAL: --live 需要有效的 profile 登录态"; exit 12; }
  [[ -n "$TEST_THEME" ]] || { log "FATAL: --live 需要 SHOPLAZZA_TEST_THEME_ID"; exit 12; }
  run_cli themes get --params "{\"theme_id\":\"$TEST_THEME\"}"
  [[ $CODE -eq 0 ]] || { log "FATAL: 测试主题 $TEST_THEME 不可访问"; exit 12; }
  PUBLISHED=$(jsonq "$OUT" "d['data'].get('theme',d['data']).get('published')")
  [[ "$PUBLISHED" == "true" || "$PUBLISHED" == "1" || "$PUBLISHED" == "True" ]] && { log "FATAL: $TEST_THEME 是上线主题，拒绝作为写目标"; exit 13; }
  log "  测试主题 $TEST_THEME 就绪（非上线）"
fi

# ---------- b00：DryRun（安全模式即可跑，零真实调用）----------
log "== DryRun 轨道 =="
b00() {
  local name="b00-dryrun"
  run_cli themes block +edit --session ose_dry --content "$FX/gen_block_min.liquid" --template index --target 111.blocks --dry-run
  expect "$name" "+edit --dry-run exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  expect "$name" "dry_run:true" "$(is_true "$(jsonq "$OUT" "d.get('dry_run')")")" || return
  expect "$name" "含 <doc_id> 占位" "$([[ "$OUT" == *"<doc_id>"* ]] && echo 1 || echo 0)" || return
  expect "$name" "不含 /shop 请求" "$([[ "$OUT" != *"/2026-01/shop"* ]] && echo 1 || echo 0)" || return
  local last; last=$(jsonq "$OUT" "d['requests'][-1]['data']['operations'][0]['op']")
  expect "$name" "末条请求是 append_array_item 批次" "$([[ "$last" == "append_array_item" ]] && echo 1 || echo 0)" || return
  run_cli themes block +edit --session ose_dry --id gen_x --content "$FX/gen_block_min.liquid" --dry-run
  expect "$name" "改卡 dry-run 只有 PATCH gen-blocks" "$([[ $CODE -eq 0 && "$(jsonq "$OUT" "len(d['requests'])")" == "1" && "$(jsonq "$OUT" "d['requests'][0]['method']")" == "PATCH" ]] && echo 1 || echo 0)" || return
  run_cli themes block +get --session ose_dry --id gen_x --with-content --dry-run
  expect "$name" "+get dry-run 带 type/with_content" "$([[ $CODE -eq 0 && "$(jsonq "$OUT" "d['requests'][0]['params']['type']")" == "blocks/gen_x" && "$(jsonq "$OUT" "d['requests'][0]['params']['with_content']")" == "true" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b00

if [[ "$LIVE" != "1" ]]; then
  log ""; log "安全模式结束（--live 跑完整场景）。"; log "结果：PASS=$PASS FAIL=$FAIL SKIP=$SKIP"
  [[ $FAIL -eq 0 ]] || exit 1
  exit 0
fi

# ---------- Live 轨道 ----------
log "== Live 轨道（theme=${TEST_THEME}）=="
OSEID=""; CONTAINER=""; CONTAINER_LEN=0; INDEX_DOC=""; BRANCH_TA=""; BRANCH_TB=""; BRANCH_NEW=""
TYPE1=""; ID1=""; TARGET1=""; TYPE3=""; ID3=""; SECTION_NEW=""; SETTINGS1=""

cleanup() {
  [[ -n "$OSEID" ]] || return 0
  for t in "${CREATED_TYPES[@]:-}"; do
    [[ -n "$t" ]] || continue
    run_cli themes block delete-gen --params "{\"oseid\":\"$OSEID\",\"type\":\"$t\"}"
    [[ $CODE -eq 0 ]] && log "  清理 $t" || log "  ⚠️ 清理 $t 失败: $ERR"
  done
}
trap cleanup EXIT

b01() { # 建会话，取一个容器 target
  local name="b01-session"
  run_cli themes +page --template index --theme "$TEST_THEME" --area page
  expect "$name" "+page exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  OSEID=$(jsonq "$OUT" "d['data']['oseid']")
  expect "$name" "拿到 oseid" "$([[ -n "$OSEID" ]] && echo 1 || echo 0)" || return
  # 首选一个没有 blocks 的普通 section（不受其 schema 白名单影响的容器），否则退回首个 section
  CONTAINER=$(jsonq "$OUT" "next((s['section_id'] for s in d['data']['sections'] if s.get('kind')!='pb' and not s.get('blocks')), d['data']['sections'][0]['section_id'])")
  CONTAINER_LEN=$(jsonq "$OUT" "len(next((s for s in d['data']['sections'] if s['section_id']=='$CONTAINER'))['blocks'] or [])")
  expect "$name" "取到容器 section" "$([[ -n "$CONTAINER" ]] && echo 1 || echo 0)" || return
  run_cli themes file tree --params "{\"theme_id\":\"$TEST_THEME\"}"
  INDEX_DOC=$(jsonq "$OUT" "next(i['id'] for i in d['data'].get('doctree',d['data'])['templates'] if i['location']=='index.liquid')")
  expect "$name" "取到 index doc id" "$([[ -n "$INDEX_DOC" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b01
[[ -n "$OSEID" && -n "$CONTAINER" ]] || { log "FATAL: 会话/容器不可用，后续场景无法进行"; exit 14; }

b02() { # 建卡 + 落位到已有容器
  local name="b02-create-place"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_min.liquid" --template index --target "$CONTAINER.blocks"
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  TYPE1=$(jsonq "$OUT" "d['data']['type']"); ID1="${TYPE1#blocks/}"; CREATED_TYPES+=("$TYPE1")
  expect "$name" "type 以 blocks/gen_ 开头" "$([[ "$TYPE1" == blocks/gen_* ]] && echo 1 || echo 0)" || return
  TARGET1=$(jsonq "$OUT" "d['data']['instance']['target']")
  expect "$name" "instance.target == 容器[N]" "$([[ "$TARGET1" == "$CONTAINER.blocks[$CONTAINER_LEN]" ]] && echo 1 || echo 0)" || return
  expect "$name" "revert_id 32 位" "$([[ "$(jsonq "$OUT" "len(d['data']['revert_id'])")" == "32" ]] && echo 1 || echo 0)" || return
  expect "$name" "preview_url 含 oseid" "$([[ "$(jsonq "$OUT" "d['data']['preview_url']")" == *"oseid=$OSEID"* ]] && echo 1 || echo 0)" || return
  expect "$name" "branched false" "$([[ "$(jsonq "$OUT" "d['data']['branched']")" == "False" ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --session "$OSEID" --section "$CONTAINER"
  local got; got=$(jsonq "$OUT" "next(b['type'] for b in (d['data'].get('sections') or [d['data'].get('section')])[0]['blocks'] if b['target']=='$TARGET1')")
  expect "$name" "回读该位置 type 一致" "$([[ "$got" == "$TYPE1" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b02

b03() { # 建卡省略 --template：只写文件
  local name="b03-create-file-only"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_min.liquid"
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  TYPE3=$(jsonq "$OUT" "d['data']['type']"); ID3="${TYPE3#blocks/}"; CREATED_TYPES+=("$TYPE3")
  expect "$name" "instance null" "$([[ "$(jsonq "$OUT" "d['data']['instance']")" == "None" ]] && echo 1 || echo 0)" || return
  expect "$name" "无 preview_url" "$([[ "$(jsonq "$OUT" "'preview_url' in d['data']")" == "False" ]] && echo 1 || echo 0)" || return
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID3"
  expect "$name" "+get ref_count 0 / instances []" "$([[ $CODE -eq 0 && "$(jsonq "$OUT" "d['data']['ref_count']")" == "0" && "$(jsonq "$OUT" "len(d['data']['instances'])")" == "0" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b03

b04() { # 建卡省略 --target：自建 _blocks 容器
  local name="b04-create-new-section"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_min.liquid" --template index
  if [[ $CODE -ne 0 ]]; then
    result FAIL "$name" "后端不接受 _blocks 容器（待后端）: $ERR"; return
  fi
  local t; t=$(jsonq "$OUT" "d['data']['type']"); CREATED_TYPES+=("$t")
  expect "$name" "section_created true" "$(is_true "$(jsonq "$OUT" "d['data']['instance']['section_created']")")" || return
  SECTION_NEW=$(jsonq "$OUT" "d['data']['instance']['target'].split('.')[0]")
  expect "$name" "回显新 section id" "$([[ -n "$SECTION_NEW" && "$SECTION_NEW" != "$CONTAINER" ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --session "$OSEID" --section "$SECTION_NEW"
  local st; st=$(jsonq "$OUT" "(d['data'].get('sections') or [d['data'].get('section')])[0]['type']")
  local bt; bt=$(jsonq "$OUT" "(d['data'].get('sections') or [d['data'].get('section')])[0]['blocks'][0]['type']")
  expect "$name" "回读 _blocks section 含该 block" "$([[ "$st" == "_blocks" && "$bt" == "$t" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b04

b05() { # +get 无 section：清单
  local name="b05-get-list"
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID1"
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "$(get_fail_reason)"; return; }
  expect "$name" "doc.location 以 .liquid 结尾" "$([[ "$(jsonq "$OUT" "d['data']['doc']['location']")" == *.liquid ]] && echo 1 || echo 0)" || return
  expect "$name" "ref_count 1" "$([[ "$(jsonq "$OUT" "d['data']['ref_count']")" == "1" ]] && echo 1 || echo 0)" || return
  expect "$name" "instances[0] == {index, TARGET1}" "$([[ "$(jsonq "$OUT" "d['data']['instances'][0]['template']")" == "index" && "$(jsonq "$OUT" "d['data']['instances'][0]['target']")" == "$TARGET1" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b05

b06() { # +get --section --with-content
  local name="b06-get-section"
  SETTINGS1=$(page_block index "$CONTAINER" "$TARGET1" "b['settings']")
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --section "$CONTAINER" --with-content
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "$(get_fail_reason)"; return; }
  expect "$name" "instance.target 一致" "$([[ "$(jsonq "$OUT" "d['data']['instance']['target']")" == "$TARGET1" ]] && echo 1 || echo 0)" || return
  expect "$name" "settings 是对象且含 title" "$([[ "$(jsonq "$OUT" "d['data']['instance']['settings']['title']")" == "Hello from CLI" ]] && echo 1 || echo 0)" || return
  expect "$name" "doc.content 非空" "$([[ "$(jsonq "$OUT" "len(d['data']['doc']['content'])>100")" == "True" ]] && echo 1 || echo 0)" || return
  expect "$name" "name 来自 schema cname" "$([[ "$(jsonq "$OUT" "d['data']['name']['en-US']")" == "CLI e2e card" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b06

b07() { # 改卡不分叉：v2 schema（+subtitle −bg_color），settings 取 b06；回读走 +page
  local name="b07-update-in-place"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --content "$FX/gen_block_v2.liquid" --settings "$SETTINGS1" --template index --target "$TARGET1"
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  expect "$name" "branched false / type 不变" "$([[ "$(jsonq "$OUT" "d['data']['branched']")" == "False" && "$(jsonq "$OUT" "d['data']['type']")" == "$TYPE1" ]] && echo 1 || echo 0)" || return
  expect "$name" "settings 迁移：title 保留、subtitle 取默认、bg_color 丢弃" "$([[ "$(jsonq "$OUT" "d['data']['settings']['title']")" == "Hello from CLI" && "$(jsonq "$OUT" "d['data']['settings']['subtitle']")" == "v2 subtitle" && "$(jsonq "$OUT" "'bg_color' in d['data']['settings']")" == "False" ]] && echo 1 || echo 0)" || return
  expect "$name" "applied 一条 replace_props success" "$([[ "$(jsonq "$OUT" "d['data']['applied'][0]['op']")" == "replace_props" && "$(jsonq "$OUT" "d['data']['applied'][0]['result']")" == "success" ]] && echo 1 || echo 0)" || return
  expect "$name" "+page 回读实例 subtitle 已写入" "$([[ "$(page_block index "$CONTAINER" "$TARGET1" "b['settings']['subtitle']")" == "v2 subtitle" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b07

b07b() { # 改卡后经 +get 回读源码（依赖 gen-blocks 读接口）
  local name="b07b-get-after-update"
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --section "$CONTAINER" --with-content
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "$(get_fail_reason)"; return; }
  expect "$name" "+get 内容已变为 v2" "$([[ "$(jsonq "$OUT" "'cli-e2e-card--v2' in d['data']['doc']['content']")" == "True" ]] && echo 1 || echo 0)" || return
  expect "$name" "+get 实例 subtitle 已写入" "$([[ "$(jsonq "$OUT" "d['data']['instance']['settings']['subtitle']")" == "v2 subtitle" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b07b

b08() { # 改卡 + --ops
  local name="b08-update-ops"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --content "$FX/gen_block_v2.liquid" --template index --target "$TARGET1" --ops '{"title":"e2e ops"}'
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  expect "$name" "applied 末条 replace_props success" "$([[ "$(jsonq "$OUT" "d['data']['applied'][-1]['op']")" == "replace_props" && "$(jsonq "$OUT" "d['data']['applied'][-1]['result']")" == "success" ]] && echo 1 || echo 0)" || return
  expect "$name" "+page 回读 title == e2e ops" "$([[ "$(page_block index "$CONTAINER" "$TARGET1" "b['settings']['title']")" == "e2e ops" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b08

b09() { # 建卡 + --ops：同批第二条落到新下标
  local name="b09-create-ops"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_min.liquid" --template index --target "$CONTAINER.blocks" --ops '{"title":"created with ops"}'
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  local t tgt; t=$(jsonq "$OUT" "d['data']['type']"); CREATED_TYPES+=("$t"); tgt=$(jsonq "$OUT" "d['data']['instance']['target']")
  expect "$name" "落到下一个下标" "$([[ "$tgt" == "$CONTAINER.blocks[$((CONTAINER_LEN+1))]" ]] && echo 1 || echo 0)" || return
  expect "$name" "+page 回读新实例 title/type" "$([[ "$(page_block index "$CONTAINER" "$tgt" "b['settings']['title']")" == "created with ops" && "$(page_block index "$CONTAINER" "$tgt" "b['type']")" == "$t" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b09

b10() { # 分叉：同一卡两处 → 改其一（定位与回读走 +page）
  local name="b10-branch"
  # 用原始 batch-ops 复制两处 ID3（ref_count 0 → 2），绕开 CLI 白名单
  run_cli themes session batch-ops --params "{\"oseid\":\"$OSEID\",\"doc_id\":\"$INDEX_DOC\"}" --data "{\"operations\":[{\"op\":\"append_array_item\",\"target\":\"$CONTAINER.blocks\",\"value\":{\"type\":\"$TYPE3\",\"settings\":{\"title\":\"A\",\"bg_color\":\"#FFF3E0\"}}},{\"op\":\"append_array_item\",\"target\":\"$CONTAINER.blocks\",\"value\":{\"type\":\"$TYPE3\",\"settings\":{\"title\":\"B\",\"bg_color\":\"#FFF3E0\"}}}]}"
  expect "$name" "预置两处实例" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  run_cli themes +page --template index --theme "$TEST_THEME" --session "$OSEID" --section "$CONTAINER"
  local tA tB
  tA=$(jsonq "$OUT" "[b['target'] for b in (d['data'].get('sections') or [d['data'].get('section')])[0]['blocks'] if b['type']=='$TYPE3'][0]")
  tB=$(jsonq "$OUT" "[b['target'] for b in (d['data'].get('sections') or [d['data'].get('section')])[0]['blocks'] if b['type']=='$TYPE3'][1]")
  expect "$name" "+page 看到两处实例" "$([[ -n "$tA" && -n "$tB" ]] && echo 1 || echo 0)" || return
  BRANCH_TA="$tA"; BRANCH_TB="$tB"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --id "$ID3" --content "$FX/gen_block_v2.liquid" --template index --target "$tA"
  expect "$name" "改其一 exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  local newT; newT=$(jsonq "$OUT" "d['data']['type']"); CREATED_TYPES+=("$newT"); BRANCH_NEW="$newT"
  expect "$name" "branched true / 新 type / previous_type" "$([[ "$(jsonq "$OUT" "d['data']['branched']")" == "True" && "$newT" != "$TYPE3" && "$(jsonq "$OUT" "d['data']['previous_type']")" == "$TYPE3" ]] && echo 1 || echo 0)" || return
  expect "$name" "迁移后 title 保留 A" "$([[ "$(jsonq "$OUT" "d['data']['settings']['title']")" == "A" ]] && echo 1 || echo 0)" || return
  expect "$name" "applied: remove+append(+move) 全 success" "$([[ "$(jsonq "$OUT" "d['data']['applied'][0]['op']")" == "remove_array_item" && "$(jsonq "$OUT" "d['data']['applied'][1]['op']")" == "append_array_item" && "$(jsonq "$OUT" "all(a['result']=='success' for a in d['data']['applied'])")" == "True" ]] && echo 1 || echo 0)" || return
  local gotA gotB; gotA=$(page_block index "$CONTAINER" "$tA" "b['type']"); gotB=$(page_block index "$CONTAINER" "$tB" "b['type']")
  expect "$name" "A 换成新卡、B 保持旧卡" "$([[ "$gotA" == "$newT" && "$gotB" == "$TYPE3" ]] && echo 1 || echo 0)" || return
  expect "$name" "A 的 subtitle 已按新 schema 写入" "$([[ "$(page_block index "$CONTAINER" "$tA" "b['settings'].get('subtitle')")" == "v2 subtitle" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b10

b10b() { # 分叉后经 +get 核对 ref_count 与多命中数组（依赖 gen-blocks 读接口）
  local name="b10b-get-after-branch"
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID3"
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "$(get_fail_reason)"; return; }
  expect "$name" "旧卡 ref_count 回落到 1" "$([[ "$(jsonq "$OUT" "d['data']['ref_count']")" == "1" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b10b

b11() { # 校验错误：exit 2 且零写请求
  local name="b11-validation"
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID1"; local before; before=$(jsonq "$OUT" "d['data']['ref_count']")
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_min.liquid" --target "$CONTAINER.blocks"
  expect "$name" "--target 无 --template → exit 2" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_min.liquid" --template index --target "$TARGET1"
  expect "$name" "无 id 给实例 target → exit 2" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --content "$FX/gen_block_min.liquid" --template index --target "$CONTAINER.blocks"
  expect "$name" "有 id 给容器 target → exit 2" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --content "$FX/gen_block_min.liquid" --settings '{"type":"blocks/gen_other"}'
  expect "$name" "settings.type 错配 → exit 2" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --content "$FX/gen_block_min.liquid" --template index --target "$CONTAINER.blocks[99]"
  expect "$name" "下标越界 → exit 2" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID1"
  expect "$name" "ref_count 不变（无写请求）" "$([[ "$(jsonq "$OUT" "d['data']['ref_count']")" == "$before" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b11

b12() { # 非法 liquid
  local name="b12-bad-liquid"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_bad.liquid" --template index --target "$CONTAINER.blocks"
  expect "$name" "exit 1 api" "$([[ $CODE -eq 1 && "$(jsonq "$ERR" "d['error']['type']")" == "api" ]] && echo 1 || echo 0)" || return
  expect "$name" "stage write" "$([[ "$(jsonq "$ERR" "d['error']['stage']")" == "write" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b12

b13() { # 无效 session
  local name="b13-invalid-session"
  run_cli themes block +get --theme "$TEST_THEME" --session ose_e2e_invalid --id "$ID1"
  expect "$name" "exit 1 且透传 b_invalid_themeid" "$([[ $CODE -eq 1 && "$ERR" == *b_invalid_themeid* ]] && echo 1 || echo 0)" || return
  run_cli themes block +edit --theme "$TEST_THEME" --session ose_e2e_invalid --content "$FX/gen_block_min.liquid"
  expect "$name" "+edit 同样透传" "$([[ $CODE -eq 1 && "$(jsonq "$ERR" "d['error']['stage']")" == "write" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b13

b14() { # 不存在的 id
  local name="b14-unknown-id"
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id gen_doesnotexist
  expect "$name" "exit 1 透传 404" "$([[ $CODE -eq 1 && "$(jsonq "$ERR" "d['error']['detail']['status_code']")" == "404" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b14

b15() { # 同 section 两处同卡 → +get --section 退化为数组（依赖 gen-blocks 读接口）
  local name="b15-get-multi-instance"
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --section "$CONTAINER"
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "$(get_fail_reason)"; return; }
  expect "$name" "单命中为对象" "$([[ "$(jsonq "$OUT" "isinstance(d['data']['instance'],dict)")" == "True" ]] && echo 1 || echo 0)" || return
  run_cli themes session batch-ops --params "{\"oseid\":\"$OSEID\",\"doc_id\":\"$INDEX_DOC\"}" --data "{\"operations\":[{\"op\":\"append_array_item\",\"target\":\"$CONTAINER.blocks\",\"value\":{\"type\":\"$TYPE1\",\"settings\":{\"title\":\"dup\"}}}]}"
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --section "$CONTAINER"
  expect "$name" "两处命中退化为数组" "$([[ $CODE -eq 0 && "$(jsonq "$OUT" "len(d['data']['instance'])")" == "2" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b15

b16() { # 点号 target 与方括号等价
  local name="b16-dot-target"
  local dot="${TARGET1//\[/.}"; dot="${dot//\]/}"
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --id "$ID1" --content "$FX/gen_block_v2.liquid" --template index --target "$dot" --ops '{"title":"dot"}'
  expect "$name" "+edit 点号 target 落到同一下标" "$([[ $CODE -eq 0 && "$(jsonq "$OUT" "d['data']['instance']['target']")" == "$TARGET1" ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  local rtype rid; rtype=$(jsonq "$OUT" "d['data']['type']"); rid="${rtype#blocks/}"
  [[ "$rtype" != "$TYPE1" ]] && CREATED_TYPES+=("$rtype")  # 若分叉，登记新卡以便清理
  expect "$name" "+page 回读 title == dot" "$([[ "$(page_block index "$CONTAINER" "$TARGET1" "b['settings']['title']")" == "dot" ]] && echo 1 || echo 0)" || return
  # 用 dot 形 --section 读该处实例（按结果 type 查，兼容分叉）
  run_cli themes block +get --theme "$TEST_THEME" --session "$OSEID" --id "$rid" --section "$dot"
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "+get 点号 section: $(get_fail_reason)"; return; }
  expect "$name" "+get 点号 section 命中含 TARGET1" "$([[ "$(jsonq "$OUT" "'$TARGET1' in json.dumps(d['data']['instance'])")" == "True" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b16

b17() { # product 模板的 preview 路径
  local name="b17-product-preview"
  run_cli themes +page --template product --theme "$TEST_THEME" --session "$OSEID" --area page
  # 读失败要判 FAIL，不能和「确实没有普通 section」一起吞成 SKIP
  expect "$name" "读 product 模板 exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "    $ERR"; return; }
  local psec; psec=$(jsonq "$OUT" "next((s['section_id'] for s in d['data']['sections'] if s.get('kind')!='pb'), '')")
  [[ -n "$psec" ]] || { result SKIP "$name" "product 模板无普通 section（本店数据所限）"; return; }
  run_cli themes block +edit --theme "$TEST_THEME" --session "$OSEID" --content "$FX/gen_block_min.liquid" --template product --target "$psec.blocks"
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || { log "$ERR"; return; }
  CREATED_TYPES+=("$(jsonq "$OUT" "d['data']['type']")")
  expect "$name" "preview_url 含 /products/" "$([[ "$(jsonq "$OUT" "d['data']['preview_url']")" == *"/products/"* ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
b17

log ""; log "结果：PASS=$PASS FAIL=$FAIL SKIP=$SKIP"
[[ $FAIL -eq 0 ]] || { log "失败场景：${FAILED_NAMES[*]}"; exit 1; }
exit 0
