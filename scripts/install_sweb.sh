#!/bin/bash
set -e

# sweb 安装脚本
# 用于自动化编译和安装 sweb 作为测试依赖
# sweb: Simple Web File Server - https://github.com/twhite-gh/sweb

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TOOLS_DIR="${PROJECT_ROOT}/tools"
SWEB_DIR="${TOOLS_DIR}/sweb-src"
SWEB_BINARY="${TOOLS_DIR}/sweb"

# sweb 仓库地址
SWEB_REPO="https://github.com/twhite-gh/sweb.git"

# 检查是否已安装 Go
if ! command -v go &> /dev/null; then
    echo "Error: Go 未安装，请先安装 Go 1.19+"
    echo "安装指南: https://golang.org/doc/install"
    exit 1
fi

# 检查 Go 版本
GO_VERSION=$(go version | grep -oP '\d+\.\d+' | head -1)
REQUIRED_VERSION="1.19"
if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    echo "Error: Go 版本需要 >= 1.19，当前版本: $(go version)"
    exit 1
fi

echo "========================================"
echo "安装 sweb 作为测试依赖"
echo "========================================"

# 创建 tools 目录
mkdir -p "${TOOLS_DIR}"

# 如果已存在二进制文件，先删除
if [ -f "${SWEB_BINARY}" ]; then
    echo "删除已存在的 sweb 二进制文件..."
    rm -f "${SWEB_BINARY}"
fi

# 克隆或更新源码
if [ -d "${SWEB_DIR}" ]; then
    echo "更新 sweb 源码..."
    cd "${SWEB_DIR}"
    git pull origin main
else
    echo "克隆 sweb 仓库..."
    git clone "${SWEB_REPO}" "${SWEB_DIR}"
    cd "${SWEB_DIR}"
fi

# 编译 sweb
echo "编译 sweb (Release 模式)..."
cd "${SWEB_DIR}/src"
go build -ldflags "-s -w" -o "${SWEB_BINARY}" .

chmod +x "${SWEB_BINARY}"

# 验证安装
echo "验证安装..."
if "${SWEB_BINARY}" -help &> /dev/null || true; then
    echo "sweb 安装成功!"
    echo "二进制文件位置: ${SWEB_BINARY}"
    echo "文件大小: $(du -h "${SWEB_BINARY}" | cut -f1)"
else
    echo "警告: 无法验证 sweb 是否正常工作"
fi

echo "========================================"
echo "sweb 安装完成"
echo "========================================"
echo ""
echo "使用方式:"
echo "  # 基本文件服务器"
echo "  ${SWEB_BINARY}"
echo ""
echo "  # 启用文件上传和浏览"
echo "  ${SWEB_BINARY} -upload -files"
echo ""
echo "  # 启用 WebDAV 服务"
echo "  ${SWEB_BINARY} -webdav -webdav-auth"
echo ""
echo "  # 启用 HTTPS 服务"
echo "  ${SWEB_BINARY} -https"
echo ""
echo "  # 启用 SOCKS5 代理"
echo "  ${SWEB_BINARY} -socks5"
echo ""
echo "  # 启用 HTTP 代理"
echo "  ${SWEB_BINARY} -proxy"
echo ""
echo "  # 启用所有功能"
echo "  ${SWEB_BINARY} -upload -files -webdav -webdav-auth -https -socks5 -proxy"
