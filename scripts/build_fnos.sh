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

    # 初始化基础编译环境变量
    export CGO_ENABLED=1
    export GOOS=linux
    export GOARCH=$ARCH

    # 强制开启交叉编译支持并清理旧变量
    export PKG_CONFIG_ALLOW_CROSS=1
    unset CC
    unset PKG_CONFIG_PATH
    unset CGO_CFLAGS
    unset CGO_LDFLAGS

    # 检查头文件位置（调试用）
    if [ -d "/usr/include/fuse3" ]; then
        echo "Found FUSE3 headers at /usr/include/fuse3"
    fi

    if [ "$ARCH" == "amd64" ]; then
        # x86_64 编译配置
        export CC=gcc
        # 显式指向头文件路径以防 pkg-config 失效
        export CGO_CFLAGS="-I/usr/include/fuse3"
        export CGO_LDFLAGS="-lfuse3"

    elif [ "$ARCH" == "arm64" ]; then
        # ARM64 交叉编译配置
        if command -v aarch64-linux-gnu-gcc &> /dev/null; then
            export CC=aarch64-linux-gnu-gcc
            # 关键：1. 指定 arm64 的库搜索路径
            export PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig
            # 关键：2. 显式包含 FUSE3 头文件路径
            export CGO_CFLAGS="-I/usr/include/fuse3"
            # 关键：3. 指定库文件路径并链接
            export CGO_LDFLAGS="-L/usr/lib/aarch64-linux-gnu -lfuse3"
        else
            echo "Error: aarch64-linux-gnu-gcc not found! Cannot build ARM64 with CGO."
            exit 1
        fi
    fi

    echo "Compiling binary with CGO and FUSE tags..."
    # 使用 -v 可以在日志中看到更多 Go 编译详情
    go build -tags fuse -o deploy/fnos/app/server/clearvault ./cmd/clearvault
    chmod +x deploy/fnos/app/server/clearvault

    # --- 4. 更新 Manifest 并打包 ---
    echo "Updating manifest platform to $PLATFORM..."
    cp deploy/fnos/manifest deploy/fnos/manifest.tmp
    # 删除旧的 platform 行并追加新的
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

    # 还原原始 manifest
    mv deploy/fnos/manifest.tmp deploy/fnos/manifest
}

# --- 5. 执行构建循环 ---
# 先编译 ARM64（通常交叉编译更容易报错，先测试）
build_arch "arm64" "arm"

# 再编译 AMD64
build_arch "amd64" "x86"

echo "========================================"
echo "Build Complete!"
echo "Packages located in: $OUTPUT_DIR"
echo "========================================"
ls -lh $OUTPUT_DIR
