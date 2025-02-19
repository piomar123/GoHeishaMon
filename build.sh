#!/bin/bash

set -ex

cd "/usr/src/app"
# go get github.com/rs/xid
# go get github.com/BurntSushi/toml
# go get github.com/eclipse/paho.mqtt.golang
# go get go.bug.st/serial
make deps
#make all
make test
make build-mips
make upx
#make compilesquash
make compile-plain-squashfs
cp OS/Kernel/openwrt-ar71xx-generic-cus531-16M-kernel.bin dist/
