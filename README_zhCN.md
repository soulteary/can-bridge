# CAN-Bridge 项目文档

## 项目概述

**CAN-Bridge** 是一个基于 Golang 开发的硬件适配网桥服务，专门用于支持使用 CAN（Controller Area Network）协议的设备进行高效稳定的数据通信。

本项目旨在提供一个简单易用的 HTTP API 接口，允许用户通过网络请求实现 CAN 消息的发送、接收以及设备状态监控，同时支持动态配置多个 CAN 接口。

## 功能特性

* **动态接口配置**：支持从命令行或环境变量动态配置多个 CAN 接口（例如 `can0`, `can1`）。
* **HTTP API 服务**：提供易于调用的 RESTful API 接口。
* **消息发送与验证**：支持发送标准 CAN 消息，并对消息进行数据长度及接口有效性验证。
* **健康检查与自动恢复**：内置接口健康监测，自动重启失效接口。
* **实时监控与统计**：提供接口状态、消息统计、发送成功率、错误记录以及平均延迟监控。

## 安装与使用

目前支持三种方式使用：裸金属安装、Docker 容器环境使用，从源码构建。

### 裸金属安装

推荐使用项目自带的一键安装脚本，将 CAN-Bridge 作为 systemd 服务运行。

#### 快速安装（推荐）

```bash
curl -sSL https://raw.githubusercontent.com/linker-bot/can-bridge/main/install_with_systemd.sh | sudo bash
```

或已克隆仓库时：

```bash
cd can-bridge
chmod +x install_with_systemd.sh
./install_with_systemd.sh
```

该脚本会自动：
- 下载最新版本的可执行文件
- 安装到 `/usr/local/bin/can-bridge`
- 生成 systemd 服务文件 `/etc/systemd/system/can-bridge.service`
- 启动并设置开机自启

如需自定义 `CAN_PORTS` 或 `SERVER_PORT`，可编辑 `/etc/systemd/system/can-bridge.service`，修改后执行：

```bash
sudo systemctl daemon-reload
sudo systemctl restart can-bridge.service
```

查看服务状态：

```bash
sudo systemctl status can-bridge.service
```

### Docker 容器环境使用

#### 方法一：使用构建好的 Docker 镜像

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

#### 方法二：自行构建

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

### 从源码构建

系统要求：

* Linux 操作系统
* Golang 环境（Go 1.20 以上版本推荐）
* CAN 设备接口（物理或虚拟）

#### 安装与启动

1. 克隆项目

```bash
git clone https://github.com/linker-bot/can-bridge.git
cd can-bridge
```

2. 编译

```bash
go build -o can-bridge
```

3. 运行

通过命令行参数指定 CAN 接口：

```bash
./can-bridge -can-ports can0,can1 -port 5260
```

或使用环境变量指定配置：

```bash
export CAN_PORTS=can0,can1
export SERVER_PORT=5260
./can-bridge
```

## API 文档

### 基础路径

`http://localhost:5260/api`

### 发送原始 CAN 消息

* **URL**: `/can`
* **方法**: `POST`

**请求体示例**:

```json
{
  "interface": "can0",
  "id": 256,
  "data": [1, 2, 3, 4]
}
```

### 控制手指姿势（兼容旧版）

* **URL**: `/fingers`
* **方法**: `POST`

**请求体示例**:

```json
{
  "pose": [10, 20, 30, 40, 50, 60]
}
```

### 控制手掌姿势（兼容旧版）

* **URL**: `/palm`
* **方法**: `POST`

**请求体示例**:

```json
{
  "pose": [70, 80, 90, 100]
}
```

### 查询接口状态与统计信息

* **URL**: `/status`
* **方法**: `GET`

返回所有接口的运行状态、发送统计和错误信息。

### 获取已配置的 CAN 接口

* **URL**: `/interfaces`
* **方法**: `GET`

## 性能优化与稳定性

* 支持消息发送重试机制，确保数据传输可靠性。
* 使用互斥锁（Mutex）确保多线程安全性。
* 实时监测接口健康状态并进行自动恢复。

## 日志与调试

日志采用标准输出，格式友好，包含清晰的错误提示和运行状态信息。

## 部署建议

建议使用 systemd 或 Docker 容器化进行部署，确保服务长期稳定运行。

## 贡献指南

欢迎提交 Issue 和 Pull Request 来帮助项目完善和优化。

## 许可证

本项目使用 Apache-2.0 license 许可证。
