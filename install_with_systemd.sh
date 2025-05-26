#!/usr/bin/env bash

set -e

if [ "$EUID" -ne 0 ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "Error: This script requires root privileges. Please run as root or install sudo."
    exit 1
  fi
fi

# Repository details
REPO="eli-yip/can-bridge"
BINARY_NAME="can-bridge_linux_amd64"
INSTALL_PATH="/usr/local/bin/can-bridge"

# Systemd service details
SERVICE_NAME="can-bridge.service"
SYSTEMD_PATH="/etc/systemd/system/${SERVICE_NAME}"

echo "Installing latest can-bridge version"

# 1. Get the latest release version from GitHub API
# Using curl to fetch the latest release information and grep/perl to parse the tag_name.
LATEST_VERSION=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep -Po '"tag_name": "\K[^"]*')

if [ -z "$LATEST_VERSION" ]; then
    echo "Error: Could not retrieve the latest version. Check the repository name or network connection."
    exit 1
fi

echo "Detected latest version: $LATEST_VERSION"

# 2. Create a temporary directory and navigate into it
# mktemp -d creates a unique temporary directory.
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

# 3. Download and install the binary
# Construct the download URL using the detected latest version.
ASSET_URL="https://github.com/$REPO/releases/download/$LATEST_VERSION/can-bridge_${LATEST_VERSION#v}_linux_amd64.tar.gz"
echo "Downloading: $ASSET_URL"
# curl -sSL: Silent, show errors, follow redirects. -o: Output to file.
curl -sSL -o can-bridge.tar.gz "$ASSET_URL"

# Extract the tarball, make the binary executable, and move it to the install path.
tar -xzf can-bridge.tar.gz
chmod +x "$BINARY_NAME"
sudo mv "$BINARY_NAME" "$INSTALL_PATH"

# 4. Generate the systemd unit file
# tee -a: Append output to files, -a is not strictly needed here as we want to overwrite.
# > /dev/null: Redirect standard output to null to prevent tee from printing to console.
sudo tee "$SYSTEMD_PATH" > /dev/null <<EOF
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

# 5. Reload systemd and enable/start the service
# daemon-reload: Reload systemd manager configuration.
# enable --now: Enable the service to start on boot and start it immediately.
sudo systemctl daemon-reload
sudo systemctl enable --now "$SERVICE_NAME"

echo "can-bridge installed and started as systemd service."
echo "Edit $SYSTEMD_PATH to customize CAN_PORTS or SERVER_PORT if needed."