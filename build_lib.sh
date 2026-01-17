#!/bin/bash

# 设置输出目录
OUT_DIR="out/ffi/lib"
mkdir -p "$OUT_DIR"

LIB_NAME="exceltranslator"

echo "Building C-Shared library..."

# 根据操作系统检测平台
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$OS" in
    darwin*)
        echo "Detected macOS, building .dylib..."
        go build -buildmode=c-shared -o "$OUT_DIR/lib$LIB_NAME.dylib" -ldflags="-s -w" cmd/lib/main.go
        ;;
    linux*)
        echo "Detected Linux, building .so..."
        go build -buildmode=c-shared -o "$OUT_DIR/lib$LIB_NAME.so" -ldflags="-s -w" cmd/lib/main.go
        ;;
    msys*|cygwin*|mingw*)
        echo "Detected Windows-like environment, building .dll..."
        go build -buildmode=c-shared -o "$OUT_DIR/$LIB_NAME.dll" -ldflags="-s -w" cmd/lib/main.go
        ;;
    *)
        echo "Unknown OS: $OS, attempting default .so build..."
        go build -buildmode=c-shared -o "$OUT_DIR/lib$LIB_NAME.so" -ldflags="-s -w" cmd/lib/main.go
        ;;
esac

if [ $? -eq 0 ]; then
    echo "Successfully built library in $OUT_DIR"
    ls -lh "$OUT_DIR"
else
    echo "Build failed!"
    exit 1
fi
