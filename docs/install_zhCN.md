# 安装与使用

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