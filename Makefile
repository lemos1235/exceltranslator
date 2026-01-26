NAME=Excel-Translator
BINDIR=bin
GOFILES=cmd/qt/*.go

darwin-arm64:
	GOARCH=arm64 GOOS=darwin go build -ldflags '-s -w' -o $(BINDIR)/$(NAME)-$@ $(GOFILES)

windows-amd64:
	GOARCH=amd64 GOOS=windows go build -ldflags '-s -w -H windowsgui' -o $(BINDIR)/$(NAME)-$@.exe $(GOFILES)
