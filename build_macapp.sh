NAME="Excel Translator"
BINDIR=bin
GOFILES=cmd/qt/*.go

APP="out/$NAME.app"
ICON=icon.icns
QT=/opt/homebrew/opt/qt

# 编译 Go 二进制
go build -ldflags "-s -w" -o "$BINDIR/$NAME" $GOFILES

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
