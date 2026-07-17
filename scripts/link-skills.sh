#!/bin/bash
# 将 taichi/skills/ 下的 AI Agent Skill 软链到 .trae/skills/ 目录
# 使 Trae IDE 的 AI Agent 可以发现并使用这些 Skill

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TAICHI_ROOT="$(dirname "$SCRIPT_DIR")"
WORKSPACE_ROOT="$(dirname "$TAICHI_ROOT")"
TRAE_SKILLS_DIR="$WORKSPACE_ROOT/.trae/skills"

mkdir -p "$TRAE_SKILLS_DIR"

for skill_dir in "$TAICHI_ROOT/skills"/*/; do
    if [ -f "$skill_dir/SKILL.md" ]; then
        skill_name=$(basename "$skill_dir")
        target="$TRAE_SKILLS_DIR/taichi-$skill_name"
        if [ -L "$target" ]; then
            rm "$target"
        fi
        ln -s "$skill_dir" "$target"
        echo "linked: taichi-$skill_name -> $skill_dir"
    fi
done
