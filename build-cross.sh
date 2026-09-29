#!/usr/bin/env bash
#
# 飞牛 fnOS 交叉编译脚本：Docker buildx 编译 amd64 + arm64 后端，再用 fnpack 打包
# 用法（飞牛 root 下执行）：
#   bash build-cross.sh
# 产物：imgconv-x86-<版本>.fpk 和 imgconv-arm-<版本>.fpk
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

VERSION="$(sed -n 's/^version[[:space:]]*=[[:space:]]*//p' imgConv/manifest | tr -d '[:space:]')"
[ -n "$VERSION" ] || { echo "无法从 imgConv/manifest 读取 version"; exit 1; }

echo "==> [1/5] 确保 Docker + buildx + QEMU"
if ! command -v docker >/dev/null 2>&1; then
  apt-get update
  apt-get install -y --no-install-recommends docker.io
  systemctl enable --now docker
fi
# 注册 QEMU 二进制格式，让 buildx 能模拟 arm64
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes || true
docker buildx create --name imgconv-builder --use 2>/dev/null || docker buildx use imgconv-builder

echo "==> [2/5] 多架构编译后端 (amd64 + arm64)"
rm -rf build-out
docker buildx build --platform linux/amd64,linux/arm64 \
  -f Dockerfile.build \
  --output type=local,dest=build-out .

echo "==> [3/5] 构建前端"
if [ ! -d frontend/dist ]; then
  (cd frontend && npm install && npm run build)
fi

echo "==> [4/5] 准备 fnpack"
FNPACK=""
if command -v fnpack >/dev/null 2>&1; then
  FNPACK="fnpack"
elif [ -x "./fnpack" ]; then
  FNPACK="./fnpack"
else
  wget -q "https://static2.fnnas.com/fnpack/fnpack-1.2.3-linux-amd64" -O fnpack
  chmod +x fnpack
  FNPACK="./fnpack"
fi

echo "==> [5/5] 分别打包 x86 和 arm"
for pair in "amd64 x86" "arm64 arm"; do
  set -- $pair
  arch=$1; platform=$2
  echo "    -> 打包 ${platform} (${arch})"
  cp "build-out/linux_${arch}/imgconv" imgConv/app/imgconv
  chmod +x imgConv/app/imgconv
  rm -rf imgConv/app/dist
  cp -r frontend/dist imgConv/app/dist
  sed -i "s/^platform[[:space:]]*=.*/platform=${platform}/" imgConv/manifest
  chmod +x imgConv/cmd/*
  "$FNPACK" build -d imgConv
  mv imgconv.fpk "imgconv-${platform}-${VERSION}.fpk"
done

# 恢复 manifest platform 为 x86（默认）
sed -i "s/^platform[[:space:]]*=.*/platform=x86/" imgConv/manifest

echo ""
echo "完成！产物："
ls -la imgconv-*.fpk
