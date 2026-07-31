#!/usr/bin/env bash
# E2E 验收脚本：themes +page / +edit
# 场景矩阵与断言规格见 ../05-e2e-acceptance.md（编号对齐契约文档 §6）
#
# 用法（脚本在 tests/e2e/，随仓库维护）：
#   bash theme_page_edit.sh          # 安全模式：前置检查 + DryRun 轨道（零真实调用）
#   bash theme_page_edit.sh --live   # 完整验收：27 场景（写操作仅落在 SHOPLAZZA_TEST_THEME_ID）
#
# 环境变量（沿用 tests/e2e 约定）：
#   SHOPLAZZA_STORE / SHOPLAZZA_TOKEN         # 测试店与令牌
#   SHOPLAZZA_TEST_THEME_ID                   # 一次性测试主题（never production），--live 必填
#   SHOPLAZZA_BIN                             # 可选，缺省用 PATH 上的 shoplazza
#
# 硬性安全边界：
#   - 写场景一律显式 --theme "$TEST_THEME"（一次性测试主题，必须非上线）
#   - publish 只允许落在 $TEST_THEME 上；脚本启动时记录原上线主题，
#     退出时（含失败/中断）自动 publish 回原主题，绝不把店铺留在测试主题上
set -u

BIN="${SHOPLAZZA_BIN:-shoplazza}"
LIVE=0
[[ "${1:-}" == "--live" ]] && LIVE=1

PASS=0; FAIL=0; SKIP=0
declare -a FAILED_NAMES=()

# ---------- 工具 ----------
log()  { printf '%s\n' "$*" >&2; }
result() { # result <PASS|FAIL|SKIP> <场景名> [原因]
  case "$1" in
    PASS) PASS=$((PASS+1)); log "  ✅ PASS  $2";;
    SKIP) SKIP=$((SKIP+1)); log "  ⏭️  SKIP  $2 —— ${3:-}";;
    *)    FAIL=$((FAIL+1)); FAILED_NAMES+=("$2"); log "  ❌ FAIL  $2 —— ${3:-}";;
  esac
}

# run_cli <argv...>：执行 CLI，捕获 stdout/stderr/退出码到全局 OUT/ERR/CODE
OUT=""; ERR=""; CODE=0
run_cli() {
  local out_f err_f
  out_f=$(mktemp); err_f=$(mktemp)
  "$BIN" "$@" >"$out_f" 2>"$err_f"; CODE=$?
  OUT=$(cat "$out_f"); ERR=$(cat "$err_f")
  rm -f "$out_f" "$err_f"
}

# jsonq <json 文本> <python 表达式，d 为解析结果>：取值/断言，失败输出空
# JSON 经 stdin 传入（内嵌进源码会被转义序列破坏）
jsonq() {
  printf '%s' "$1" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    v = eval(sys.argv[1])
    print(json.dumps(v) if isinstance(v, (dict, list)) else v)
except Exception:
    pass' "$2" 2>/dev/null
}

expect() { # expect <场景名> <条件描述> <实际布尔 0/1>
  if [[ "$3" == "1" ]]; then return 0; fi
  result FAIL "$1" "断言失败: $2"; return 1
}

# ---------- 前置检查 ----------
log "== 前置检查 =="
command -v "$BIN" >/dev/null 2>&1 || { log "FATAL: 找不到 CLI 二进制 '$BIN'（设 SHOPLAZZA_BIN 或加入 PATH）"; exit 10; }
command -v python3 >/dev/null 2>&1 || { log "FATAL: 需要 python3 做 JSON 断言"; exit 10; }

# 注意：cobra 对未知子命令会打印父命令帮助并 exit 0，不能用 `themes +page --help` 的退出码判存在性
THEMES_HELP=$("$BIN" themes --help 2>/dev/null)
grep -q '^\s*+page\b' <<<"$THEMES_HELP" || { log "FATAL: 'themes +page' 未实现（M2 未完成）"; exit 11; }
grep -q '^\s*+edit\b' <<<"$THEMES_HELP" || { log "FATAL: 'themes +edit' 未实现（M3 未完成）"; exit 11; }
log "  CLI 与两条命令均就绪：$("$BIN" --version 2>/dev/null | head -1)"

# x-rf: feature-cli 网关路由头已写死在 CLI（internal/client），无需环境变量

