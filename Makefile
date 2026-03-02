SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

ARCH ?= amd64
IMAGE ?= alpine:3.20
KERNEL_VERSION ?= 6.1.155
TESTDATA := e2e/testdata

.PHONY: build initrd rootfs test kernel init e2e testdata clean

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build \
		-trimpath -ldflags="-s -w" \
		-o build/init ./cmd/init

initrd: build
	scripts/build-initrd.sh build/initrd.cpio $(CONFIG)

rootfs:
	@mkdir -p $(TESTDATA)
	scripts/build-rootfs.sh $(IMAGE) $(TESTDATA)/rootfs.ext4

test:
	go test ./...

kernel:
	@mkdir -p $(TESTDATA)
	curl -fSL -o $(TESTDATA)/kernel.tar.gz \
		https://github.com/pigeon-as/pigeon-kernel-kit/releases/download/v$(KERNEL_VERSION)/pigeon-kernel-$(KERNEL_VERSION)-x86_64.tar.gz
	tar -xzf $(TESTDATA)/kernel.tar.gz -C $(TESTDATA)
	rm -f $(TESTDATA)/kernel.tar.gz

init: build
	scripts/build-initrd.sh $(TESTDATA)/initrd.cpio

testdata: kernel init rootfs

e2e: clean testdata
	go test -tags=e2e -count=1 -v ./e2e  # requires root (sudo make e2e)

clean:
	rm -rf build/ $(TESTDATA)/
