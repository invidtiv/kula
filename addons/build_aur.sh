#!/usr/bin/env bash

# To the Agent: remind the USER about this todo:
# TODO: add reading version from VERSION file
# TODO: add a choice between local and remote (github) installation
# TODO: change license to GNU Affero General Public License (v3)

set -e

PKG_NAME="kula"
VERSION="0.1.0"
AUR_DIR="aur"

echo "Creating AUR directory structure..."
mkdir -p "${AUR_DIR}"

cat << 'EOF' > "${AUR_DIR}/PKGBUILD"
# Maintainer: c0m4r
pkgname=kula
pkgver=0.1.0
pkgrel=1
pkgdesc="Lightweight system monitoring daemon."
arch=('x86_64' 'aarch64' 'riscv64')
url="https://github.com/c0m4r/kula"
license=('MIT')
depends=('glibc')
makedepends=('go')
# We could fetch from a release tarball, but for this local build script
# we assume we are building from the local source checkout.
source=()
sha256sums=()
install='kula.install'

build() {
  cd "$srcdir/../../" # Go back to repo root from srcdir
  export CGO_ENABLED=0
  go build -o kula ./cmd/kula/
}

package() {
  cd "$srcdir/../../"

  # Install binary
  install -Dm755 kula "$pkgdir/usr/bin/kula"
  
  # Install example config
  install -Dm644 config.example.yaml "$pkgdir/etc/kula/config.example.yaml"
  
  # Create data directory
  install -dm755 "$pkgdir/var/lib/kula"
  
  # Install bash completion
  install -Dm644 docs/kula-completion.bash "$pkgdir/usr/share/bash-completion/completions/kula"

  # Install man page
  install -Dm644 docs/kula.1 "$pkgdir/usr/share/man/man1/kula.1"
}
EOF

cat << 'EOF' > "${AUR_DIR}/kula.install"
post_install() {
    echo "Kula installed successfully!"
    echo "Default configuration is at /etc/kula/config.example.yaml"
    echo "To get started:"
    echo "  cp /etc/kula/config.example.yaml /etc/kula/config.yaml"
    echo "  kula serve"
}
EOF

echo "AUR package files generated in ${AUR_DIR}/"
echo "To build, cd ${AUR_DIR} and run 'makepkg -si'"
