#!/usr/bin/env bash
#
# selftest_interaction.sh — L2 自测：交互功能的「非交互契约」回归。
#
# 只覆盖可自动化的一半：Agent/管道/CI 路径。对每个本轮改动的命令，断言在非交互
# 下（stdin 关闭）—— 缺必填 → 结构化 "required flag(s) not set"；破坏性 → 不弹确认
# 直接放行；--dry-run → 返回预览 —— 且全部快速返回、绝不挂起（超时即判 FAIL）。
#
# 交互的另一半（真实 TTY 下的提示/选择/确认/取消/回退）无法在此脚本验证，见
# docs/M3_INTERACTION_TEST_PLAN.md 的 L3 手动用例。
#
# 用法：bash scripts/selftest_interaction.sh
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$(mktemp -d)/shoplazza"
TIMEOUT=8
pass=0 fail=0

echo "building binary…"
( cd "$ROOT" && go build -trimpath -o "$BIN" . ) || { echo "BUILD FAILED"; exit 1; }

# check <name> <expect-substring> -- <cli args...>
# 断言：命令在 TIMEOUT 内返回（不挂起）且输出含 expect。
check() {
  local name="$1" expect="$2"; shift 2; [ "$1" = "--" ] && shift
  local out rc
  out="$(timeout "$TIMEOUT" "$BIN" "$@" </dev/null 2>&1)"; rc=$?
  if [ "$rc" -eq 124 ]; then
    printf "  FAIL  %-46s (挂起 / 超时 %ss)\n" "$name" "$TIMEOUT"; fail=$((fail+1)); return
  fi
  if printf '%s' "$out" | grep -qF -- "$expect"; then
    printf "  PASS  %-46s\n" "$name"; pass=$((pass+1))
  else
    printf "  FAIL  %-46s (未见 '%s')\n" "$name" "$expect"
    printf '        got: %s\n' "$(printf '%s' "$out" | head -1)"; fail=$((fail+1))
  fi
}

echo "── I1/I2 缺必填 → 结构化错误、不挂起 ──"
check "shortcut fill: products +create"      "required flag(s) not set" -- products +create
check "shortcut fill: orders +refund"        "--order-id"               -- orders +refund --amount 5
check "shortcut fill: themes push"           "--theme-id"               -- themes push
check "cmd fill: checkout deploy"            "--extension-id"           -- checkout-extension deploy
check "cmd fill: app extension create"       "--type"                   -- app extension create
check "cmd fill: app function compile"       "--name"                   -- app function compile

echo "── I4 --dry-run → 预览（确认/网络前返回） ──"
check "dynamic dry-run: webhook delete"      "dry_run"                  -- webhook delete --params '{"id":"1"}' --dry-run
check "cmd dry-run: checkout deploy"         "dry_run"                  -- checkout-extension deploy --extension-id E1 --version 1.0 --dry-run

echo "── I1 破坏性非交互 → 不弹确认、直接放行（快速失败于鉴权/参数，不挂起） ──"
check "dynamic destructive: webhook delete"  "error"                    -- webhook delete --params '{"id":"1"}'

echo "── I2 --format json 下仍为结构化 error ──"
check "json 缺必填: products +create"        "\"type\": \"validation\"" -- products +create --format json

echo ""
echo "结果：PASS=$pass  FAIL=$fail"
[ "$fail" -eq 0 ] || exit 1
