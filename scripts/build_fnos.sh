#!/bin/bash
set -e

OUTPUT_DIR="build/fnos"
mkdir -p "${OUTPUT_DIR}"

# --- 1. 读取基础信息 ---
APPNAME="$(awk -F= '/^appname/ {gsub(/ /,"",$2); print $2}' deploy/fnos/manifest | head -n 1)"
VERSION="$(awk -F= '/^version/ {gsub(/ /,"",$2); print $2}' deploy/fnos/manifest | head -n 1)"

if [ -z "${APPNAME}" ] || [ -z "${VERSION}" ]; then
    echo "Error: failed to read appname/version from deploy/fnos/manifest"
    exit 1
fi

# --- 2. 通用准备工作 ---
echo "Copying config..."
if [ -f config.fnos.example.yaml ]; then
    cp config.fnos.example.yaml deploy/fnos/app/server/config.default.yaml
elif [ -f config.example.yaml ]; then
    cp config.example.yaml deploy/fnos/app/server/config.default.yaml
else
    echo "Warning: No example config found, creating empty default"
    touch deploy/fnos/app/server/config.default.yaml
fi

echo "Copying icons..."
TEMPLATE_DIR="/tmp/fnos_template/demo-app"
if [ -d "$TEMPLATE_DIR" ]; then
    [ ! -f deploy/fnos/ICON.PNG ] && cp "$TEMPLATE_DIR/ICON.PNG" deploy/fnos/ICON.PNG
    [ ! -f deploy/fnos/ICON_256.PNG ] && cp "$TEMPLATE_DIR/ICON_256.PNG" deploy/fnos/ICON_256.PNG
else
    echo "Warning: Template icons not found, ensuring placeholders exist"
    [ ! -f deploy/fnos/ICON.PNG ] && touch deploy/fnos/ICON.PNG
    [ ! -f deploy/fnos/ICON_256.PNG ] && touch deploy/fnos/ICON_256.PNG
fi

chmod +x deploy/fnos/cmd/* || true

# --- 3. 核心构建函数 ---
build_arch() {
    ARCH=$1      # go arch: amd64, arm64
    PLATFORM=$2  # fnos manifest platform: x86, arm

    echo "========================================"
    echo "Building for $ARCH (Platform: $PLATFORM)..."
    echo "========================================"

    # 初始化编译环境变量
    export CGO_ENABLED=1
    export GOOS=linux
    export GOARCH=$ARCH

    # 清理旧的环境变量，防止交叉影响
    unset CC
    unset PKG_CONFIG_PATH
    unset PKG_CONFIG_LIBDIR
    export PKG_CONFIG_ALLOW_CROSS=1

    if [ "$ARCH" == "amd64" ]; then
        # 本地编译环境
        export CC=gcc
        # 默认使用系统的 pkg-config 路径
    elif [ "$ARCH" == "arm64" ]; then
        # 交叉编译环境
        if command -v aarch64-linux-gnu-gcc &> /dev/null; then
            export CC=aarch64-linux-gnu-gcc
            # 关键：指定 pkg-config 寻找 arm64 架构的库文件 (.pc文件)
            export PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig
        else
            echo "Error: aarch64-linux-gnu-gcc not found! Cannot build ARM64 with CGO."
            exit 1
        fi
    fi

    echo "Compiling binary with CGO and FUSE tags..."
    go build -tags fuse -o deploy/fnos/app/server/clearvault ./cmd/clearvault
    chmod +x deploy/fnos/app/server/clearvault

    # --- 4. 打包 FPK ---
    echo "Updating manifest platform to $PLATFORM..."
    cp deploy/fnos/manifest deploy/fnos/manifest.tmp
    # 确保 manifest 中有正确的 platform
    sed -i "/^platform/d" deploy/fnos/manifest
    echo "platform              = $PLATFORM" >> deploy/fnos/manifest

    echo "Packing FPK for $PLATFORM..."
    rm -f *.fpk
    fnpack build -d deploy/fnos

    GENERATED_FPK=$(ls *.fpk | head -n 1)
    if [ -n "$GENERATED_FPK" ]; then
        TARGET_NAME="${APPNAME}_${VERSION}_${PLATFORM}.fpk"
        mv "$GENERATED_FPK" "${OUTPUT_DIR}/${TARGET_NAME}"
        echo "Successfully created: ${OUTPUT_DIR}/${TARGET_NAME}"
    else
        echo "Error: FPK generation failed"
        exit 1
    fi

    # 还原 manifest 供下次循环使用
    mv deploy/fnos/manifest.tmp deploy/fnos/manifest
}

# --- 5. 执行构建 ---
# 先跑 arm64，再跑 amd64
build_arch "arm64" "arm"
build_arch "amd64" "x86"

echo "========================================"
echo "All builds completed! Files in $OUTPUT_DIR"
echo "========================================"
ls -lh $OUTPUT_DIR
