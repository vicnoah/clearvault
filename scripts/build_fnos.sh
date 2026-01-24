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
    echo "Warning: No example config found, creating dummy"
    touch deploy/fnos/app/server/config.default.yaml
fi

echo "Copying icons..."
TEMPLATE_DIR="/tmp/fnos_template/demo-app"
if [ -d "$TEMPLATE_DIR" ]; then
    [ ! -f deploy/fnos/ICON.PNG ] && cp "$TEMPLATE_DIR/ICON.PNG" deploy/fnos/ICON.PNG
    [ ! -f deploy/fnos/ICON_256.PNG ] && cp "$TEMPLATE_DIR/ICON_256.PNG" deploy/fnos/ICON_256.PNG
else
    [ ! -f deploy/fnos/ICON.PNG ] && touch deploy/fnos/ICON.PNG
    [ ! -f deploy/fnos/ICON_256.PNG ] && touch deploy/fnos/ICON_256.PNG
fi

chmod +x deploy/fnos/cmd/* || true

# --- 3. 核心构建函数 ---
build_arch() {
    ARCH=$1
    PLATFORM=$2

    echo "========================================"
    echo "Building for $ARCH (Platform: $PLATFORM)..."
    echo "========================================"

    export CGO_ENABLED=1
    export GOOS=linux
    export GOARCH=$ARCH
    export PKG_CONFIG_ALLOW_CROSS=1

    unset CC
    unset PKG_CONFIG_PATH
    unset CGO_CFLAGS
    unset CGO_LDFLAGS

    if [ "$ARCH" == "amd64" ]; then
        export CC=gcc
    elif [ "$ARCH" == "arm64" ]; then
        if command -v aarch64-linux-gnu-gcc &> /dev/null; then
            export CC=aarch64-linux-gnu-gcc
            # 交叉编译必须指定 pkg-config 寻找路径，否则找不到 arm64 的 libfuse3.so
            export PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig
        else
            echo "Error: aarch64-linux-gnu-gcc not found!"
            exit 1
        fi
    fi

    echo "Compiling binary..."
    # 关键点：使用 -tags "fuse fuse3" 来触发 cgofuse 源码中的 FUSE3 宏定义
    go build -tags "fuse fuse3" -o deploy/fnos/app/server/clearvault ./cmd/clearvault
    chmod +x deploy/fnos/app/server/clearvault

    # --- 4. 打包 ---
    echo "Updating manifest platform to $PLATFORM..."
    cp deploy/fnos/manifest deploy/fnos/manifest.tmp
    sed -i "/^platform/d" deploy/fnos/manifest
    echo "platform              = $PLATFORM" >> deploy/fnos/manifest

    echo "Packing FPK..."
    rm -f *.fpk
    fnpack build -d deploy/fnos

    GENERATED_FPK=$(ls *.fpk | head -n 1)
    if [ -n "$GENERATED_FPK" ]; then
        TARGET_NAME="${APPNAME}_${VERSION}_${PLATFORM}.fpk"
        mv "$GENERATED_FPK" "${OUTPUT_DIR}/${TARGET_NAME}"
        echo "Created: ${TARGET_NAME}"
    else
        exit 1
    fi
    mv deploy/fnos/manifest.tmp deploy/fnos/manifest
}

# --- 5. 执行 ---
build_arch "arm64" "arm"
build_arch "amd64" "x86"

echo "Done! Files in $OUTPUT_DIR"
