# Release

```bash
# Update version on changelog
vim CHANGELOG.md
git add .
git commit -m "release: Version $VERSION"
# Tag release
git tag $VERSION
git push origin $VERSION
```

In a clean repository:

```bash
cd /tmp/
git clone https://github.com/pando85/GoHeishaMon

make all
make upx
make compilesquash-usb
```

Manually upload dist directory to the release page.