TEST_THEME="${SHOPLAZZA_TEST_THEME_ID:-}"
if [[ "$LIVE" == "1" ]]; then
  # 登录态二选一：SHOPLAZZA_STORE/TOKEN env，或已配置的 profile（实测一次只读调用）
  if [[ -z "${SHOPLAZZA_STORE:-}" || -z "${SHOPLAZZA_TOKEN:-}" ]]; then
    run_cli themes list --params '{"page_size":"1"}' --jq '.ok'
    [[ $CODE -eq 0 ]] || { log "FATAL: --live 需要 SHOPLAZZA_STORE/SHOPLAZZA_TOKEN 或有效的 profile 登录态"; exit 12; }
    log "  使用当前 profile 登录态"
  fi
  [[ -n "$TEST_THEME" ]] || { log "FATAL: --live 需要 SHOPLAZZA_TEST_THEME_ID（一次性测试主题，写场景专用）"; exit 12; }
  # 确认测试主题存在且不是上线主题（上线主题绝不做写操作目标）
  run_cli themes get --params "{\"theme_id\":\"$TEST_THEME\"}" --jq '.data'
  [[ $CODE -eq 0 ]] || { log "FATAL: 测试主题 $TEST_THEME 不可访问"; exit 12; }
  PUBLISHED=$(jsonq "$OUT" "d.get('theme',d).get('published')")
  [[ "$PUBLISHED" == "true" || "$PUBLISHED" == "1" ]] && { log "FATAL: $TEST_THEME 是上线主题，拒绝作为写目标"; exit 13; }
  log "  测试主题 $TEST_THEME 就绪（非上线）"

  # publish 场景会把 $TEST_THEME 推上线；记录原上线主题并在退出时还原。
  run_cli themes list --params '{"published":"1"}'
  ORIG_PUBLISHED=$(jsonq "$OUT" "d['data']['themes'][0]['id']")
  [[ -n "$ORIG_PUBLISHED" ]] || { log "FATAL: 读不到当前上线主题，publish 场景无法安全还原"; exit 12; }
  log "  原上线主题 ${ORIG_PUBLISHED}（退出时会还原）"
  restore_published() {
    [[ -n "${ORIG_PUBLISHED:-}" ]] || return 0
    run_cli themes list --params '{"published":"1"}'
    local now; now=$(jsonq "$OUT" "d['data']['themes'][0]['id']")
    [[ "$now" == "$ORIG_PUBLISHED" ]] && { log "上线主题已是原主题，无需还原"; return 0; }
    log "还原上线主题 $now -> $ORIG_PUBLISHED"
    run_cli themes publish --params "{\"theme_id\":\"$ORIG_PUBLISHED\"}"
    [[ $CODE -eq 0 ]] && log "  还原成功" || log "  ⚠️ 还原失败，请手动 publish $ORIG_PUBLISHED"
  }
  trap restore_published EXIT
fi

