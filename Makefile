NAME=Excel-Translator
BINDIR=bin
GOFILES=cmd/qt/*.go
# 版本号取自 VERSION 文件（此处只读取，递增由各 build_*.sh 完成）
FULL_VERSION=$(shell test -f VERSION && tr -d '[:space:]' < VERSION || echo dev)
LDFLAGS_VERSION=-X exceltranslator/pkg/version.Full=$(FULL_VERSION)

darwin-arm64:
	GOARCH=arm64 GOOS=darwin go build -ldflags '-s -w $(LDFLAGS_VERSION)' -o $(BINDIR)/$(NAME)-$@ $(GOFILES)

windows-amd64:
	GOARCH=amd64 GOOS=windows go build -ldflags '-s -w -H windowsgui $(LDFLAGS_VERSION)' -o $(BINDIR)/$(NAME)-$@.exe $(GOFILES)
