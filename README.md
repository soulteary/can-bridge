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

TBD

### Docker Container Environment

TBD

### Building from Source

System requirements:

* Linux operating system
* Golang environment (Go 1.20 or higher recommended)
* CAN device interface (physical or virtual)

#### Installation and Startup

1. Clone the project

```bash
git clone https://github.com/your-repo/can-bridge.git
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
