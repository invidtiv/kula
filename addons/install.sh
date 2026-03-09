#!/usr/bin/env bash

set -e

GITHUB_REPO="c0m4r/kula"

if command -v curl >/dev/null; then
    VERSION=$(curl -sI "https://github.com/${GITHUB_REPO}/releases/latest" | grep -i 'location:' | sed -E 's|.*/tag/([^[:space:]]+).*|\1|' | tail -n1 | tr -d '\r')
elif command -v wget >/dev/null; then
    VERSION=$(wget --server-response --max-redirect=0 "https://github.com/${GITHUB_REPO}/releases/latest" 2>&1 | grep -i 'location:' | sed -E 's|.*/tag/([^[:space:]]+).*|\1|' | tail -n1 | tr -d '\r')
else
    echo -e "\033[0;31mError: Neither curl nor wget is installed.\033[0m"
    exit 1
fi

if [ -z "$VERSION" ]; then
    echo -e "\033[0;31mError: Failed to fetch the latest version.\033[0m"
    exit 1
fi

if [[ ! "$VERSION" =~ ^[a-zA-Z0-9.-]+$ ]]; then
    echo -e "\033[0;31mError: Invalid version format received: $VERSION\033[0m"
    exit 1
fi

# Secure Temp Directory allocation
SECURE_TMP=$(mktemp -d /tmp/kula-install-XXXXXX)
trap 'rm -rf "$SECURE_TMP"' EXIT

RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"

# Define colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}===========================================${NC}"
echo -e "${CYAN}      kula - system monitoring daemon      ${NC}"
echo -e "${CYAN}===========================================${NC}"
echo -e "Version: ${VERSION}"
echo ""

# Detect Architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) HOST_ARCH="amd64" ;;
    aarch64) HOST_ARCH="arm64" ;;
    riscv64) HOST_ARCH="riscv64" ;;
    *)
        echo -e "${RED}Error: Unsupported architecture $ARCH${NC}"
        exit 1
        ;;
esac
echo -e "Detected Architecture: ${GREEN}${HOST_ARCH}${NC}"

# Detect OS
OS_FAMILY="unknown"
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_ID=${ID}
    OS_LIKE=${ID_LIKE:-""}
    
    if [[ "$OS_ID" == "debian" || "$OS_ID" == "ubuntu" || "$OS_LIKE" == *"debian"* || "$OS_LIKE" == *"ubuntu"* ]]; then
        OS_FAMILY="debian"
    elif [[ "$OS_ID" == "arch" || "$OS_ID" == "manjaro" || "$OS_LIKE" == *"arch"* ]]; then
        OS_FAMILY="arch"
    elif [[ "$OS_ID" == "fedora" || "$OS_ID" == "rhel" || "$OS_ID" == "rocky" || "$OS_ID" == "alma" || "$OS_LIKE" == *"fedora"* || "$OS_LIKE" == *"rhel"* ]]; then
        OS_FAMILY="rpm"
    fi
fi
echo -e "Detected OS Family: ${GREEN}${OS_FAMILY}${NC}"

# Detect Init System
INIT_SYSTEM="unknown"
if command -v systemctl >/dev/null 2>&1 && systemctl --no-pager >/dev/null 2>&1 || [ -d /run/systemd/system ]; then
    INIT_SYSTEM="systemd"
fi
echo -e "Detected Init System: ${GREEN}${INIT_SYSTEM}${NC}"

