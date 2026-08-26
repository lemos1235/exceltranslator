#!/bin/bash

NAME="Excel Translator"
BINDIR=bin
GOFILES=cmd/qt/*.go

APP="out/macos/$NAME.app"
ICON=icon.icns
QT=/opt/homebrew/opt/qt

# 递增版本号末位发布号（1.0.0+1 -> 1.0.0+2）
source "$(dirname "$0")/version.sh"
bump_version

# Qt / miqt 需要 C++17
export CGO_CXXFLAGS="${CGO_CXXFLAGS:--std=c++17}"
export CXXFLAGS="${CXXFLAGS:--std=c++17}"
export PATH="$QT/bin:${PATH}"
export PKG_CONFIG_PATH="$QT/lib/pkgconfig:${PKG_CONFIG_PATH:-}"

# 编译 Go 二进制
go build -ldflags "-s -w $(version_ldflags)" -o "$BINDIR/$NAME" $GOFILES

# 创建 .app 结构
mkdir -p "$APP/Contents/"{MacOS,Resources,Frameworks,PlugIns}

cp "$BINDIR/$NAME" "$APP/Contents/MacOS/"

# 添加 Info.plist
cat <<EOF > "$APP/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>$NAME</string>
    <key>CFBundleName</key>
    <string>$NAME</string>
    <key>CFBundleShortVersionString</key>
    <string>$SHORT_VERSION</string>
    <key>CFBundleVersion</key>
    <string>$BUILD_NUMBER</string>
    <key>CFBundleIconFile</key>
    <string>$ICON</string>
    <key>CFBundleIdentifier</key>
    <string>localhost.command_line_arguments</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
</dict>
</plist>
EOF

# 添加图标
cp $ICON "$APP/Contents/Resources/$ICON"

# 签名
codesign --deep --force --sign - "$APP"
