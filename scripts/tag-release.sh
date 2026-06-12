#!/usr/bin/env bash
# =============================================================================
# tag-release.sh — 打标签辅助脚本
#
# 用法：
#   bash scripts/tag-release.sh           # 从 package.json 读取版本
#   bash scripts/tag-release.sh 1.2.0     # 手动指定版本（不含 v 前缀）
#
# 前置要求：
#   - 当前分支为 main
#   - package.json 已提交（无 uncommitted changes）
#   - 当前分支已与 remote 同步
#   - 对应标签尚未存在
# =============================================================================

set -euo pipefail

# ── 版本号 ────────────────────────────────────────────────────────────────────

if [[ $# -ge 1 ]]; then
    VERSION="$1"
else
    # 从 package.json 读取
    if ! command -v node &>/dev/null; then
        echo "Error: node not found. Pass version explicitly: bash scripts/tag-release.sh 1.0.0" >&2
        exit 1
    fi
    VERSION=$(node -p "require('./package.json').version")
fi

# 去掉可能的 v 前缀，再统一加上
VERSION="${VERSION#v}"
TAG="v${VERSION}"

echo "Preparing to tag: ${TAG}"

# ── 校验：必须在 main 分支 ──────────────────────────────────────────────────

BRANCH=$(git symbolic-ref --short HEAD 2>/dev/null || echo "")
if [[ "$BRANCH" != "main" ]]; then
    echo "Error: must be on main branch (currently on '${BRANCH}')" >&2
    exit 1
fi

# ── 校验：package.json 已提交 ─────────────────────────────────────────────────

if ! git diff --quiet -- package.json || ! git diff --cached --quiet -- package.json; then
    echo "Error: package.json has uncommitted changes. Commit it first." >&2
    exit 1
fi

# ── 校验：本地与 remote 同步 ──────────────────────────────────────────────────

git fetch origin main --quiet
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)
if [[ "$LOCAL" != "$REMOTE" ]]; then
    echo "Error: local main is out of sync with origin/main." >&2
    echo "  local:  ${LOCAL}" >&2
    echo "  remote: ${REMOTE}" >&2
    echo "Run 'git pull origin main' or 'git push origin main' first." >&2
    exit 1
fi

# ── 校验：标签不重复 ──────────────────────────────────────────────────────────

if git rev-parse "${TAG}" &>/dev/null; then
    echo "Error: tag ${TAG} already exists locally. Did you mean to bump the version?" >&2
    exit 1
fi

if git ls-remote --tags origin "${TAG}" | grep -q "${TAG}"; then
    echo "Error: tag ${TAG} already exists on remote." >&2
    exit 1
fi

# ── 确认 ──────────────────────────────────────────────────────────────────────

echo
echo "  Branch:  ${BRANCH} (in sync with origin)"
echo "  Tag:     ${TAG}"
echo "  Commit:  ${LOCAL}"
echo
read -r -p "Push tag ${TAG}? [y/N] " CONFIRM
if [[ "$CONFIRM" != "y" && "$CONFIRM" != "Y" ]]; then
    echo "Aborted."
    exit 0
fi

# ── 打标签并推送 ──────────────────────────────────────────────────────────────

git tag -a "${TAG}" -m "Release ${TAG}"
git push origin "${TAG}"

echo
echo "Tag ${TAG} pushed — GitHub Actions release workflow will trigger shortly."
echo "Monitor: https://github.com/Shoplazza/shoplazza-cli/actions"