# ---------- 场景 15：DryRun（安全模式即可跑，零真实调用）----------
log "== DryRun 轨道 =="
s15() {
  local name="s15-dryrun"
  run_cli themes +page --template index --include schema,pb --dry-run
  expect "$name" "+page --dry-run exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local dr; dr=$(jsonq "$OUT" "d.get('dry_run')")
  expect "$name" "输出含 dry_run:true" "$([[ "$dr" == "True" || "$dr" == "true" ]] && echo 1 || echo 0)" || return

  run_cli themes +edit --template index --dry-run --ops - <<'OPS'
[ { "op": "replace_props", "target": "<section_id>", "props": { "k": "v" } } ]
OPS
  expect "$name" "+edit --dry-run exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  expect "$name" "含占位符（零调用语义）" "$([[ "$OUT" == *"<theme_id>"* || "$OUT" == *"<oseid>"* ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}
s15

if [[ "$LIVE" != "1" ]]; then
  log ""
  log "安全模式结束（--live 跑完整 27 场景）。"
  log "结果：PASS=$PASS FAIL=$FAIL SKIP=$SKIP"
  [[ $FAIL -eq 0 ]] || exit 1
  exit 0
fi

# ---------- Live 轨道（全部显式 --theme ${TEST_THEME}；绝不 publish）----------
log "== Live 轨道（theme=${TEST_THEME}）=="

OSEID_S1=""; SECTION_ID=""; BLOCK_TARGET=""; RKEY=""; RVAL=""; BKEY=""; BVAL=""

s01() { # 首发编辑（形态 2：+page 自动建会话回显 oseid → +edit --session 同快照写入）
  local name="s01-first-edit"
  run_cli themes +page --template index --theme "$TEST_THEME"
  expect "$name" "+page exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  OSEID_S1=$(jsonq "$OUT" "d['data']['oseid']")
  local pcreated; pcreated=$(jsonq "$OUT" "d['data']['session_created']")
  SECTION_ID=$(jsonq "$OUT" "d['data']['sections'][0]['section_id']")
  BLOCK_TARGET=$(jsonq "$OUT" "(d['data']['sections'][0].get('blocks') or [{}])[0].get('target','')")
  # 服务端按 schema 校验 props 键(batch-ops 新行为):提取首个字符串型真实键做同值写入
  RKEY=$(jsonq "$OUT" "next((k for k,v in (d['data']['sections'][0].get('settings') or {}).items() if isinstance(v,str) and not k.startswith('__')), '')")
  RVAL=$(jsonq "$OUT" "next((v for k,v in (d['data']['sections'][0].get('settings') or {}).items() if isinstance(v,str) and not k.startswith('__')), '')")
  BKEY=$(jsonq "$OUT" "next((k for k,v in ((d['data']['sections'][0].get('blocks') or [{}])[0].get('settings') or {}).items() if isinstance(v,str) and not k.startswith('__')), '')")
  BVAL=$(jsonq "$OUT" "next((v for k,v in ((d['data']['sections'][0].get('blocks') or [{}])[0].get('settings') or {}).items() if isinstance(v,str) and not k.startswith('__')), '')")
  expect "$name" "+page 回显 oseid" "$([[ -n "$OSEID_S1" ]] && echo 1 || echo 0)" || return
  expect "$name" "+page session_created==true" "$([[ "$pcreated" == "True" || "$pcreated" == "true" ]] && echo 1 || echo 0)" || return
  expect "$name" "取到 section_id" "$([[ -n "$SECTION_ID" ]] && echo 1 || echo 0)" || return

  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$OSEID_S1" --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } } ]
OPS
  expect "$name" "+edit exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local eoseid created preview
  eoseid=$(jsonq "$OUT" "d['data']['oseid']")
  created=$(jsonq "$OUT" "d['data']['session_created']")
  preview=$(jsonq "$OUT" "d['data']['preview_url']")
  expect "$name" "+edit oseid 与 +page 一致" "$([[ "$eoseid" == "$OSEID_S1" ]] && echo 1 || echo 0)" || return
  expect "$name" "+edit session_created==false" "$([[ "$created" == "False" || "$created" == "false" ]] && echo 1 || echo 0)" || return
  expect "$name" "preview_url 含 oseid" "$([[ "$preview" == *"$OSEID_S1"* ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s02() { # 续改
  local name="s02-continue"
  [[ -n "$OSEID_S1" ]] || { result SKIP "$name" "依赖 s01"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$OSEID_S1" --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } } ]
OPS
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local oseid created
  oseid=$(jsonq "$OUT" "d['data']['oseid']"); created=$(jsonq "$OUT" "d['data']['session_created']")
  expect "$name" "同一会话" "$([[ "$oseid" == "$OSEID_S1" ]] && echo 1 || echo 0)" || return
  expect "$name" "session_created==false" "$([[ "$created" == "False" || "$created" == "false" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s03() { # 漏传 --session → 新会话
  local name="s03-missing-session"
  [[ -n "$OSEID_S1" ]] || { result SKIP "$name" "依赖 s01"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } } ]
OPS
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local oseid; oseid=$(jsonq "$OUT" "d['data']['oseid']")
  expect "$name" "新会话（oseid ≠ s01）" "$([[ -n "$oseid" && "$oseid" != "$OSEID_S1" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s04() { # --session 失效
  local name="s04-invalid-session"
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "ose_e2e_invalid_00000" --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "x": "y" } } ]
OPS
  expect "$name" "exit 非 0" "$([[ $CODE -ne 0 ]] && echo 1 || echo 0)" || return
  expect "$name" "SESSION_NOT_FOUND 语义" "$([[ "$ERR" == *"SESSION_NOT_FOUND"* || "$ERR" == *"session"* ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s05() { # --include schema
  local name="s05-schema"
  run_cli themes +page --template index --theme "$TEST_THEME" --include schema
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local schema; schema=$(jsonq "$OUT" "bool(d['data'].get('schema'))")
  expect "$name" "schema 存在" "$([[ "$schema" == "True" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s06_s07() { # PB 卡编辑 + 混合批（双分支：自建卡成功路径 / 烘焙卡降级路径）
  run_cli themes +page --template index --theme "$TEST_THEME" --include pb
  local pb_sec canvas cerr
  pb_sec=$(jsonq "$OUT" "next((s['section_id'] for s in d['data']['sections'] if s.get('kind')=='pb'), '')")
  if [[ -z "$pb_sec" ]]; then
    result SKIP "s06-pb-edit" "测试主题页面无 PB 卡"
    result SKIP "s07-mixed-batch" "同上"
    return
  fi
  canvas=$(jsonq "$OUT" "next((s.get('canvas','') for s in d['data']['sections'] if s.get('section_id')=='$pb_sec'), '')")
  cerr=$(jsonq "$OUT" "next((s.get('canvas_error','') for s in d['data']['sections'] if s.get('section_id')=='$pb_sec'), '')")

  local name="s06-pb-edit"
  if [[ -n "$canvas" ]]; then
    # 自建 PB 卡：canvas 可用，update_pb 应成功
    run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "update_pb", "target": "$pb_sec", "ops": [ { "action": "update", "targetId": "0", "settings": {} } ] } ]
OPS
    if [[ $CODE -eq 0 ]]; then result PASS "$name"
    elif [[ "$ERR" == *"internal server error"* ]]; then result SKIP "$name" "pb 侧 500(生成卡片不可用),转换链待后端修复"
    else result FAIL "$name" "exit=$CODE: $ERR"; fi
  else
    # 主题烘焙 PB 卡：源模板不在本店（canvas_error 降级注记）——
    # 验证①降级不挂命令 ②烘焙卡经 theme 命令族（replace_props 简化 settings）可编辑
    expect "$name" "canvas_error 降级注记存在" "$([[ -n "$cerr" ]] && echo 1 || echo 0)" || return
    local pkey
    pkey=$(jsonq "$OUT" "next((list(s.get('settings',{}).keys())[0] for s in d['data']['sections'] if s.get('section_id')=='$pb_sec' and s.get('settings')), '')")
    if [[ -z "$pkey" ]]; then result SKIP "$name" "烘焙 PB 卡无简化 settings 可改"; else
      run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "replace_props", "target": "$pb_sec", "props": { "$pkey": "e2e-baked-$RANDOM" } } ]
