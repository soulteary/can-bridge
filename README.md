# CAN-Bridge Project Documentation

[中文](README_zhCN.md)

<p><img src=".github/logo.png" width=240></p>

## Project Overview

**CAN-Bridge** is a hardware adaptation bridge service developed in Golang, specifically designed to support efficient and stable data communication for devices using the CAN (Controller Area Network) protocol.

The project aims to provide an easy-to-use HTTP API interface, allowing users to send and receive CAN messages, manage interface configurations, and monitor device status via network requests, while also supporting dynamic configuration of multiple CAN interfaces.

## Features

### New Features

* **Interface Setup Manager**:

  * Automatically executes `ip link set can0 up type can bitrate 1000000`
  * Configurable bitrate, sample point, and restart timeout
  * Provides retry mechanism and error handling
  * Supports interface state querying and validation

* **Enhanced Configuration System**:

  * Supports configuration via command-line parameters and environment variables
  * Provides configuration validation to ensure parameter correctness, including bitrate
  * Includes detailed usage instructions

* **Comprehensive Interface Management API**:

  * Configuration management API (`GET /api/setup/config`, `PUT /api/setup/config`)
  * Interface operation API (setup, shutdown, reset, status query)
  * Batch operation API (setup or teardown all interfaces at once)

### Main Functions

* **Dynamic Interface Configuration**: Supports dynamic configuration of multiple CAN interfaces (e.g., `can0`, `can1`) via command-line or environment variables.
* **HTTP API Service**: Provides easy-to-use RESTful API interfaces.
* **Message Sending and Validation**: Supports sending standard CAN messages and performs validation of data length and interface availability.
* **Health Check and Automatic Recovery**: Built-in interface health monitoring with automatic recovery of failed interfaces.
* **Real-time Monitoring and Statistics**: Offers monitoring of interface status, message statistics, success rates, error logging, and average latency.

### Program Features

* ✅ Automatic Initialization: Automatically configures CAN interfaces on program startup.
* ✅ Retry Mechanism: Automatically retries interface setup upon failure.
* ✅ Status Monitoring: Real-time monitoring of interface status and error statistics.
* ✅ Graceful Shutdown: Automatically shuts down interfaces upon program exit.
* ✅ Dependency Injection: Facilitates testing and extension.
* ✅ Error Handling: Comprehensive error handling and logging.

## Installation and Usage

Supports installation on bare-metal systems, Docker containers, and source code builds.

For detailed installation instructions, see the [Installation Guide](docs/install.md).

### Usage Examples

**Basic Usage**

```bash
./can-bridge -can-ports can0,can1
```

**Custom Bitrate**

```bash
./can-bridge -can-ports can0 -bitrate 500000
```

**Disable Automatic Setup (Managed via API)**

```bash
./can-bridge -auto-setup=false
```

**Configure Interface via API**

```bash
curl -X POST localhost:5260/api/setup/interfaces/can0 \
  -H "Content-Type: application/json" \
  -d '{"bitrate": 500000, "withRetry": true}'
```

## API Documentation

### Base Path

`http://localhost:5260/api`

### New Interface Setup Management API

**Configuration Management**:

* `GET /api/setup/config`
* `PUT /api/setup/config`

**Interface Operations**:

* `GET /api/setup/available`
* `POST /api/setup/interfaces/{name}`
* `DELETE /api/setup/interfaces/{name}`
* `POST /api/setup/interfaces/{name}/reset`
* `GET /api/setup/interfaces/{name}/state`

**Batch Operations**:

* `POST /api/setup/interfaces/setup-all`
* `POST /api/setup/interfaces/teardown-all`

## Performance Optimization and Stability

* Implements retry mechanisms for reliable message transmission.
* Utilizes mutex locks to ensure thread safety.
* Real-time monitoring of interface health status with automatic recovery.

## Logging and Debugging

Logs are output to the standard output stream in a friendly format, including clear error messages and runtime status information.

## Deployment Recommendations

Deployment using systemd or Docker containers is recommended to ensure long-term stable operation.

## Contribution Guide

Issues and Pull Requests are welcomed to improve and optimize the project.

## License

This project is licensed under the Apache-2.0 license.
