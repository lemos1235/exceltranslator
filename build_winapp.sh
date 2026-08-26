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

# 递增版本号末位发布号（1.0.0+1 -> 1.0.0+2）
source "$(dirname "$0")/version.sh"
bump_version

echo "=== 构建 Windows 应用: $NAME ==="

# 创建输出目录
mkdir -p "$OUTDIR"
mkdir -p "$BINDIR"

# 生成 Windows 资源文件（图标 + 版本信息）
if [ -f "$ICON" ]; then
    generate_win_syso "$ICON" "$SRCDIR/rsrc_windows_amd64.syso" "$NAME" || true
fi

# 编译 Go 二进制（Windows .exe）
echo ">>> 编译 Go 二进制..."
go build -ldflags "-s -w -H windowsgui $(version_ldflags)" -o "$BINDIR/$NAME.exe" ./$SRCDIR

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
