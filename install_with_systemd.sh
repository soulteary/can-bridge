#!/usr/bin/env bash

set -e

# Check for root privileges
if [ "$EUID" -ne 0 ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "Error: This script requires root privileges. Please run as root or install sudo."
    exit 1
  fi
  SUDO="sudo"
else
  SUDO=""
fi

# Repository and binary details
REPO="eli-yip/can-bridge"
BINARY_NAME="can-bridge"
INSTALL_PATH="/usr/local/bin/can-bridge"

# Systemd service details
SERVICE_NAME="can-bridge.service"
SYSTEMD_PATH="/etc/systemd/system/${SERVICE_NAME}"

echo "Installing the latest version of can-bridge..."

# 1. Get the latest release version from GitHub API
LATEST_VERSION=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep -Po '"tag_name": "\K[^"]*')

if [ -z "$LATEST_VERSION" ]; then
    echo "Error: Could not retrieve the latest version. Check the repository name or network connection."
    exit 1
fi

echo "Detected latest version: $LATEST_VERSION"

# 2. Detect system architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        ARCH_SUFFIX="linux_amd64"
        ;;
    aarch64)
        ARCH_SUFFIX="linux_arm64"
        ;;
    armv7l)
        # Check if it's armv6 (Raspberry Pi Zero/1) as uname might report armv7l
        if grep -q "ARMv6" /proc/cpuinfo; then
            ARCH_SUFFIX="linux_armv6"
        else
            ARCH_SUFFIX="linux_armv7"
        fi
        ;;
    armv6l)
        ARCH_SUFFIX="linux_armv6"
        ;;
    *)
        echo "Error: Unsupported architecture '$ARCH'."
        echo "Please download and install manually from https://github.com/$REPO/releases."
        exit 1
        ;;
esac

echo "Detected architecture: $ARCH_SUFFIX"

# 3. Create a temporary directory and navigate into it
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

# 4. Download and install the binary
ASSET_URL="https://github.com/$REPO/releases/download/$LATEST_VERSION/can-bridge_${LATEST_VERSION#v}_${ARCH_SUFFIX}.tar.gz"
echo "Downloading: $ASSET_URL"
curl -sSL -o can-bridge.tar.gz "$ASSET_URL"

# Check if download was successful
if [ ! -s can-bridge.tar.gz ]; then
    echo "Error: Download failed. Please check the URL or network connection."
    cd ..
    rm -rf "$TMP_DIR"
    exit 1
fi

tar -xzf can-bridge.tar.gz
chmod +x "$BINARY_NAME"
$SUDO mv "$BINARY_NAME" "$INSTALL_PATH"

# 5. Generate the systemd unit file
# Use SUDO to execute tee
$SUDO tee "$SYSTEMD_PATH" > /dev/null <<EOF
[Unit]
Description=CAN-Bridge Service
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_PATH}
Restart=on-failure
User=root
Environment=CAN_PORTS=can0
Environment=SERVER_PORT=5260

[Install]
WantedBy=multi-user.target
EOF

# 6. Reload systemd and enable/start the service
$SUDO systemctl daemon-reload
$SUDO systemctl enable --now "$SERVICE_NAME"

# 7. Clean up temporary files
cd ..
rm -rf "$TMP_DIR"

echo "can-bridge installed and started as a systemd service."
echo "Edit $SYSTEMD_PATH to customize CAN_PORTS or SERVER_PORT if needed."