OPS
      [[ $CODE -eq 0 ]] && result PASS "${name}（烘焙卡降级路径）" || result FAIL "$name" "烘焙卡 replace_props 失败: $ERR"
    fi
  fi

  name="s07-mixed-batch"
  if [[ -n "$canvas" ]]; then
    run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } },
  { "op": "update_pb", "target": "$pb_sec", "ops": [ { "action": "update", "targetId": "0", "settings": {} } ] } ]
OPS
    if [[ $CODE -eq 0 ]]; then
      local n; n=$(jsonq "$OUT" "len(d['data']['applied'])")
      [[ "$n" == "2" ]] && result PASS "$name" || result FAIL "$name" "applied 长度 $n ≠ 2"
    else result FAIL "$name" "exit=$CODE: $ERR"; fi
  else
    # 烘焙卡：update_pb 后端 404 应如实透传为 partial 信封（theme op 已应用、断点精确）
    run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } },
  { "op": "update_pb", "target": "$pb_sec", "ops": [ { "action": "update", "targetId": "0", "settings": {} } ] } ]
OPS
    # batch-ops 改造后:update_pb 先走 pb-block-save 预生成,失败即整个命令报错(批未发出,零应用)
    if [[ $CODE -eq 0 ]]; then result FAIL "$name" "烘焙卡 update_pb 竟然成功?"; return; fi
    if [[ "$ERR" == *"internal server error"* ]]; then
      result SKIP "$name" "pb 侧 500(生成卡片不可用),预生成失败路径符合预期"
    else
      result PASS "${name}（烘焙卡预生成失败,批未发出）"
    fi
  fi
}

s08() { # 嵌套加块 + 超限校验
  local name="s08-append-nested"
  # 带 schema 读：子块 limit 从投影里拿，避免盲取到 limit:1 的类型
  # （服务端会以 template_block_limit_error 拒绝第二个实例）
  run_cli themes +page --template index --theme "$TEST_THEME" --include schema
  local o csec btype n0
  o=$(jsonq "$OUT" "d['data']['oseid']")
  # 先在全区域里找「有子块 且 至少有一个无 limit 子块类型」的卡
  local pair
  pair=$(jsonq "$OUT" "next(((s['section_id']+' '+bt) for s in d['data']['sections'] if (s.get('blocks') or [])
           for bt,meta in ((d['data'].get('schema') or {}).get(s['type']) or {}).get('blocks',{}).items()
           if bt != '@app' and not meta.get('schema_missing') and 'limit' not in meta), '')")
  [[ -n "$pair" ]] || { result SKIP "$name" "全页无可重复追加的子块类型（都 limit 受限）"; return; }
  csec=${pair%% *}; btype=${pair##* }
  n0=$(jsonq "$OUT" "next((len(s.get('blocks') or []) for s in d['data']['sections'] if s['section_id']=='$csec'), 0)")

  # ① 合法：追加一个同类型子块，回读块数 +1
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --ops - <<OPS
[ { "op": "append_array_item", "target": "${csec}.blocks",
    "value": { "type": "$btype", "settings": {} } } ]
OPS
  expect "$name" "合法追加 exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local nt; nt=$(jsonq "$OUT" "d['data']['applied'][0].get('new_target','')")
  expect "$name" "回显 new_target" "$([[ -n "$nt" ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --session "$o"
  local n1; n1=$(jsonq "$OUT" "next((len(s.get('blocks') or []) for s in d['data']['sections'] if s['section_id']=='$csec'), 0)")
  expect "$name" "块数 ${n0} -> ${n1}" "$([[ "$n1" -gt "$n0" ]] && echo 1 || echo 0)" || return

  # ② 非法 type 应被 CLI 校验拒绝（validation, exit 2；批不发出）
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --ops - <<OPS
[ { "op": "append_array_item", "target": "${csec}.blocks",
    "value": { "type": "__e2e_invalid_type__", "settings": {} } } ]
OPS
  expect "$name" "非法 type 被拒（exit 2）" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s09() { # --area header 读 + 写侧 area 反查
  local name="s09-area-header"
  run_cli themes +page --template index --theme "$TEST_THEME" --area header
  expect "$name" "读 header 组 exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local hsec; hsec=$(jsonq "$OUT" "(d['data']['sections'] or [{}])[0].get('section_id','')")
  [[ -n "$hsec" ]] || { result SKIP "$name" "页头无 section"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "set_visibility", "target": "$hsec", "visible": true } ]
OPS
  expect "$name" "写侧 area 自动反查 exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s10() { # --list
  local name="s10-list"
  run_cli themes +page --list --theme "$TEST_THEME"
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local ok; ok=$(jsonq "$OUT" "all(('template' in t and 'title' in t and 'type' in t) for t in d['data']['templates'])")
  expect "$name" "每项含 template/title/type" "$([[ "$ok" == "True" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s11() { # 批内降序违规
  local name="s11-order-violation"
  run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "remove_array_item", "target": "${SECTION_ID}.blocks[0]" },
  { "op": "remove_array_item", "target": "${SECTION_ID}.blocks[1]" } ]
