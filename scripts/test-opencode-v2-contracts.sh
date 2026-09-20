#!/usr/bin/env bash
# Exact released SDK check; no lifecycle scripts, global install, or user config.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/gentle-ai-opencode-v2.XXXXXX")
fixture=$(cd "$fixture" && pwd -P)
trap 'rm -rf "$fixture"' EXIT
node=$(command -v node)
npm=$(command -v npm)
mkdir -p "$fixture/home" "$fixture/assets" "$fixture/tmp"
touch "$fixture/user.npmrc" "$fixture/global.npmrc"
cp "$root"/internal/assets/opencode/plugins-v2/*.ts "$fixture/assets/"
printf '{"private":true,"type":"module"}\n' > "$fixture/package.json"
cd "$fixture"
clean_env=(env -i "HOME=$fixture/home" "XDG_CONFIG_HOME=$fixture/config" "XDG_DATA_HOME=$fixture/data" "XDG_STATE_HOME=$fixture/state" "XDG_CACHE_HOME=$fixture/cache" "TMPDIR=$fixture/tmp" "PATH=$(dirname "$node"):/usr/bin:/bin")
"${clean_env[@]}" "$node" "$npm" install --ignore-scripts --no-audit --no-fund \
  --userconfig="$fixture/user.npmrc" --globalconfig="$fixture/global.npmrc" \
  --cache="$fixture/npm-cache" --registry=https://registry.npmjs.org --save-exact \
  @opencode/plugin@2.0.4 typescript@5.8.2 @types/node@24.12.2
"${clean_env[@]}" "$node" node_modules/typescript/bin/tsc --noEmit --strict \
  --target es2022 --module nodenext --moduleResolution nodenext --skipLibCheck assets/*.ts
