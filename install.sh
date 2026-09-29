#!/usr/bin/env bash
#
# 飞牛 fnOS (Debian) 原生安装脚本 —— 不使用 Docker
# 用法：将整个项目目录上传到飞牛后，在项目根目录执行：
#   sudo bash install.sh
#
set -euo pipefail

APP_NAME="imgconv"
INSTALL_DIR="/opt/${APP_NAME}"
GO_VERSION="1.25.0"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 国内网络优先走 goproxy.cn，避免拉取依赖超时
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

echo "==> [1/5] 安装系统依赖 (gcc / pkg-config / libvips)"
apt-get update
apt-get install -y --no-install-recommends \
    build-essential pkg-config libvips-dev ca-certificates wget

echo "==> [2/5] 安装 Go ${GO_VERSION}（libvips 的 Go 绑定需要较新版本）"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)  GO_ARCH="amd64" ;;
    aarch64|arm64) GO_ARCH="arm64" ;;
    *) echo "不支持的架构: ${ARCH}"; exit 1 ;;
esac
TARBALL="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
if [ ! -x "/usr/local/go/bin/go" ] || ! /usr/local/go/bin/go version | grep -q "go${GO_VERSION}"; then
    # 优先国内镜像，失败回退官方源
    for base in "https://mirrors.aliyun.com/golang" "https://go.dev/dl"; do
        if wget -q "${base}/${TARBALL}" -O "/tmp/${TARBALL}"; then
            break
        fi
    done
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${TARBALL}"
    rm -f "/tmp/${TARBALL}"
fi
export PATH="/usr/local/go/bin:${PATH}"
echo "    Go 版本: $(go version)"

echo "==> [3/5] 编译后端（本地编译，自动适配当前架构）"
cd "${SCRIPT_DIR}/backend"
CGO_ENABLED=1 go build -o "${APP_NAME}" .

echo "==> [4/5] 准备前端静态文件"
cd "${SCRIPT_DIR}"
if [ ! -d "frontend/dist" ]; then
    if command -v npm >/dev/null 2>&1; then
        echo "    未检测到 frontend/dist，尝试在飞牛上构建前端..."
        (cd frontend && npm install && npm run build)
    else
        echo "    [警告] 未安装 npm，跳过前端构建。"
        echo "    请在本机执行 npm run build 后将 frontend/dist 目录上传到飞牛，再重新运行本脚本。"
    fi
fi

echo "    安装到 ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"
install -m 755 "backend/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"
rm -rf "${INSTALL_DIR}/dist"
if [ -d "frontend/dist" ]; then
    cp -r frontend/dist "${INSTALL_DIR}/dist"
fi

echo "==> [5/5] 配置并启动 systemd 服务"
cat > "/etc/systemd/system/${APP_NAME}.service" <<EOF
[Unit]
Description=Image Format Converter (imgconv)
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${APP_NAME}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now "${APP_NAME}"

echo ""
echo "安装完成！"
echo "  查看状态: systemctl status ${APP_NAME}"
echo "  查看日志: journalctl -u ${APP_NAME} -f"
echo "  访问地址: http://<飞牛IP>:8080"