# Download function
download_and_verify() {
    local filename=$1
    local target="$SECURE_TMP/$filename"
    local url="${RELEASE_URL}/${filename}"

    echo -e "${BLUE}Downloading $filename...${NC}" >&2
    if command -v curl >/dev/null; then
        curl -sL "$url" -o "$target"
    elif command -v wget >/dev/null; then
        wget -qO "$target" "$url"
    else
        echo -e "${RED}Error: Neither curl nor wget is installed.${NC}" >&2
        exit 1
    fi

    if [ ! -s "$target" ]; then
        echo -e "${RED}Error: Download failed or file is empty ($url)${NC}" >&2
        rm -f "$target"
        exit 1
    fi

    local actual_hash
    if command -v sha256sum >/dev/null; then
        echo -e "${BLUE}Calculating SHA256 sum...${NC}" >&2
        actual_hash=$(sha256sum "$target" | awk '{print $1}')
    else
        echo -e "${YELLOW}Warning: sha256sum command is not available.${NC}" >&2
        echo -ne "Do you want to skip checksum verification? [y/N] " >&2
        read -n 1 -r < /dev/tty
        echo >&2
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            actual_hash="SKIPPED"
        else
            echo -e "${RED}Error: Checksum verification is required but sha256sum is missing.${NC}" >&2
            rm -f "$target"
            exit 1
        fi
    fi

    echo -e "${CYAN}Downloaded file: ${filename}${NC}" >&2
    echo -e "${CYAN}SHA256 sum:      ${actual_hash}${NC}" >&2

    echo -ne "Do you want to proceed with this installation? [y/N] " >&2
    read -n 1 -r < /dev/tty
    echo >&2
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${RED}Installation aborted by user.${NC}" >&2
        rm -f "$target"
        exit 1
    fi

    echo "$target"
}

# Determine action
INSTALL_METHOD=""

if [ "$OS_FAMILY" == "debian" ]; then
    INSTALL_METHOD="deb"
elif [ "$OS_FAMILY" == "rpm" ]; then
    INSTALL_METHOD="rpm"
elif [ "$OS_FAMILY" == "arch" ] && command -v pacman >/dev/null; then
    INSTALL_METHOD="aur"
else
    # Fallback options
    if [ "$INIT_SYSTEM" == "systemd" ]; then
        INSTALL_METHOD="tarball_systemd"
    elif command -v docker >/dev/null; then
        INSTALL_METHOD="docker"
    else
        INSTALL_METHOD="tarball_opt"
    fi
fi

echo -e "\nProposed installation method: ${YELLOW}${INSTALL_METHOD}${NC}"
read -p "Do you want to continue with this installation method? [y/N] " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${RED}Installation aborted.${NC}"
    exit 1
fi

exec_as_root() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    elif command -v sudo >/dev/null; then
        sudo "$@"
    elif command -v doas >/dev/null; then
        doas "$@"
    elif command -v su >/dev/null; then
        su -c "$*"
    else
        echo -e "${YELLOW}Warning: You are not root and sudo is not available. Installation may fail.${NC}"
        "$@"
    fi
}

echo ""

# Execute installation
if [ "$INSTALL_METHOD" == "deb" ]; then
    filename="kula-${VERSION}-${HOST_ARCH}.deb"
    target=$(download_and_verify "$filename")
    echo -e "${BLUE}Installing Debian package...${NC}"
    exec_as_root dpkg -i "$target" || exec_as_root apt-get install -f -y "$target"
    rm -f "$target"
    echo -e "${GREEN}Installation successful!${NC}"

elif [ "$INSTALL_METHOD" == "rpm" ]; then
    RPM_ARCH=$HOST_ARCH
    if [ "$HOST_ARCH" == "amd64" ]; then RPM_ARCH="x86_64"; fi
    if [ "$HOST_ARCH" == "arm64" ]; then RPM_ARCH="aarch64"; fi
    
    filename="kula-${VERSION}-${RPM_ARCH}.rpm"
    target=$(download_and_verify "$filename")
    echo -e "${BLUE}Installing RPM package...${NC}"
    if command -v dnf >/dev/null; then
        exec_as_root dnf install -y "$target"
    elif command -v yum >/dev/null; then
        exec_as_root yum install -y "$target"
    else
        exec_as_root rpm -ivh "$target"
    fi
    rm -f "$target"
    echo -e "${GREEN}Installation successful!${NC}"

