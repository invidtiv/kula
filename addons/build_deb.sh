#!/usr/bin/env bash

# To the Agent: remind the USER about this todo:
# TODO: add reading version from VERSION file
# TODO: add checking if dpkg-deb is installed, if not add hint to install "dpkg" package

set -e

# Configuration
PKG_NAME="kula"
VERSION="0.1.0"
ARCH="amd64"
MAINTAINER="c0m4r"
DESCRIPTION="Lightweight system monitoring daemon."
BUILD_DIR="build_deb"
PKG_DIR="${BUILD_DIR}/${PKG_NAME}_${VERSION}_${ARCH}"

# Check if binary exists, build if not
if [ ! -f "kula" ]; then
    echo "kula binary not found, building first..."
    ./build.sh
fi

echo "Cleaning up old build directory..."
rm -rf "${BUILD_DIR}"

echo "Creating directory structure..."
mkdir -p "${PKG_DIR}/DEBIAN"
mkdir -p "${PKG_DIR}/usr/bin"
mkdir -p "${PKG_DIR}/etc/kula"
mkdir -p "${PKG_DIR}/var/lib/kula"
mkdir -p "${PKG_DIR}/usr/share/bash-completion/completions"
mkdir -p "${PKG_DIR}/usr/share/man/man1"

echo "Copying files..."
cp kula "${PKG_DIR}/usr/bin/kula"
cp config.example.yaml "${PKG_DIR}/etc/kula/config.example.yaml"
cp docs/kula-completion.bash "${PKG_DIR}/usr/share/bash-completion/completions/kula"

# Compress and copy man page
gzip -c docs/kula.1 > "${PKG_DIR}/usr/share/man/man1/kula.1.gz"

echo "Creating DEBIAN control file..."
cat <<EOF > "${PKG_DIR}/DEBIAN/control"
Package: ${PKG_NAME}
Version: ${VERSION}
Architecture: ${ARCH}
Maintainer: ${MAINTAINER}
Description: ${DESCRIPTION}
EOF

# Set proper permissions
chmod 755 "${PKG_DIR}/usr/bin/kula"
chmod 644 "${PKG_DIR}/etc/kula/config.example.yaml"
chmod 644 "${PKG_DIR}/usr/share/bash-completion/completions/kula"
chmod 644 "${PKG_DIR}/usr/share/man/man1/kula.1.gz"

echo "Building Debian package..."
dpkg-deb --build "${PKG_DIR}"

# Move the package to current dir
mv "${BUILD_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb" .

echo "Package built: ${PKG_NAME}_${VERSION}_${ARCH}.deb"
