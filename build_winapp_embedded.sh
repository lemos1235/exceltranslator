#!/bin/bash

# =============================================================================
# MSYS2 环境下的 Windows QT 内嵌构建脚本
# 此脚本在 MSYS2 (mingw64/ucrt64) 环境中运行，编译并打包 QT 应用
# =============================================================================

set -e

NAME="Excel Translator"
BINDIR=bin
OUTDIR=out/windows
SRCDIR=cmd/qt
ICON=icon.ico

# =============================================================================
# 环境检测
# =============================================================================

# 检测 MSYS2 环境
if [ -z "$MSYSTEM" ]; then
    echo "警告: 未检测到 MSYSTEM 环境变量"
    echo "请在 MSYS2 MinGW64 或 UCRT64 终端中运行此脚本"
    exit 1
fi

# 检测 QT 安装路径 - 支持多种安装方式
# 优先级：
# 1. 环境变量 QT_DIR（如果设置）
# 2. MSYS2 默认安装: $MINGW_PREFIX/share/qt6/plugins
# 3. Qt 官方安装: /c/Qt/6.10.1/mingw_64/plugins
# 4. 其他自定义路径

QT_PLUGIN_PATH=""

# 1. 检查是否设置了 QT_DIR 环境变量
if [ -n "$QT_DIR" ] && [ -d "$QT_DIR/plugins" ]; then
    QT_PLUGIN_PATH="$QT_DIR/plugins"
    MINGW_PREFIX="$QT_DIR"
# 2. 检查 MSYS2 默认安装
elif [ -n "$MINGW_PREFIX" ] && [ -d "$MINGW_PREFIX/share/qt6/plugins" ]; then
    QT_PLUGIN_PATH="$MINGW_PREFIX/share/qt6/plugins"
# 3. 检查 Qt 官方安装（常见路径）
elif [ -d "/c/Qt/6.10.1/mingw_64/plugins" ]; then
    MINGW_PREFIX="/c/Qt/6.10.1/mingw_64"
    QT_PLUGIN_PATH="$MINGW_PREFIX/plugins"
elif [ -d "C:/Qt/6.10.1/mingw_64/plugins" ]; then
    MINGW_PREFIX="C:/Qt/6.10.1/mingw_64"
    QT_PLUGIN_PATH="$MINGW_PREFIX/plugins"
fi

# 检查是否找到 Qt 安装
if [ -z "$QT_PLUGIN_PATH" ] || [ ! -d "$QT_PLUGIN_PATH" ]; then
    echo "错误: 未找到 QT6 安装"
    echo ""
    echo "已检查的路径："
    echo "  - \$QT_DIR/plugins (环境变量)"
    echo "  - $MINGW_PREFIX/share/qt6/plugins (MSYS2)"
    echo "  - /c/Qt/6.10.1/mingw_64/plugins (Qt 官方)"
    echo ""
    echo "解决方法："
    echo "  1. 安装 MSYS2 Qt: pacman -S mingw-w64-x86_64-qt6-base"
    echo "  2. 或设置环境变量: export QT_DIR=/c/Qt/6.10.1/mingw_64"
    exit 1
fi

echo "=== 构建 Windows 应用（内嵌QT）: $NAME ==="
echo ">>> MSYS2 环境: $MSYSTEM"
echo ">>> MINGW 前缀: $MINGW_PREFIX"

# =============================================================================
# 准备目录
# =============================================================================

mkdir -p "$OUTDIR"
mkdir -p "$BINDIR"

# =============================================================================
# 嵌入图标
# =============================================================================

# 检查并安装 rsrc 工具（用于嵌入图标）
if ! command -v rsrc &> /dev/null; then
    echo ">>> 安装 rsrc 工具..."
    go install github.com/akavel/rsrc@latest
fi

# 使用 rsrc 将图标嵌入到 .syso 文件
if [ -f "$ICON" ]; then
    echo ">>> 嵌入图标到可执行文件..."
    rsrc -ico "$ICON" -o "$SRCDIR/rsrc_windows_amd64.syso"
    if [ $? -ne 0 ]; then
        echo "警告: 图标嵌入失败，继续编译..."
    fi
fi

# =============================================================================
# 编译 Go 二进制
# =============================================================================

echo ">>> 编译 Go 二进制..."
CGO_ENABLED=1 go build -ldflags "-s -w -H windowsgui" -o "$BINDIR/$NAME.exe" ./$SRCDIR

BUILD_RESULT=$?

# 清理 .syso 文件
rm -f "$SRCDIR/rsrc_windows_amd64.syso"