elif [ "$INSTALL_METHOD" == "aur" ]; then
    filename="kula-${VERSION}-aur.tar.gz"
    # For makepkg, we should NOT be root.
    if [ "$(id -u)" -eq 0 ]; then
        echo -e "${RED}Error: AUR installation should not be run as root.${NC}"
        echo -e "Please run this script as a normal user with sudo privileges."
        exit 1
    fi
    target=$(download_and_verify "$filename")
    
    echo -e "${BLUE}Extracting and building AUR package...${NC}"
    build_dir="$SECURE_TMP/kula-aur-build"
    mkdir -p "$build_dir"
    tar -xzf "$target" -C "$build_dir"
    
    cd "$build_dir/kula-$VERSION-aur"
    makepkg -si
    cd - >/dev/null
    rm -f "$target"
    echo -e "${GREEN}Installation successful!${NC}"

elif [ "$INSTALL_METHOD" == "tarball_systemd" ]; then
    filename="kula-${VERSION}-${HOST_ARCH}.tar.gz"
    target=$(download_and_verify "$filename")
    
    echo -e "${BLUE}Installing from tarball to system directories...${NC}"
    extract_dir="$SECURE_TMP/kula_extract"
    mkdir -p "$extract_dir"
    tar -xzf "$target" -C "$extract_dir"
    
    cd "$extract_dir/kula"
    exec_as_root install -Dm755 kula /usr/bin/kula
    exec_as_root install -Dm644 addons/init/systemd/kula.service /etc/systemd/system/kula.service
    exec_as_root install -Dm640 config.example.yaml /etc/kula/config.example.yaml
    exec_as_root install -dm750 /var/lib/kula
    
    if ! getent group kula >/dev/null; then
        exec_as_root groupadd --system kula
    fi
    if ! getent passwd kula >/dev/null; then
        exec_as_root useradd --system -g kula -d /var/lib/kula -s /bin/false -c "Kula System Monitoring Daemon" kula
    fi
    exec_as_root chown -R kula:kula /etc/kula /var/lib/kula
    
    echo -e "${BLUE}Reloading systemd and enabling service...${NC}"
    exec_as_root systemctl daemon-reload
    exec_as_root systemctl enable kula.service
    exec_as_root systemctl start kula.service
    
    cd - >/dev/null
    rm -f "$target"
    echo -e "${GREEN}Installation successful!${NC}"
    
elif [ "$INSTALL_METHOD" == "docker" ]; then
    echo -e "${BLUE}Docker is installed. You can run Kula via Docker container.${NC}"
    echo -e "Run the following command to start Kula:"
    echo -e "${CYAN}docker run -d --name kula --net host -v kula_data:/var/lib/kula c0m4r/kula:latest${NC}"
    echo ""
    echo -e "To persist configuration, use volume mounts and provide your config.yaml."
    echo -e "You can find more at https://hub.docker.com/r/c0m4r/kula"
    echo ""
    exit 0
elif [ "$INSTALL_METHOD" == "tarball_opt" ]; then
    filename="kula-${VERSION}-${HOST_ARCH}.tar.gz"
    target=$(download_and_verify "$filename")
    
    echo -e "${BLUE}Installing to /opt/kula...${NC}"
    if [ ! -d "/opt/kula" ]; then
        exec_as_root mkdir -p /opt/kula
    fi
    exec_as_root tar -xzf "$target" -C /opt
    
    rm -f "$target"
    echo -e "${GREEN}Extracted to /opt/kula successfully.${NC}"
    echo -e "To run Kula manually:"
    echo -e "${CYAN}  cd /opt/kula${NC}"
    echo -e "${CYAN}  cp config.example.yaml config.yaml${NC}"
    echo -e "${CYAN}  ./kula serve${NC}"
fi

echo -e "\n${GREEN}Thank you for installing Kula!${NC}"