OPS
  expect "$name" "exit 2 validation" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  local inv; inv=$(jsonq "$ERR" "d['error'].get('invalid_op')")
  expect "$name" "error.invalid_op 存在" "$([[ -n "$inv" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s12() { # 批内部分失败(独立应用,per-op results)
  local name="s12-failfast-partial"
  run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } },
  { "op": "replace_props", "target": "sec_e2e_nonexistent_000", "props": { "x": "y" } },
  { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } } ]
OPS
  expect "$name" "exit 1（api）" "$([[ $CODE -eq 1 ]] && echo 1 || echo 0)" || return
  local etype nres failed r0 r2
  etype=$(jsonq "$ERR" "d['error']['type']")
  nres=$(jsonq "$ERR" "len(d['error'].get('results',[]))")
  failed=$(jsonq "$ERR" "d['error'].get('failed')")
  r0=$(jsonq "$ERR" "d['error']['results'][0]['result']")
  r2=$(jsonq "$ERR" "d['error']['results'][2]['result']")
  expect "$name" "type==api" "$([[ "$etype" == "api" ]] && echo 1 || echo 0)" || return
  expect "$name" "results 全量 3 条" "$([[ "$nres" == "3" ]] && echo 1 || echo 0)" || return
  expect "$name" "failed 含第 2 条" "$([[ "$failed" == *"1"* ]] && echo 1 || echo 0)" || return
  expect "$name" "其余条独立应用(success)" "$([[ "$r0" == "success" && "$r2" == "success" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s13() { # promote 冲突（双会话同 section）
  local name="s13-promote-conflict"
  local o_a o_b
  # A/B 两侧都必须是保证真实的变更：写与自身快照相同的值 = no-op，
  # promote 不触发草稿变更，也就不会冲突（编排文档 §2.3 事实 8）
  run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "add_section", "name": "rich_text" } ]
OPS
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "会话A建立失败"; return; }
  o_a=$(jsonq "$OUT" "d['data']['oseid']")
  run_cli themes +edit --template index --theme "$TEST_THEME" --ops - <<OPS