if [ $BUILD_RESULT -ne 0 ]; then
    echo "错误: 编译失败"
    exit 1
fi

# =============================================================================
# 复制可执行文件
# =============================================================================

echo ">>> 复制可执行文件..."
cp "$BINDIR/$NAME.exe" "$OUTDIR/"

# =============================================================================
# 复制 QT DLL
# =============================================================================

copy_dll() {
    local dll_name="$1"
    local dll_path="$MINGW_PREFIX/bin/$dll_name"
    if [ -f "$dll_path" ]; then
        cp "$dll_path" "$OUTDIR/"
        echo "    复制: $dll_name"
    else
        echo "    警告: 未找到 $dll_name"
    fi
}

echo ">>> 复制 QT 核心 DLL..."
copy_dll "Qt6Core.dll"
copy_dll "Qt6Gui.dll"
copy_dll "Qt6Widgets.dll"

# =============================================================================
# 复制 QT 插件
# =============================================================================

copy_plugin() {
    local plugin_subdir="$1"
    local plugin_name="$2"
    local src_path="$QT_PLUGIN_PATH/$plugin_subdir/$plugin_name"
    local dst_dir="$OUTDIR/$plugin_subdir"
    
    if [ -f "$src_path" ]; then
        mkdir -p "$dst_dir"
        cp "$src_path" "$dst_dir/"
        echo "    复制: $plugin_subdir/$plugin_name"
    else
        echo "    警告: 未找到插件 $plugin_subdir/$plugin_name"
    fi
}

echo ">>> 复制 QT 插件..."
# 平台插件（必需）
copy_plugin "platforms" "qwindows.dll"

# 样式插件（可选，Fusion样式是内置的）
copy_plugin "styles" "qwindowsvistastyle.dll"
copy_plugin "styles" "qmodernwindowsstyle.dll"

# 图标引擎插件（用于图标显示）
copy_plugin "iconengines" "qsvgicon.dll"

# 图片格式插件（用于加载图片）
copy_plugin "imageformats" "qico.dll"
copy_plugin "imageformats" "qjpeg.dll"
copy_plugin "imageformats" "qpng.dll"
copy_plugin "imageformats" "qsvg.dll"

# =============================================================================
# 复制 MinGW 运行时 DLL
# =============================================================================

echo ">>> 复制 MinGW 运行时..."
copy_dll "libgcc_s_seh-1.dll"
copy_dll "libstdc++-6.dll"
copy_dll "libwinpthread-1.dll"

# 额外的依赖 DLL（QT 可能需要）
echo ">>> 复制额外依赖..."
# 压缩和编码库
copy_dll "zlib1.dll"
copy_dll "libbz2-1.dll"
copy_dll "libbrotlidec.dll"
copy_dll "libbrotlicommon.dll"
copy_dll "libzstd.dll"

# 图片格式支持
copy_dll "libpng16-16.dll"
copy_dll "libjpeg-8.dll"

# SVG 支持
copy_dll "Qt6Svg.dll"

# 字体和文本渲染
copy_dll "libharfbuzz-0.dll"
copy_dll "libfreetype-6.dll"

# 国际化和字符编码
copy_dll "libglib-2.0-0.dll"
copy_dll "libintl-8.dll"
copy_dll "libiconv-2.dll"
copy_dll "libpcre2-8-0.dll"
copy_dll "libicuin76.dll"
copy_dll "libicuuc76.dll"
copy_dll "libicudt76.dll"

# 其他
copy_dll "libdouble-conversion.dll"
copy_dll "libmd4c.dll"

# =============================================================================
# 创建 qt.conf 配置文件
# =============================================================================

echo ">>> 创建 qt.conf 配置文件..."
cat > "$OUTDIR/qt.conf" <<EOF
[Paths]
Plugins = .
EOF

echo "    创建: qt.conf"

# =============================================================================
# 完成
# =============================================================================

echo ""
echo "=== 构建完成 ==="
echo "输出目录: $OUTDIR"
echo ""
echo "目录内容:"
ls -la "$OUTDIR/"
echo ""
echo "插件目录:"
ls -la "$OUTDIR/platforms/" 2>/dev/null || echo "  (无 platforms 目录)"
ls -la "$OUTDIR/styles/" 2>/dev/null || echo "  (无 styles 目录)"
echo ""

# 计算总大小
TOTAL_SIZE=$(du -sh "$OUTDIR" | cut -f1)
echo "总大小: $TOTAL_SIZE"
