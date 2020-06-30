.PHONY: build
build:
	go build -o bin/tagane -ldflags "-X github.com/jpillora/chisel/share.BuildVersion=1.6.0" .