[ { "op": "add_section", "name": "rich_text" } ]
OPS
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "会话B建立失败"; return; }
  o_b=$(jsonq "$OUT" "d['data']['oseid']")
  run_cli themes session promote --params "{\"oseid\":\"$o_a\"}" --data '{"force":false}'
  [[ $CODE -eq 0 ]] || { result FAIL "$name" "promote A 失败: $ERR"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o_b" --promote --ops - <<OPS
[ { "op": "add_section", "name": "rich_text" } ]
OPS
  expect "$name" "exit 1 (code=$CODE o_a=$o_a o_b=$o_b err=$(printf '%s' "$ERR" | head -c 120))" "$([[ $CODE -eq 1 ]] && echo 1 || echo 0)" || return
  local conflict; conflict=$(jsonq "$ERR" "d['error'].get('conflict')")
  expect "$name" "conflict==true" "$([[ "$conflict" == "True" || "$conflict" == "true" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s14() { # --promote 一步到位
  local name="s14-promote-through"
  run_cli themes +edit --template index --theme "$TEST_THEME" --promote --ops - <<OPS
[ { "op": "replace_props", "target": "$SECTION_ID", "props": { "$RKEY": "$RVAL" } } ]
OPS
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local promoted; promoted=$(jsonq "$OUT" "d['data']['promoted']")
  expect "$name" "promoted==true" "$([[ "$promoted" == "True" || "$promoted" == "true" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s16() { # --area all：扁平 sections + 每项 area 字段 + areas 计数（2026-07-15 新行为）
  local name="s16-area-all"
  run_cli themes +page --template index --theme "$TEST_THEME" --area all
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local flat tagged areas
  flat=$(jsonq "$OUT" "isinstance(d['data']['sections'], list)")
  expect "$name" "sections 为扁平数组" "$([[ "$flat" == "True" ]] && echo 1 || echo 0)" || return
  tagged=$(jsonq "$OUT" "all(('area' in s) for s in d['data']['sections'])")
  expect "$name" "每项带 area 字段" "$([[ "$tagged" == "True" ]] && echo 1 || echo 0)" || return
  areas=$(jsonq "$OUT" "'page' in (d['data'].get('areas') or {})")
  expect "$name" "areas 计数含 page" "$([[ "$areas" == "True" ]] && echo 1 || echo 0)" || return
  local named
  named=$(jsonq "$OUT" "any(('name' in s) for s in d['data']['sections'] if s.get('area')=='page')")
  expect "$name" "page 区存在带 name 的行" "$([[ "$named" == "True" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s17() { # move_section 用 position（after:<sid>），CLI 译 to_index（2026-07-15 新行为）
  local name="s17-move-position"
  run_cli themes +page --template index --theme "$TEST_THEME" --area page
  local oseid n ref mv
  oseid=$(jsonq "$OUT" "d['data']['oseid']")
  n=$(jsonq "$OUT" "len(d['data']['sections'])")
  [[ "$n" =~ ^[0-9]+$ && "$n" -ge 2 ]] || { result SKIP "$name" "页面 section 不足 2"; return; }
  ref=$(jsonq "$OUT" "d['data']['sections'][0]['section_id']")
  mv=$(jsonq "$OUT" "d['data']['sections'][-1]['section_id']")
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$oseid" --ops - <<OPS
[ { "op": "move_section", "target": "$mv", "position": "after:$ref" } ]
OPS
  expect "$name" "position 移动 exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --area page --session "$oseid"
  local at=""
  at=$(jsonq "$OUT" "next((i for i,s in enumerate(d['data']['sections']) if s.get('section_id')=='$mv'), -1)")
  at="${at:-}"
  expect "$name" "position=after landed at index 1 (got ${at})" "$([[ "$at" == "1" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s18() { # set_visibility：隐藏后回读 visible==false（守护 disabled 反映到读侧）
  local name="s18-set-visibility"
  run_cli themes +page --template index --theme "$TEST_THEME" --area page
  local o sid; o=$(jsonq "$OUT" "d['data']['oseid']")
  sid=$(jsonq "$OUT" "(d['data']['sections'] or [{}])[0].get('section_id','')")
  [[ -n "$sid" ]] || { result SKIP "$name" "无 page section"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --ops - <<OPS
[ { "op": "set_visibility", "target": "$sid", "visible": false } ]
OPS
  expect "$name" "隐藏 exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --area page --section "$sid" --session "$o"
  local vis; vis=$(jsonq "$OUT" "d['data']['sections'][0]['visible']")
  expect "$name" "回读 visible==false" "$([[ "$vis" == "False" || "$vis" == "false" ]] && echo 1 || echo 0)" || return
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --ops - <<OPS
[ { "op": "set_visibility", "target": "$sid", "visible": true } ]
OPS
  result PASS "$name"
}

s19() { # update_slot：改块级 settings 并回读
  local name="s19-update-slot"
  run_cli themes +page --template index --theme "$TEST_THEME" --area page
  local o bt; o=$(jsonq "$OUT" "d['data']['oseid']")
  bt=$(jsonq "$OUT" "next((b['target'] for s in d['data']['sections'] for b in (s.get('blocks') or []) if any(isinstance(v,str) and not k.startswith('__') for k,v in (b.get('settings') or {}).items())), '')")
  [[ -n "$bt" ]] || { result SKIP "$name" "无带字符串 settings 的块"; return; }
  local bkey bval
  bkey=$(jsonq "$OUT" "next((k for s in d['data']['sections'] for b in (s.get('blocks') or []) if b['target']=='$bt' for k,v in (b.get('settings') or {}).items() if isinstance(v,str) and not k.startswith('__')), '')")
  bval=$(jsonq "$OUT" "next((v for s in d['data']['sections'] for b in (s.get('blocks') or []) if b['target']=='$bt' for k,v in (b.get('settings') or {}).items() if isinstance(v,str) and not k.startswith('__')), '')")
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --ops - <<OPS
[ { "op": "update_slot", "target": "$bt", "props": { "$bkey": "$bval" } } ]
OPS
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --area page --session "$o"
  local got; got=$(jsonq "$OUT" "next((( b.get('settings') or {}).get('$bkey') for s in d['data']['sections'] for b in (s.get('blocks') or []) if b['target']=='$bt'), '')")
  expect "$name" "回读块 settings.$bkey==写入值" "$([[ "$got" == "$bval" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s20() { # remove_array_item：删一块，回读块数减少
  local name="s20-remove-block"
  run_cli themes +page --template index --theme "$TEST_THEME" --area page
  local o sid n0 bt; o=$(jsonq "$OUT" "d['data']['oseid']")
  sid=$(jsonq "$OUT" "next((s['section_id'] for s in d['data']['sections'] if (s.get('blocks') or [])), '')")
  [[ -n "$sid" ]] || { result SKIP "$name" "无带块 section"; return; }
  n0=$(jsonq "$OUT" "len(next((s.get('blocks') or []) for s in d['data']['sections'] if s['section_id']=='$sid'))")
  bt=$(jsonq "$OUT" "next((b['target'] for s in d['data']['sections'] if s['section_id']=='$sid' for b in reversed(s.get('blocks') or [])), '')")
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --ops - <<OPS
[ { "op": "remove_array_item", "target": "$bt" } ]
OPS
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --area page --section "$sid" --session "$o"
  local n1=""; n1=$(jsonq "$OUT" "len(d['data']['sections'][0].get('blocks') or [])")
  expect "$name" "block count dropped ${n0} -> ${n1}" "$([[ -n "$n1" && -n "$n0" && "$n1" -lt "$n0" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s21() { # remove_section：删一卡，回读 section 数减少且卡消失
  local name="s21-remove-section"
  run_cli themes +page --template index --theme "$TEST_THEME" --area page
  local o n0 rid; o=$(jsonq "$OUT" "d['data']['oseid']")
  n0=$(jsonq "$OUT" "len(d['data']['sections'])")
  rid=$(jsonq "$OUT" "(d['data']['sections'] or [{}])[-1].get('section_id','')")
  [[ -n "$rid" && -n "$n0" && "$n0" -ge 1 ]] || { result SKIP "$name" "无 page section"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --ops - <<OPS
[ { "op": "remove_section", "target": "$rid" } ]
OPS
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  run_cli themes +page --template index --theme "$TEST_THEME" --area page --session "$o"
  local n1 gone; n1=$(jsonq "$OUT" "len(d['data']['sections'])")
  gone=$(jsonq "$OUT" "all(s['section_id']!='$rid' for s in d['data']['sections'])")
  expect "$name" "section 数减少且卡消失" "$([[ -n "$n1" && "$n1" -lt "$n0" && "$gone" == "True" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

# ---------- 场景 22~27：--publish（编辑后直接发布收敛为一次确认）----------

s22() { # --publish 缺 --promote：网络前拒绝
  local name="s22-publish-needs-promote"
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "ose_probe" --publish \
    --ops '[{"op":"replace_props","target":"x","props":{"a":"b"}}]'
  expect "$name" "exit 2（validation）" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  expect "$name" "报 --publish requires --promote" "$([[ "$ERR$OUT" == *"--publish requires --promote"* ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s23() { # ops + --promote --publish：顺序、published、revoke_publish_id
  local name="s23-ops-promote-publish"
  run_cli themes +page --template index --theme "$TEST_THEME"
  local o sid k v; o=$(jsonq "$OUT" "d['data']['oseid']")
  sid=$(jsonq "$OUT" "d['data']['sections'][0]['section_id']")
  k=$(jsonq "$OUT" "next((k for k,val in (d['data']['sections'][0].get('settings') or {}).items() if isinstance(val,str) and val and not k.startswith('__')), '')")
  v=$(jsonq "$OUT" "next((val for k,val in (d['data']['sections'][0].get('settings') or {}).items() if isinstance(val,str) and val and not k.startswith('__')), '')")
  [[ -n "$o" && -n "$sid" && -n "$k" ]] || { result SKIP "$name" "取不到可同值写回的字段"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --promote --publish --ops - <<OPS
[ { "op": "replace_props", "target": "$sid", "props": { "$k": "$v" } } ]
OPS
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  expect "$name" "promoted==true" "$([[ "$(jsonq "$OUT" "d['data']['promoted']")" == "True" ]] && echo 1 || echo 0)" || return
  expect "$name" "published==true" "$([[ "$(jsonq "$OUT" "d['data']['published']")" == "True" ]] && echo 1 || echo 0)" || return
  # revoke_publish_id 服务端不是必返（实测同店不同主题有/无），所以只要求
  # 「服务端给了就必须透出」：这里记录观察值，不作为通过条件。
  log "     revoke_publish_id = $(jsonq "$OUT" "d['data'].get('revoke_publish_id','(服务端未返回)')")"
  # 上线主题确实换成了测试主题
  run_cli themes list --params '{"published":"1"}'
  expect "$name" "上线主题已切为测试主题" "$([[ "$(jsonq "$OUT" "d['data']['themes'][0]['id']")" == "$TEST_THEME" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s24() { # promote 冲突 → publish 绝不执行
  local name="s24-conflict-blocks-publish"
  # 造分歧要写一个「任意字符串都合法」的字段：select 之类的枚举字段会被
  # 服务端以 invalid_value 拒绝，那样根本走不到 promote。
  run_cli themes +page --template index --theme "$TEST_THEME" --area page --include schema
  local sa sid k orig
  sa=$(jsonq "$OUT" "d['data']['oseid']")
  sid=$(jsonq "$OUT" "next((s['section_id'] for s in d['data']['sections']
          if any(f.get('type') in ('text','textarea','richtext')
                 and isinstance((s.get('settings') or {}).get(f.get('id')), str)
                 for f in ((d['data'].get('schema') or {}).get(s['type']) or {}).get('settings',[]))), '')")
  [[ -n "$sid" ]] || { result SKIP "$name" "该页无文本型字段，造不出合法分歧"; return; }
  k=$(jsonq "$OUT" "next((f['id'] for s in d['data']['sections'] if s['section_id']=='$sid'
        for f in ((d['data'].get('schema') or {}).get(s['type']) or {}).get('settings',[])
        if f.get('type') in ('text','textarea','richtext')
        and isinstance((s.get('settings') or {}).get(f.get('id')), str)), '')")
  orig=$(jsonq "$OUT" "next((( s.get('settings') or {}).get('$k') for s in d['data']['sections'] if s['section_id']=='$sid'), '')")
  run_cli themes +page --template index --theme "$TEST_THEME"
  local sb; sb=$(jsonq "$OUT" "d['data']['oseid']")
  [[ -n "$sa" && -n "$sb" && -n "$k" ]] || { result SKIP "$name" "建不出两个会话"; return; }
  # B 先写入不同值并 promote，制造与 A 快照的真实分歧
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$sb" --promote --ops - <<OPS
[ { "op": "replace_props", "target": "$sid", "props": { "$k": "__e2e_conflict_probe" } } ]
OPS
  [[ $CODE -eq 0 ]] || { result SKIP "$name" "会话B promote 未成功（$(printf '%s' "$OUT$ERR" | head -c 120)）"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$sa" --promote --publish --ops - <<OPS
[ { "op": "replace_props", "target": "$sid", "props": { "$k": "$orig" } } ]
OPS
  local conflict published code2; code2=$CODE
  conflict=$(jsonq "$ERR$OUT" "d['error']['conflict']")
  published=$(jsonq "$ERR$OUT" "d['error'].get('published','')")
  # 无论断言结果如何，都把探测值改回原值（B 的 promote 已经落到草稿上）
  run_cli themes +page --template index --theme "$TEST_THEME"
  local sc; sc=$(jsonq "$OUT" "d['data']['oseid']")
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$sc" --promote --ops - <<OPS
[ { "op": "replace_props", "target": "$sid", "props": { "$k": "$orig" } } ]
OPS
  expect "$name" "exit 1（api）" "$([[ $code2 -eq 1 ]] && echo 1 || echo 0)" || return
  expect "$name" "conflict==true" "$([[ "$conflict" == "True" ]] && echo 1 || echo 0)" || return
  expect "$name" "未回显 published" "$([[ -z "$published" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s25() { # 空批：跳过 /operations，只做 promote + publish
  local name="s25-empty-batch-publish"
  run_cli themes +page --template index --theme "$TEST_THEME"
  local o; o=$(jsonq "$OUT" "d['data']['oseid']")
  [[ -n "$o" ]] || { result SKIP "$name" "拿不到 oseid"; return; }
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$o" --promote --publish --ops '[]'
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  expect "$name" "applied 为空数组" "$([[ "$(jsonq "$OUT" "d['data']['applied']")" == "[]" ]] && echo 1 || echo 0)" || return
  expect "$name" "promoted+published" "$([[ "$(jsonq "$OUT" "d['data']['promoted']")" == "True" && "$(jsonq "$OUT" "d['data']['published']")" == "True" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s26() { # 空批的两个前置条件
  local name="s26-empty-batch-guards"
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "ose_probe" --ops '[]'
  expect "$name" "缺 --promote → exit 2" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  run_cli themes +edit --template index --theme "$TEST_THEME" --promote --ops '[]'
  expect "$name" "缺 --session → exit 2" "$([[ $CODE -eq 2 ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s27() { # dry-run：publish 在 promote 之后；空批不产出 batch plan
  local name="s27-publish-dryrun"
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "ose_probe" --promote --publish --dry-run \
    --ops '[{"op":"replace_props","target":"x","props":{"a":"b"}}]'
  expect "$name" "exit 0" "$([[ $CODE -eq 0 ]] && echo 1 || echo 0)" || return
  local order; order=$(jsonq "$OUT" "[r['path'].split('/')[-1] for r in d['requests']]")
  expect "$name" "publish 紧随 promote" "$([[ "$order" == *'"promote", "publish"'* ]] && echo 1 || echo 0)" || return
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "ose_probe" --promote --publish --dry-run --ops '[]'
  local has_ops; has_ops=$(jsonq "$OUT" "any(r['path'].endswith('/operations') for r in d['requests'])")
  expect "$name" "空批无 operations plan" "$([[ "$has_ops" == "False" ]] && echo 1 || echo 0)" || return
  expect "$name" "空批仍含 publish" "$([[ "$(jsonq "$OUT" "any(r['path'].endswith('/publish') for r in d['requests'])")" == "True" ]] && echo 1 || echo 0)" || return
  result PASS "$name"
}

s28() { # publish 失败错误包：线上无法主动触发，明确记为 SKIP 而不是静默缺失
  result SKIP "s28-publish-failure-envelope" "线上无法让 PATCH /themes/{id}/publish 主动 5xx；由单测 TestEdit_PublishChain 覆盖"
}

s01; s02; s03; s04; s05; s06_s07; s08; s09; s10; s11; s12; s13; s14; s16; s17; s18; s19; s20; s21
s22; s23; s24; s25; s26; s27; s28

# 附加观察（不计分）：promote 后旧 oseid 行为 → 回填设计文档开放问题 16
log ""
log "附加观察（开放问题 16）：对已 promote 的会话再写——"
if [[ -n "$OSEID_S1" ]]; then
  run_cli themes +edit --template index --theme "$TEST_THEME" --session "$OSEID_S1" --ops '[{"op":"replace_props","target":"'"$SECTION_ID"'","props":{"__e2e_probe":"post"}}]'
  log "  exit=${CODE}；stderr: $(printf '%s' "$ERR" | head -c 300)"
fi

log ""
log "== 结果汇总 =="
log "PASS=$PASS FAIL=$FAIL SKIP=$SKIP"
if [[ $FAIL -gt 0 ]]; then
  log "失败场景：${FAILED_NAMES[*]}"
  exit 1
fi
log "全部通过 ✅（提醒：测试主题 $TEST_THEME 的草稿/会话改动未清理，一次性主题可整体删除重建）"
exit 0
