SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

ARCH ?= amd64
IMAGE ?= alpine:3.20

.PHONY: build initrd rootfs test clean

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build \
		-trimpath -ldflags="-s -w" \
		-o out/init ./cmd/init

initrd: build
	scripts/build-initrd.sh $(or $(CONFIG),example/run.json)

rootfs:
	scripts/build-rootfs.sh $(IMAGE)

test:
	go test ./...

clean:
	rm -rf out/
