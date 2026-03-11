#!/usr/bin/env bash
# （默认下文所提及的{dir}为脚本所处的位置）
# 用法：
#   ./build_and_run.sh            # 正常 build + 运行
#   ./build_and_run.sh -update    # 先同步上层 ./src/* 到 {dir}/alist-web/ 并执行 i18n，再 build + 运行

set -Eeuo pipefail

trap 'echo "[ERROR] line=$LINENO cmd=$BASH_COMMAND" >&2' ERR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEB_DIR="${SCRIPT_DIR}/alist-web"
BACKEND_DIR="${SCRIPT_DIR}/alist-backend"
BACKEND_PUBLIC_DIR="${BACKEND_DIR}/public"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "[ERROR] missing command: $1" >&2; exit 127; }
}

UPDATE=false
SERVER_ARGS=()
for arg in "$@"; do
  case "$arg" in
    -update) UPDATE=true ;;
    -h|--help)
      sed -n '1,30p' "$0"
      exit 0
      ;;
    *) SERVER_ARGS+=("$arg") ;;
  esac
done

# 基础依赖检查
apt-get update && apt-get install -y musl-tools
need_cmd bash
need_cmd pnpm
need_cmd node
need_cmd go
need_cmd git
need_cmd wget
need_cmd awk
need_cmd sed
need_cmd grep
need_cmd date
need_cmd cp
need_cmd mkdir
need_cmd rm

log() { echo "[$(date +'%F %T')] $*"; }

if $UPDATE; then
  # 去到 {dir} 的上层文件夹，复制 ./src/* 到 {dir}/alist-web/ 下，然后执行 i18n

  log "UPDATE mode: sync ${SCRIPT_DIR}/src/* -> ${WEB_DIR}/src"
  mkdir -p "$WEB_DIR"
  # 复制 ./src/* 到 alist-web（包含目录/文件），尽量保留属性
  # 等价于从上层目录执行：cp -a ./src/* {dir}/alist-web/
  shopt -s dotglob nullglob
  cp -a "${SCRIPT_DIR}/src/"* "${WEB_DIR}/src"
  shopt -u dotglob nullglob

  log "Run i18n: (cd ${WEB_DIR} && node scripts/i18n.mjs)"
  ( cd "$WEB_DIR" && node "./scripts/i18n.mjs" )
fi

# 1) 在 {dir}/alist-web 执行 pnpm install && pnpm build
log "Build web: (cd ${WEB_DIR} && pnpm install && pnpm build)"
cd "$WEB_DIR"
export HUSKY=0
pnpm install
pnpm build

# 2) 复制 {dir}/alist-web/dist/* 到 {dir}/alist-backend/public/
log "Copy web dist to backend public: ${WEB_DIR}/dist/* -> ${BACKEND_PUBLIC_DIR}/dist"
mkdir -p "$BACKEND_PUBLIC_DIR/dist"
cp -a "${WEB_DIR}/dist/." "$BACKEND_PUBLIC_DIR/dist"

# 3) 在 {dir}/alist-backend 执行 go build（带 ldflags），然后 ./alist server
log "Build backend: (cd ${BACKEND_DIR} && go build ...)"
cd "$BACKEND_DIR"
export GOOS=linux GOARCH=amd64
export CGO_ENABLED=1
export CC=x86_64-linux-musl-gcc
builtAt="$(date +'%F %T %z')"
goVersion="$(go version | sed 's/go version //')"
gitAuthor="AA"
gitCommit="bb"
version="cc"
webVersion="$(wget -qO- -t1 -T2 "https://api.github.com/repos/alist-org/alist-web/releases/latest" \
  | grep "tag_name" | head -n 1 | awk -F ":" '{print $2}' | sed 's/\"//g;s/,//g;s/ //g')"

#ldflags="\
#-s -w \
#-X github.com/alist-org/alist/v3/internal/conf.BuiltAt=${builtAt} \
#-X github.com/alist-org/alist/v3/internal/conf.GoVersion=${goVersion} \
#-X github.com/alist-org/alist/v3/internal/conf.GitAuthor=${gitAuthor} \
#-X github.com/alist-org/alist/v3/internal/conf.GitCommit=${gitCommit} \
#-X github.com/alist-org/alist/v3/internal/conf.Version=${version} \
#-X github.com/alist-org/alist/v3/internal/conf.WebVersion=${webVersion} \
#"

# 4) 构建（输出名你要用 alist 或 $appName 都行）
appName="${appName:-alist}"
#go build -trimpath -ldflags="$ldflags" -o "$appName" .
go build -trimpath -ldflags "-s -w -linkmode external -extldflags '-static'" -o alist .


# log "Run: ${BACKEND_DIR}/./${appName} server ${SERVER_ARGS[*]-}"
# exec "./${appName}" server "${SERVER_ARGS[@]}"
