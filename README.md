# CAN-Bridge Project Documentation

## Project Overview

**CAN-Bridge** is a Golang-based hardware-adapted bridge service, specifically designed to efficiently and reliably support data communication for devices using the CAN (Controller Area Network) protocol.

This project aims to provide a simple and user-friendly HTTP API that allows users to send and receive CAN messages and monitor device status through network requests, with support for dynamically configuring multiple CAN interfaces.

## Features

* **Dynamic Interface Configuration**: Supports dynamic configuration of multiple CAN interfaces (e.g., `can0`, `can1`) through command line or environment variables.
* **HTTP API Service**: Provides easy-to-use RESTful API endpoints.
* **Message Sending and Validation**: Supports sending standard CAN messages with data length and interface validity validation.
* **Health Check and Automatic Recovery**: Built-in health monitoring of interfaces and automatic restart of failed interfaces.
* **Real-time Monitoring and Statistics**: Provides interface status, message statistics, success rates, error logs, and average latency monitoring.

## Installation and Usage

Currently supports three usage methods: bare-metal installation, Docker container environment, and building from source.

### Bare-metal Installation

You can install and run CAN-Bridge as a systemd service using the provided installation script.

#### Quick Install (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/linker-bot/can-bridge/main/install_with_systemd.sh | sudo bash
```

Or, if you have already cloned the repository:

```bash
cd can-bridge
chmod +x install_with_systemd.sh
./install_with_systemd.sh
```

This script will:
- Download the latest release binary from GitHub
- Install it to `/usr/local/bin/can-bridge`
- Set up a systemd service at `/etc/systemd/system/can-bridge.service`
- Start and enable the service

You can edit `/etc/systemd/system/can-bridge.service` to customize `CAN_PORTS` or `SERVER_PORT` as needed, then reload and restart:

```bash
sudo systemctl daemon-reload
sudo systemctl restart can-bridge.service
```

Check status:

```bash
sudo systemctl status can-bridge.service
```

### Docker Container Usage

#### Method 1: Using a Pre-built Docker Image

```bash
docker run -d --rm \
  --device=/dev/can0:/dev/can0 \
  --device=/dev/can1:/dev/can1 \
  -p 5260:5260 \
  -e CAN_PORTS="can0,can1" \
  -e SERVER_PORT="5260" \
  --name can-bridge-service \
  eliyip/can-bridge:latest
```

#### Method 2: Building It Yourself

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
  eliyip/can-bridge:latest
```

### Building from Source

System requirements:

* Linux operating system
* Golang environment (Go 1.20 or higher recommended)
* CAN device interface (physical or virtual)

#### Installation and Startup

1. Clone the project

```bash
git clone https://github.com/linker-bot/can-bridge.git
cd can-bridge
```

2. Build

```bash
go build -o can-bridge
```

3. Run

Specify CAN interfaces via command line arguments:

```bash
./can-bridge -can-ports can0,can1 -port 5260
```

Or specify configuration using environment variables:

```bash
export CAN_PORTS=can0,can1
export SERVER_PORT=5260
./can-bridge
```

## API Documentation

### Base Path

`http://localhost:5260/api`

### Sending Raw CAN Messages

* **URL**: `/can`
* **Method**: `POST`

**Example Request**:

```json
{
  "interface": "can0",
  "id": 256,
  "data": [1, 2, 3, 4]
}
```

### Finger Pose Control (Legacy Support)

* **URL**: `/fingers`
* **Method**: `POST`

**Example Request**:

```json
{
  "pose": [10, 20, 30, 40, 50, 60]
}
```

### Palm Pose Control (Legacy Support)

* **URL**: `/palm`
* **Method**: `POST`

**Example Request**:

```json
{
  "pose": [70, 80, 90, 100]
}
```

### Interface Status and Statistics

* **URL**: `/status`
* **Method**: `GET`

Returns running status, message statistics, and error information for all interfaces.

### Get Configured CAN Interfaces

* **URL**: `/interfaces`
* **Method**: `GET`

## Performance Optimization and Stability

* Supports message retry mechanisms to ensure reliable data transmission.
* Uses mutex locks to ensure multi-threaded safety.
* Real-time monitoring of interface health and automatic recovery.

## Logging and Debugging

Logs use standard output with a friendly format, clearly indicating errors and operational status.

## Deployment Recommendations

It is recommended to deploy using systemd or Docker containerization to ensure long-term stable operation.

## Contribution Guidelines

Contributions via Issues and Pull Requests are welcome to help improve and optimize the project.

## License

This project is licensed under the Apache-2.0 License.
