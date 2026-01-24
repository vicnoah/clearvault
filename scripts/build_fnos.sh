#!/bin/bash
set -e

OUTPUT_DIR="build/fnos"
mkdir -p "${OUTPUT_DIR}"

APPNAME="$(awk -F= '/^appname/ {gsub(/ /,"",$2); print $2}' deploy/fnos/manifest | head -n 1)"
VERSION="$(awk -F= '/^version/ {gsub(/ /,"",$2); print $2}' deploy/fnos/manifest | head -n 1)"
if [ -z "${APPNAME}" ] || [ -z "${VERSION}" ]; then
    echo "Error: failed to read appname/version from deploy/fnos/manifest"
    exit 1
fi

# Common Setup
# Copy default config for fnOS package
echo "Copying config..."
if [ -f config.fnos.example.yaml ]; then
    cp config.fnos.example.yaml deploy/fnos/app/server/config.default.yaml
elif [ -f config.example.yaml ]; then
    cp config.example.yaml deploy/fnos/app/server/config.default.yaml
else
    echo "Warning: config.fnos.example.yaml not found, creating dummy default config"
    touch deploy/fnos/app/server/config.default.yaml
fi

# Copy icons from template if available (and if not already present)
echo "Copying icons..."
TEMPLATE_DIR="/tmp/fnos_template/demo-app"
if [ -d "$TEMPLATE_DIR" ]; then
    [ ! -f deploy/fnos/ICON.PNG ] && cp "$TEMPLATE_DIR/ICON.PNG" deploy/fnos/ICON.PNG
    [ ! -f deploy/fnos/ICON_256.PNG ] && cp "$TEMPLATE_DIR/ICON_256.PNG" deploy/fnos/ICON_256.PNG
    [ ! -f deploy/fnos/app/ui/images/icon_64.png ] && cp "$TEMPLATE_DIR/app/ui/images/icon_64.png" deploy/fnos/app/ui/images/icon_64.png
    [ ! -f deploy/fnos/app/ui/images/icon_256.png ] && cp "$TEMPLATE_DIR/app/ui/images/icon_256.png" deploy/fnos/app/ui/images/icon_256.png
else
    echo "Warning: Template icons not found, using placeholders if needed"
    [ ! -f deploy/fnos/ICON.PNG ] && touch deploy/fnos/ICON.PNG
    [ ! -f deploy/fnos/ICON_256.PNG ] && touch deploy/fnos/ICON_256.PNG
fi

# Make scripts executable
chmod +x deploy/fnos/cmd/*

# Build Function
build_arch() {
    ARCH=$1
    PLATFORM=$2

    echo "========================================"
    echo "Building for $ARCH (Platform: $PLATFORM)..."
    echo "========================================"

    # 1. Compile Binary
    echo "Compiling ClearVault..."
    # Note: Cross-compiling CGO (for FUSE) is tricky.
    # If on x86 host, building for arm64 requires cross-compiler (aarch64-linux-gnu-gcc).
    # We assume the build environment has this if CGO is enabled.
    # If not, we might need to disable CGO for ARM or use a docker builder.
    # For now, let's try standard go build.

    if [ "$ARCH" == "amd64" ]; then
        CC=gcc CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags fuse -o deploy/fnos/app/server/clearvault ./cmd/clearvault
    elif [ "$ARCH" == "arm64" ]; then
        # Assuming cross-compiler is available as aarch64-linux-gnu-gcc
        if command -v aarch64-linux-gnu-gcc &> /dev/null; then
            CC=aarch64-linux-gnu-gcc CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -tags fuse -o deploy/fnos/app/server/clearvault ./cmd/clearvault
        else
            echo "Warning: aarch64-linux-gnu-gcc not found. Building ARM64 without CGO (No FUSE support)."
            CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o deploy/fnos/app/server/clearvault ./cmd/clearvault
        fi
    fi

    # Ensure binary is executable (critical for fnOS execution)
    chmod +x deploy/fnos/app/server/clearvault

    # Ensure default config is present
    if [ ! -f deploy/fnos/app/server/config.default.yaml ]; then
        echo "Error: config.default.yaml missing!"
        exit 1
    fi

    # 2. Update Manifest
    # Use a temporary manifest file
    cp deploy/fnos/manifest deploy/fnos/manifest.tmp
    echo "platform              = $PLATFORM" >> deploy/fnos/manifest

    # 3. Pack
    echo "Packing FPK..."
    # Ensure no stale fpk exists
    rm -f *.fpk

    # fnpack build generates file in current dir by default, usually named appname_version_arch.fpk
    fnpack build -d deploy/fnos

    # Move FPK to output dir with explicit name to avoid overwrites
    # We find the generated fpk and rename it to include our explicit arch name if needed,
    # but fnpack usually names it App.Native.Appname_Version_Arch.fpk.
    # However, if fnpack doesn't respect platform override in filename, we force it.

    GENERATED_FPK=$(ls *.fpk | head -n 1)
    if [ -n "$GENERATED_FPK" ]; then
        TARGET_NAME="${APPNAME}_${VERSION}_${PLATFORM}.fpk"
        mv "$GENERATED_FPK" "${OUTPUT_DIR}/${TARGET_NAME}"
        echo "Created package: ${OUTPUT_DIR}/${TARGET_NAME}"
    else
        echo "Error: FPK generation failed or file not found"
        exit 1
    fi

    # 4. Restore Manifest
    mv deploy/fnos/manifest.tmp deploy/fnos/manifest
}

# Build ARM (Build first so x86 binary remains for CI verification)
build_arch "arm64" "arm"

# Build x86
build_arch "amd64" "x86"

echo "========================================"
echo "Build Complete! Packages are in $OUTPUT_DIR"
echo "========================================"
ls -lh $OUTPUT_DIR
