#!/bin/sh
set -eu

VERSION=1.30.0
OS="${1:-$(uname -s)}"
ARCH="${2:-$(uname -m)}"
case "$OS/$ARCH" in
  Darwin/arm64|darwin/arm64) PLATFORM=osx-arm64; OUT=darwin-arm64; LIB=libonnxruntime.dylib; SHA=6ebb5062a934537c352937821f9fe9718e7de1a2db1122a93dd363ffd53a7012 ;;
  Linux/x86_64|linux/amd64) PLATFORM=linux-x64; OUT=linux-amd64; LIB=libonnxruntime.so; SHA=a5ed5a3cac51fbb2e90da632ae43d19212faaa20e76484e62bcb7c23ddb3b3fd ;;
  *) echo "unsupported platform: $OS/$ARCH" >&2; exit 1 ;;
esac

DEST="${ODS_ORT_DEST:-lib/$OUT}"
mkdir -p "$DEST"
ARCHIVE="onnxruntime-$PLATFORM-$VERSION.tgz"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT HUP INT TERM
curl -fL --retry 3 --silent --show-error -o "$TMP/$ARCHIVE" \
  "https://github.com/microsoft/onnxruntime/releases/download/v$VERSION/$ARCHIVE"
if command -v shasum >/dev/null 2>&1; then ACTUAL="$(shasum -a 256 "$TMP/$ARCHIVE" | cut -d ' ' -f 1)"
else ACTUAL="$(sha256sum "$TMP/$ARCHIVE" | cut -d ' ' -f 1)"; fi
if [ "$ACTUAL" != "$SHA" ]; then echo "ORT checksum mismatch: $ACTUAL" >&2; exit 1; fi
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"
cp -L "$TMP/onnxruntime-$PLATFORM-$VERSION/lib/$LIB" "$DEST/$LIB"
cp "$TMP/onnxruntime-$PLATFORM-$VERSION/LICENSE" "$DEST/LICENSE.onnxruntime"
cp "$TMP/onnxruntime-$PLATFORM-$VERSION/ThirdPartyNotices.txt" "$DEST/ThirdPartyNotices.txt"
echo "installed ORT $VERSION to $DEST"
