#!/usr/bin/env bash
set -euo pipefail
: "${RELEASE_TAG:?}" "${RELEASE_VERSION:?}" "${GH_REPO:?}" "${GH_TOKEN:?}"
python3 tools/release.py validate "$RELEASE_VERSION"
[[ "$RELEASE_TAG" == "v$RELEASE_VERSION" ]]
assets=("dist/release/mod-list-discovery-server-${RELEASE_VERSION}-linux-amd64"
        "dist/release/mod-list-discovery-${RELEASE_VERSION}+mc26.2.jar"
        dist/release/config.example.yaml dist/release/SHA256SUMS)
for asset in "${assets[@]}"; do test -s "$asset"; done
(cd dist/release && sha256sum --check SHA256SUMS)
# Stage all assets in a draft so users never see a half-uploaded release.
if draft=$(gh release view "$RELEASE_TAG" --json isDraft --jq '.isDraft'); then
  if [[ "$draft" != true ]]; then
    echo "Release $RELEASE_TAG is already published; its assets will not be replaced."
    exit 0
  fi
else
  notes=$(mktemp)
  trap 'rm -f "$notes"' EXIT
  cat > "$notes" <<'NOTES'
## 下载说明
- `*.jar`：Minecraft 26.2 的 Fabric 客户端 MOD；需要 Java 25+、Fabric Loader 0.19.0+ 和匹配游戏版本的 Fabric API。
- `*-linux-amd64`：Linux x86_64 服务端二进制，内嵌管理页面；下载后先执行 `chmod +x`。
- `config.example.yaml`：配置模板，不包含运行数据或凭证。
- `SHA256SUMS`：在同一目录运行 `sha256sum --check SHA256SUMS` 校验下载文件。

安装、配置、玩家使用和技术资料请查看本标签下的 README 与 docs/README.md。
NOTES
  flags=()
  if [[ "$RELEASE_VERSION" == *-* ]]; then flags+=(--prerelease); fi
  gh release create "$RELEASE_TAG" --verify-tag --draft --generate-notes \
    --title "Mod List Discovery $RELEASE_VERSION" --notes-file "$notes" "${flags[@]}"
fi
# Clobber is allowed only while resuming an unpublished draft.
gh release upload "$RELEASE_TAG" "${assets[@]}" --clobber
flags=(--draft=false)
if [[ "$RELEASE_VERSION" == *-* ]]; then flags+=(--prerelease --latest=false); else flags+=(--prerelease=false); fi
gh release edit "$RELEASE_TAG" "${flags[@]}"
