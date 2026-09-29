#!/usr/bin/env bash
#
# 飞牛 fnOS 打包脚本：编译后端 + 构建前端 + 用 fnpack 打包生成 .fpk
#
# 注意：必须在飞牛（或任意 Debian/Linux x86_64）上执行，因为：
#   1. 后端是 cgo + libvips，需要 Linux 编译；
#   2. fnpack 是 Linux 可执行文件。
# 用法（root）：
#   bash build-fpk.sh
# 产物：imgconv-1.0.0.fpk（位于脚本同目录）
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FPK_DIR="${SCRIPT_DIR}/fpk"
GO_VERSION="1.25.0"
FNPACK_VERSION="1.2.0"
VERSION="${VERSION:-1.0.0}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

echo "==> [1/6] 安装系统依赖 (gcc / pkg-config / libvips)"
apt-get update
apt-get install -y --no-install-recommends build-essential pkg-config libvips-dev wget ca-certificates

echo "==> [2/6] 安装 Go ${GO_VERSION}"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GO_ARCH="amd64" ;;
  aarch64|arm64) GO_ARCH="arm64" ;;
  *) echo "不支持的架构: ${ARCH}"; exit 1 ;;
esac
TARBALL="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
if [ ! -x "/usr/local/go/bin/go" ] || ! /usr/local/go/bin/go version | grep -q "go${GO_VERSION}"; then
  for base in "https://mirrors.aliyun.com/golang" "https://go.dev/dl"; do
    if wget -q "${base}/${TARBALL}" -O "/tmp/${TARBALL}"; then break; fi
  done
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/${TARBALL}"
  rm -f "/tmp/${TARBALL}"
fi
export PATH="/usr/local/go/bin:${PATH}"

echo "==> [3/6] 编译后端"
cd "${SCRIPT_DIR}/backend"
CGO_ENABLED=1 go build -o "${FPK_DIR}/app/imgconv" .
cd "${SCRIPT_DIR}"

echo "==> [4/6] 构建前端"
if [ ! -d "${SCRIPT_DIR}/frontend/dist" ]; then
  (cd "${SCRIPT_DIR}/frontend" && npm install && npm run build)
fi
rm -rf "${FPK_DIR}/app/dist"
cp -r "${SCRIPT_DIR}/frontend/dist" "${FPK_DIR}/app/dist"

echo "==> [5/6] 准备 fnpack 并写入版本号"
# 更新 manifest 版本号
sed -i "s/^version=.*/version=${VERSION}/" "${FPK_DIR}/manifest"
# 确保生命周期脚本可执行
chmod +x "${FPK_DIR}/cmd/"*

FNPACK=""
if command -v fnpack >/dev/null 2>&1; then
  FNPACK="$(command -v fnpack)"
elif [ -x "${SCRIPT_DIR}/fnpack" ]; then
  FNPACK="${SCRIPT_DIR}/fnpack"
else
  FNPACK="${SCRIPT_DIR}/fnpack"
  wget -q "https://static2.fnnas.com/fnpack/fnpack-${FNPACK_VERSION}-linux-amd64" -O "$FNPACK" || {
    echo "fnpack 下载失败，请手动下载 fnpack 并放到 ${SCRIPT_DIR}/fnpack" >&2
    exit 1
  }
  chmod +x "$FNPACK"
fi

echo "==> [6/6] 打包 .fpk"
cd "${SCRIPT_DIR}"
# 旧版 fnpack 用 `build -d`；若失败则尝试新版 `pack`
if ! "$FNPACK" build -d "$FPK_DIR"; then
  "$FNPACK" pack "$FPK_DIR"
fi

# 定位生成的 .fpk
FPK_FILE="$(find "${SCRIPT_DIR}" "${FPK_DIR}" -maxdepth 3 -name '*.fpk' -print 2>/dev/null | sort | tail -n 1 || true)"
if [ -z "$FPK_FILE" ]; then
  echo "打包完成，但未找到 .fpk 文件，请检查 fnpack 输出。" >&2
  exit 1
fi

echo ""
echo "打包完成！产物: ${FPK_FILE}"
echo "将 .fpk 上传到飞牛「应用中心 → 手动安装」即可。"
