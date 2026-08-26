#!/bin/bash
# =============================================================================
# 版本号工具（供各构建脚本 source 使用）
#
# 版本号维护在仓库根目录的 VERSION 文件中，格式为 <语义化版本>+<发布号>，
# 例如 1.0.0+2。构建时通过 ldflags 注入到 exceltranslator/pkg/version.Full。
# =============================================================================

# 兜底 $0：脚本被非 bash 的 shell（如 zsh）source 时 BASH_SOURCE 未定义
VERSION_FILE="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)/VERSION"

# read_version 读取当前版本号并解析到 FULL_VERSION/SHORT_VERSION/BUILD_NUMBER
read_version() {
    if [ ! -f "$VERSION_FILE" ]; then
        echo "1.0.0+1" > "$VERSION_FILE"
    fi
    FULL_VERSION=$(tr -d '[:space:]' < "$VERSION_FILE")
    SHORT_VERSION=${FULL_VERSION%%+*}
    case "$FULL_VERSION" in
        *+*) BUILD_NUMBER=${FULL_VERSION##*+} ;;
        *)   BUILD_NUMBER=1 ;;
    esac
}

# bump_version 递增末位发布号（1.0.0+1 -> 1.0.0+2）并写回 VERSION 文件
bump_version() {
    read_version
    BUILD_NUMBER=$((BUILD_NUMBER + 1))
    FULL_VERSION="$SHORT_VERSION+$BUILD_NUMBER"
    printf '%s\n' "$FULL_VERSION" > "$VERSION_FILE"
    echo ">>> 构建版本: $FULL_VERSION"
}

# version_ldflags 输出注入版本号所需的 ldflags 片段
version_ldflags() {
    printf -- '-X exceltranslator/pkg/version.Full=%s' "$FULL_VERSION"
}

# generate_win_syso 生成 Windows 资源文件（.syso），同时嵌入图标与版本信息
# 使得 exe 右键属性 -> 详细信息 中可以看到版本号
# 用法: generate_win_syso <图标路径> <输出 syso 路径> <应用名>
# 依赖 read_version/bump_version 已执行（需要 SHORT_VERSION/BUILD_NUMBER）
generate_win_syso() {
    local icon="$1" out="$2" app_name="$3"

    # 先删除可能残留的旧资源文件，避免生成失败时把陈旧版本信息链接进去
    rm -f "$out"

    # 安装 goversioninfo（注意清空 GOOS/GOARCH，避免交叉编译环境下装成 Windows 二进制）
    if ! command -v goversioninfo &> /dev/null; then
        echo ">>> 安装 goversioninfo 工具..."
        GOOS="" GOARCH="" go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
    fi
    if ! command -v goversioninfo &> /dev/null; then
        echo "警告: 未找到 goversioninfo，跳过版本资源嵌入"
        return 1
    fi

    # 语义化版本拆成 major.minor.patch，发布号作为第四段
    local major minor patch
    IFS='.' read -r major minor patch <<< "$SHORT_VERSION"
    major=${major:-0}; minor=${minor:-0}; patch=${patch:-0}

    local cfg
    cfg="$(dirname "$out")/versioninfo.json"
    cat > "$cfg" <<JSON
{
  "FixedFileInfo": {
    "FileVersion": {"Major": $major, "Minor": $minor, "Patch": $patch, "Build": $BUILD_NUMBER},
    "ProductVersion": {"Major": $major, "Minor": $minor, "Patch": $patch, "Build": $BUILD_NUMBER},
    "FileFlagsMask": "3f",
    "FileFlags": "00",
    "FileOS": "040004",
    "FileType": "01",
    "FileSubType": "00"
  },
  "StringFileInfo": {
    "CompanyName": "$app_name",
    "FileDescription": "$app_name",
    "FileVersion": "$major.$minor.$patch.$BUILD_NUMBER",
    "InternalName": "$app_name.exe",
    "LegalCopyright": "",
    "OriginalFilename": "$app_name.exe",
    "ProductName": "$app_name",
    "ProductVersion": "$major.$minor.$patch.$BUILD_NUMBER"
  },
  "VarFileInfo": {
    "Translation": {"LangID": "0409", "CharsetID": "04B0"}
  },
  "IconPath": "$icon",
  "ManifestPath": ""
}
JSON

    echo ">>> 嵌入图标与版本信息 ($major.$minor.$patch.$BUILD_NUMBER)..."
    if ! goversioninfo -o "$out" "$cfg"; then
        echo "警告: 版本资源生成失败，继续编译..."
        rm -f "$cfg"
        return 1
    fi
    rm -f "$cfg"
}
