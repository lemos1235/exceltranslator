#!/bin/bash

# =============================================================================
# Windows 安装程序构建脚本
# 使用 NSIS 将 out/windows 目录打包成安装程序
# =============================================================================

set -e

NAME="Excel Translator"
SOURCE_DIR="out/windows"
OUTPUT_FILE="out/${NAME} Setup.exe"
NSI_SCRIPT="installer.nsi"

echo "=== 构建 Windows 安装程序: $NAME ==="

# =============================================================================
# 环境检测
# =============================================================================

# 检查 MSYS2 环境
if [ -z "$MSYSTEM" ]; then
    echo "警告: 未检测到 MSYSTEM 环境变量"
    echo "请在 MSYS2 MinGW64 或 UCRT64 终端中运行此脚本"
    exit 1
fi

# 检查 makensis 是否已安装
if ! command -v makensis &> /dev/null; then
    echo "错误: 未找到 makensis 命令"
    echo ""
    echo "请安装 NSIS:"
    echo "  pacman -S mingw-w64-x86_64-nsis"
    echo ""
    echo "或者 UCRT64 环境:"
    echo "  pacman -S mingw-w64-ucrt-x86_64-nsis"
    exit 1
fi

# 检查 NSIS 脚本是否存在
if [ ! -f "$NSI_SCRIPT" ]; then
    echo "错误: 未找到 NSIS 脚本: $NSI_SCRIPT"
    exit 1
fi

# =============================================================================
# 构建 Windows 应用程序
# =============================================================================

BUILD_SCRIPT="build_winapp_embedded.sh"

if [ ! -f "$BUILD_SCRIPT" ]; then
    echo "错误: 未找到构建脚本: $BUILD_SCRIPT"
    exit 1
fi

echo ""
echo ">>> 步骤 1: 构建 Windows 应用程序..."
echo ""

# 运行构建脚本
if ! ./"$BUILD_SCRIPT"; then
    echo ""
    echo "错误: Windows 应用程序构建失败"
    exit 1
fi

echo ""
echo ">>> 步骤 2: 打包安装程序..."
echo ""

# 检查源目录是否存在
if [ ! -d "$SOURCE_DIR" ]; then
    echo "错误: 源目录不存在: $SOURCE_DIR"
    exit 1
fi

# 检查主程序是否存在
if [ ! -f "$SOURCE_DIR/$NAME.exe" ]; then
    echo "错误: 主程序不存在: $SOURCE_DIR/$NAME.exe"
    exit 1
fi

# =============================================================================
# 构建安装程序
# =============================================================================

echo ">>> 源目录: $SOURCE_DIR"
echo ">>> NSIS 脚本: $NSI_SCRIPT"
echo ""

echo ">>> 编译安装程序..."
# 读取 build_winapp_embedded.sh 递增后的版本号，传给 NSIS
source "$(dirname "$0")/version.sh"
read_version
echo ">>> 安装程序版本: $SHORT_VERSION.$BUILD_NUMBER"

# 使用 MSYS_NO_PATHCONV 防止 Git Bash 转换路径
MSYS_NO_PATHCONV=1 makensis /INPUTCHARSET UTF8 -DAPP_VERSION="$SHORT_VERSION.$BUILD_NUMBER" "$NSI_SCRIPT"

# =============================================================================
# 完成
# =============================================================================

if [ -f "$OUTPUT_FILE" ]; then
    echo ""
    echo "=== 构建完成 ==="
    echo "安装程序: $OUTPUT_FILE"
    
    # 显示文件大小
    FILE_SIZE=$(du -h "$OUTPUT_FILE" | cut -f1)
    echo "文件大小: $FILE_SIZE"
    
    # 清理和重组目录
    echo ""
    echo ">>> 清理源目录..."
    rm -rf "${SOURCE_DIR:?}"/*
    
    echo ">>> 移动安装程序到 windows 目录..."
    mv "$OUTPUT_FILE" "$SOURCE_DIR/"
    
    echo ""
    echo "=== 最终输出 ==="
    echo "安装程序已移动到: $SOURCE_DIR/$(basename "$OUTPUT_FILE")"
    echo "文件大小: $FILE_SIZE"
else
    echo ""
    echo "错误: 安装程序生成失败"
    exit 1
fi
