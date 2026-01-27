#!/bin/bash

NAME="Excel Translator"
BINDIR=bin
OUTDIR=out/windows
SRCDIR=cmd/qt

ICON=icon.ico

# 设置 Windows 交叉编译环境变量
export GOOS=windows
export GOARCH=amd64
export CGO_ENABLED=1

echo "=== 构建 Windows 应用: $NAME ==="

# 创建输出目录
mkdir -p "$OUTDIR"
mkdir -p "$BINDIR"

# 检查并安装 rsrc 工具（用于嵌入图标）
if ! command -v rsrc &> /dev/null; then
    echo ">>> 安装 rsrc 工具..."
    GOOS="" GOARCH="" go install github.com/akavel/rsrc@latest
fi

# 使用 rsrc 将图标嵌入到 .syso 文件
if [ -f "$ICON" ]; then
    echo ">>> 嵌入图标到可执行文件..."
    rsrc -ico "$ICON" -o "$SRCDIR/rsrc_windows_amd64.syso"
    if [ $? -ne 0 ]; then
        echo "警告: 图标嵌入失败，继续编译..."
    fi
fi

# 编译 Go 二进制（Windows .exe）
echo ">>> 编译 Go 二进制..."
go build -ldflags "-s -w -H windowsgui" -o "$BINDIR/$NAME.exe" ./$SRCDIR

BUILD_RESULT=$?

# 清理 .syso 文件
rm -f "$SRCDIR/rsrc_windows_amd64.syso"

if [ $BUILD_RESULT -ne 0 ]; then
    echo "错误: 编译失败"
    exit 1
fi

echo ">>> 复制文件到输出目录..."
# 复制可执行文件
cp "$BINDIR/$NAME.exe" "$OUTDIR/"

echo "=== 构建完成 ==="
echo "输出目录: $OUTDIR"
echo "可执行文件: $OUTDIR/$NAME.exe"

# 显示文件大小
ls -lh "$OUTDIR/$NAME.exe"
