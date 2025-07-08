#!/bin/bash

set -e

PROJECT_NAME="slm-linux"
BIN_NAME="slm"
DEST_DIR="$HOME/bin"

clean() {
    echo "Cleaning up..."
    rm -rf "$PWD/$BIN_NAME"
    rm -rf "$DEST_DIR/$BIN_NAME"
}

build() {
    echo "Building slm..."
    go build -o "$PWD/$BIN_NAME" "$PROJECT_NAME.go"
    echo "Build complete"
}

tests() {
    echo "Running tests..."
    go test -v .
}

install() {
  install -m 755 "$BIN_NAME" "$DEST_DIR"
  echo "Installed to "$DEST_DIR". Ensure your PATH is set"
}

usage() {
  echo 'usage ./build.sh [tests|build|install|clean]' &2>1
  exit 1
}

case "$1" in
 "clean")
   clean
   ;;
 "install")
   install
   ;;
 "test")
   test
   ;;
 "build")
   build
   ;;
 *)
   usage
   ;;
esac

echo "Build completed successfully!"
exit 0

