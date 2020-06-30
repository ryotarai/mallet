.PHONY: build
build:
	go build -o bin/mallet -ldflags "-X github.com/jpillora/chisel/share.BuildVersion=1.6.0" .

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/mallet-linux-amd64 -ldflags "-X github.com/jpillora/chisel/share.BuildVersion=1.6.0" .
