SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

ARCH ?= amd64
IMAGE ?= alpine:3.20
E2E_IMAGE ?= alpine:3.20
KERNEL_VERSION ?= 6.1.102
TESTDATA := e2e/testdata

.PHONY: build initrd rootfs test e2e testdata clean

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build \
		-trimpath -ldflags="-s -w" \
		-o out/init ./cmd/init

initrd: build
	scripts/build-initrd.sh $(CONFIG)

rootfs:
	scripts/build-rootfs.sh $(IMAGE)

test:
	go test ./...

testdata: build
	@mkdir -p $(TESTDATA)
	if [ ! -f $(TESTDATA)/vmlinux ]; then \
		curl -fSL -o $(TESTDATA)/kernel.tar.gz \
			https://github.com/pigeon-as/pigeon-kernel-kit/releases/download/v$(KERNEL_VERSION)/pigeon-kernel-$(KERNEL_VERSION)-x86_64.tar.gz; \
		tar -xzf $(TESTDATA)/kernel.tar.gz -C $(TESTDATA); \
		rm -f $(TESTDATA)/kernel.tar.gz; \
	fi
	if [ ! -f $(TESTDATA)/rootfs.ext4 ]; then \
		scripts/build-rootfs.sh $(E2E_IMAGE) $(TESTDATA)/rootfs.ext4; \
	fi
	if [ ! -f $(TESTDATA)/initrd.cpio ]; then \
		scripts/build-initrd.sh; \
		mv out/initrd.cpio $(TESTDATA)/initrd.cpio; \
	fi

e2e: testdata
	go test -tags=e2e -count=1 -v ./e2e  # requires root (sudo make e2e)

clean:
	rm -rf out/ $(TESTDATA)/
