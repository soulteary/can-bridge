# Installation and Usage

Currently, three methods of usage are supported: bare-metal installation, Docker container environment, and building from source.

### Bare-metal Installation

It is recommended to use the project's built-in one-click installation script to run CAN-Bridge as a systemd service.

#### Quick Installation (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/linker-bot/can-bridge/main/install_with_systemd.sh | sudo bash
```

Or, if the repository is already cloned:

```bash
cd can-bridge
chmod +x install_with_systemd.sh
./install_with_systemd.sh
```

This script automatically:

* Downloads the latest executable
* Installs it to `/usr/local/bin/can-bridge`
* Creates a systemd service file `/etc/systemd/system/can-bridge.service`
* Starts the service and sets it to auto-start on boot

To customize `CAN_PORTS` or `SERVER_PORT`, edit `/etc/systemd/system/can-bridge.service`, then execute:

```bash
sudo systemctl daemon-reload
sudo systemctl restart can-bridge.service
```

To check the service status:

```bash
sudo systemctl status can-bridge.service
```

### Docker Container Environment

#### Method 1: Using Pre-built Docker Image

```bash
docker run -d --rm \
  --device=/dev/can0:/dev/can0 \
  --device=/dev/can1:/dev/can1 \
  -p 5260:5260 \
  -e CAN_PORTS="can0,can1" \
  -e SERVER_PORT="5260" \
  --name can-bridge-service \
  ghcr.io/linker-bot/can-bridge:latest
```

#### Method 2: Build Your Own

```bash
docker build --platform linux/amd64 -t can-bridge:latest .
```

```bash
docker run -d --rm \
  --device=/dev/can0:/dev/can0 \
  --device=/dev/can1:/dev/can1 \
  -p 5260:5260 \
  -e CAN_PORTS="can0,can1" \
  -e SERVER_PORT="5260" \
  --name can-bridge-service \
  ghcr.io/linker-bot/can-bridge:latest
```

### Building from Source

System Requirements:

* Linux Operating System
* Golang Environment (Go 1.20 or higher recommended)
* CAN device interface (physical or virtual)

#### Installation and Startup

1. Clone the Project

```bash
git clone https://github.com/linker-bot/can-bridge.git
cd can-bridge
```

2. Compile

```bash
go build -o can-bridge
```

3. Run

Specify CAN interfaces through command-line parameters:

```bash
./can-bridge -can-ports can0,can1 -port 5260
```

Or specify configurations using environment variables:

```bash
export CAN_PORTS=can0,can1
export SERVER_PORT=5260
./can-bridge
```
