# Release

```bash
# Update version on changelog
vim CHANGELOG.md
git add .
git commit -m "release: Version $VERSION"
# Tag release
git tag $VERSION
git push
git push origin $VERSION
```

In a clean repository:

```bash
cd /tmp/
git clone https://github.com/pando85/GoHeishaMon
cd GoHeishaMon
git checkout $VERSION

make all
make upx
make compilesquash-usb
```

Manually upload `dist` directory and `OS/Kernel/openwrt-ar71xx-generic-cus531-16M-kernel.bin` to the release page.
