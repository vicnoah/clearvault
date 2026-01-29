#!/bin/bash
set -e

# zs3 安装脚本
# 用于自动化编译和安装 zs3 作为测试依赖

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TOOLS_DIR="${PROJECT_ROOT}/tools"
ZS3_DIR="${TOOLS_DIR}/zs3-src"
ZS3_BINARY="${TOOLS_DIR}/zs3"

# zs3 仓库地址
ZS3_REPO="https://github.com/Lulzx/zs3.git"

# 检查是否已安装 zig
if ! command -v zig &> /dev/null; then
    echo "Error: zig 未安装，请先安装 zig 0.15+"
    echo "安装指南: https://ziglang.org/learn/getting-started/"
    exit 1
fi

# 检查 zig 版本
ZIG_VERSION=$(zig version | cut -d'.' -f1,2)
REQUIRED_VERSION="0.15"
if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$ZIG_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    echo "Error: zig 版本需要 >= 0.15，当前版本: $(zig version)"
    exit 1
fi

echo "========================================"
echo "安装 zs3 作为测试依赖"
echo "========================================"

# 创建 tools 目录
mkdir -p "${TOOLS_DIR}"

# 如果已存在二进制文件，先删除
if [ -f "${ZS3_BINARY}" ]; then
    echo "删除已存在的 zs3 二进制文件..."
    rm -f "${ZS3_BINARY}"
fi

# 克隆或更新源码
if [ -d "${ZS3_DIR}" ]; then
    echo "更新 zs3 源码..."
    cd "${ZS3_DIR}"
    git pull origin main
else
    echo "克隆 zs3 仓库..."
    git clone "${ZS3_REPO}" "${ZS3_DIR}"
    cd "${ZS3_DIR}"
fi

# 编译 zs3
echo "编译 zs3 (ReleaseFast 模式)..."
zig build -Doptimize=ReleaseFast

# 复制二进制文件到 tools 目录
echo "安装 zs3 到 ${TOOLS_DIR}..."
cp "${ZS3_DIR}/zig-out/bin/zs3" "${ZS3_BINARY}"
chmod +x "${ZS3_BINARY}"

# 验证安装
echo "验证安装..."
if "${ZS3_BINARY}" --help &> /dev/null || true; then
    echo "zs3 安装成功!"
    echo "二进制文件位置: ${ZS3_BINARY}"
    echo "文件大小: $(du -h "${ZS3_BINARY}" | cut -f1)"
else
    echo "警告: 无法验证 zs3 是否正常工作"
fi

echo "========================================"
echo "zs3 安装完成"
echo "========================================"
echo ""
echo "使用方式:"
echo "  ${ZS3_BINARY}"
echo ""
echo "或进入目录后运行:"
echo "  cd ${TOOLS_DIR} && ./zs3"
