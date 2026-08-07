# 服务器安装方案

本文说明如何在服务器上安装运行 **go-mysqlbinlog-to-es**（同步进程 + Admin 配置界面）。

源码仓库：https://github.com/JasonYe88/go-mysqlbinlog-to-es

```bash
git clone https://github.com/JasonYe88/go-mysqlbinlog-to-es.git
```


| 方式 | 适用场景 | 推荐度 |
|------|----------|--------|
| [Docker Compose](#一docker-安装推荐) | 生产 / 测试，环境隔离，运维简单 | 推荐 |
| [系统源码编译](#二系统源码编译ubuntu--debian--centos) | 无 Docker、需与主机 mysqldump 对齐、定制 systemd | 可选 |

**前置条件（两种方式通用）**

1. MySQL 已开启 binlog（`ROW` / `MIXED`），账号具备复制与读表权限  
2. 能访问目标 Elasticsearch  
3. 配置好 `configs/river.toml`（`server_id` 全局唯一）  
4. 时区建议 `Asia/Shanghai`  

更细的配置与排障见：[configuration.md](./configuration.md)、[deploy.md](./deploy.md)、[admin-ui.md](./admin-ui.md)。

---

## 一、Docker 安装（推荐）

### 1.1 安装 Docker 与 Compose

以下以常见发行版为例（需 root 或 sudo）。

#### Ubuntu / Debian

```bash
# 卸载旧版（如有）
sudo apt-get remove -y docker docker-engine docker.io containerd runc 2>/dev/null || true

sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg

sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/$(. /etc/os-release; echo "$ID")/gpg \
  | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/$(. /etc/os-release; echo "$ID") \
  $(. /etc/os-release; echo "$VERSION_CODENAME") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

sudo systemctl enable --now docker
docker version
docker compose version
```

国内若拉不动官方源，可改用发行版自带包或镜像站文档安装 Docker，只要最终有 `docker` 与 `docker compose` 即可。

#### CentOS / RHEL / Rocky / Alma

```bash
sudo yum remove -y docker docker-client docker-client-latest docker-common \
  docker-latest docker-latest-logrotate docker-logrotate docker-engine 2>/dev/null || true

sudo yum install -y yum-utils
sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo

sudo yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
docker version
docker compose version
```

CentOS 7 请确认内核与 SELinux 策略允许 Docker；生产建议 Rocky/Alma 8+。

### 1.2 获取代码并配置

```bash
# 将项目放到服务器，例如：
cd /opt
git clone https://github.com/JasonYe88/go-mysqlbinlog-to-es.git
cd /opt/go-mysqlbinlog-to-es

mkdir -p configs var
# 编辑配置（至少改 MySQL / ES / server_id / source / rule）
vi configs/river.toml
```

`river.toml` 中 Docker 场景建议：

```toml
data_dir = "/app/var"
stat_addr = "0.0.0.0:12800"
mysqldump = "mysqldump"
```

### 1.3 构建并启动

```bash
cd /opt/go-mysqlbinlog-to-es/deploy
docker compose build
docker compose up -d
docker compose ps
docker compose logs -f go-mysqlbinlog-to-es
```

| 服务 | 宿主机端口 | 说明 |
|------|------------|------|
| Admin UI | `12802` | 配置界面 |
| Sync metrics /logs | `12801` → 容器 `12800` | Prometheus + 进程内日志 |

浏览器访问：`http://<服务器IP>:12802/`

### 1.4 目录与挂载说明

Compose 会挂载：

| 宿主机 | 容器 | 用途 |
|--------|------|------|
| `../configs` | `/app/configs` | `river.toml` 与备份 `*.bak.1~3` |
| `../var` | `/app/var` | 位点 `master.info` |
| `/var/run/docker.sock` | 同左（仅 admin） | 界面「重启同步容器 / 全量 dump」 |

防火墙放行（按需）：

```bash
# firewalld 示例
sudo firewall-cmd --permanent --add-port=12802/tcp
sudo firewall-cmd --permanent --add-port=12801/tcp
sudo firewall-cmd --reload

# ufw 示例
sudo ufw allow 12802/tcp
sudo ufw allow 12801/tcp
```

### 1.5 常用运维命令

```bash
cd /opt/go-mysqlbinlog-to-es/deploy

docker compose logs -f go-mysqlbinlog-to-es
docker compose restart go-mysqlbinlog-to-es
docker compose down
docker compose up -d --build

# 手动全量（或用 Admin 勾选「下次启动先全量 dump」）
docker compose stop go-mysqlbinlog-to-es
rm -f ../var/master.info
docker compose up -d go-mysqlbinlog-to-es
```

成功标志：

```text
dump MySQL and parse OK
start binlog replication at (...)
```

若出现 `skip dump`，说明仍有位点，属于增量续跑，不是失败。

### 1.6 Docker 部署注意

1. **必须在 `deploy/` 目录执行** `docker compose`，或使用 `-f deploy/docker-compose.yml`。  
2. sync 镜像基于 `mysql:5.7`，自带兼容旧 MySQL 的 `mysqldump`。  
3. `server_id` 勿与其它同步实例冲突。  
4. Admin 挂载了 docker.sock，仅内网访问 `12802`，勿裸奔公网。  
5. 生产建议先建好 ES index/mapping，再开同步（见 [configuration.md](./configuration.md)）。

---

## 二、系统源码编译（Ubuntu / Debian / CentOS）

适用于不想用 Docker、或希望进程直接跑在宿主机上的场景。

组件：

| 二进制 | 源码路径 | 作用 |
|--------|----------|------|
| `sync` | `./cmd/go-mysqlbinlog-to-es` | binlog/dump → ES |
| `admin` | `./cmd/admin` | Web 配置界面 |

### 2.1 安装依赖

#### Ubuntu / Debian

```bash
sudo apt-get update
sudo apt-get install -y git curl wget ca-certificates build-essential

# mysqldump：版本尽量与目标 MySQL 大版本一致
# MySQL 5.7 客户端示例（按你的源调整）：
# sudo apt-get install -y mysql-client-5.7
# 或通用：
sudo apt-get install -y default-mysql-client
# 验证
mysqldump --version
```

#### CentOS / RHEL / Rocky / Alma

```bash
sudo yum groupinstall -y "Development Tools"
sudo yum install -y git curl wget ca-certificates

# MySQL 客户端（按仓库选择其一）
# sudo yum install -y mysql
# 或 mysql-community-client
mysqldump --version
```

### 2.2 安装 Go（1.20+）

官方二进制安装（各发行版通用）：

```bash
GO_VER=1.20.14
cd /tmp
wget -q https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go${GO_VER}.linux-amd64.tar.gz

echo 'export PATH=/usr/local/go/bin:$PATH' | sudo tee /etc/profile.d/go.sh
source /etc/profile.d/go.sh
go version
```

国内可设置：

```bash
export GOPROXY=https://goproxy.cn,direct
export GO111MODULE=on
```

也可写入 `~/.bashrc` 持久化。

### 2.3 编译

```bash
cd /opt
git clone https://github.com/JasonYe88/go-mysqlbinlog-to-es.git
cd /opt/go-mysqlbinlog-to-es

export GOPROXY=https://goproxy.cn,direct
export GO111MODULE=on
export CGO_ENABLED=0

mkdir -p bin var
go build -mod=mod -o bin/sync ./cmd/go-mysqlbinlog-to-es
go build -mod=mod -o bin/admin ./cmd/admin

./bin/sync -h
./bin/admin -h
```

交叉编译示例（在别的机器上编好拷过去）：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=mod -o bin/sync ./cmd/go-mysqlbinlog-to-es
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=mod -o bin/admin ./cmd/admin
```

### 2.4 配置

```bash
mkdir -p /opt/go-mysqlbinlog-to-es/configs
mkdir -p /opt/go-mysqlbinlog-to-es/var
vi /opt/go-mysqlbinlog-to-es/configs/river.toml
```

宿主机运行时建议：

```toml
data_dir = "/opt/go-mysqlbinlog-to-es/var"
stat_addr = "0.0.0.0:12800"
mysqldump = "mysqldump"   # 或写绝对路径，如 /usr/bin/mysqldump
```

### 2.5 前台试跑

开两个终端，或用 `tmux`/`screen`：

```bash
cd /opt/go-mysqlbinlog-to-es

# 终端 1：同步
./bin/sync -config=./configs/river.toml

# 终端 2：Admin（无 Docker 时不要依赖 docker.sock 重启）
./bin/admin \
  -addr=:12802 \
  -config=./configs/river.toml \
  -static=./web/admin/static \
  -sync-log-url=http://127.0.0.1:12800/logs
```

说明：

- 不挂 docker.sock 时，Admin「重启同步容器」会不可用；可用 systemd 重启（见下）。  
- 「全量 dump」在无 Docker 时：停 sync → 删 `var/master.info` → 再启 sync。  
- 进程内日志仍可通过 Admin「查看日志」的 buffer 源读取（需 `-sync-log-url` 指向 sync 的 `/logs`）。

访问：`http://<服务器IP>:12802/`  
Metrics：`http://<服务器IP>:12800/metrics`（若 `stat_addr` 为 `0.0.0.0:12800`）

### 2.6 用 systemd 托管（推荐）

创建用户（可选）：

```bash
sudo useradd -r -s /sbin/nologin river 2>/dev/null || true
sudo chown -R river:river /opt/go-mysqlbinlog-to-es
```

#### sync 服务

`/etc/systemd/system/go-mysqlbinlog-to-es.service`：

```ini
[Unit]
Description=go-mysqlbinlog-to-es sync
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=river
Group=river
WorkingDirectory=/opt/go-mysqlbinlog-to-es
Environment=TZ=Asia/Shanghai
ExecStart=/opt/go-mysqlbinlog-to-es/bin/sync -config=/opt/go-mysqlbinlog-to-es/configs/river.toml
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

#### admin 服务

`/etc/systemd/system/go-mysqlbinlog-to-es-admin.service`：

```ini
[Unit]
Description=go-mysqlbinlog-to-es admin UI
After=network-online.target go-mysqlbinlog-to-es.service
Wants=network-online.target

[Service]
Type=simple
User=river
Group=river
WorkingDirectory=/opt/go-mysqlbinlog-to-es
Environment=TZ=Asia/Shanghai
ExecStart=/opt/go-mysqlbinlog-to-es/bin/admin -addr=:12802 -config=/opt/go-mysqlbinlog-to-es/configs/river.toml -static=/opt/go-mysqlbinlog-to-es/web/admin/static -sync-log-url=http://127.0.0.1:12800/logs
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now go-mysqlbinlog-to-es
sudo systemctl enable --now go-mysqlbinlog-to-es-admin

sudo systemctl status go-mysqlbinlog-to-es
sudo journalctl -u go-mysqlbinlog-to-es -f
```

#### 宿主机全量 dump

```bash
sudo systemctl stop go-mysqlbinlog-to-es
sudo rm -f /opt/go-mysqlbinlog-to-es/var/master.info
sudo systemctl start go-mysqlbinlog-to-es
sudo journalctl -u go-mysqlbinlog-to-es -n 50 --no-pager
```

### 2.7 各发行版差异速查

| 项目 | Ubuntu / Debian | CentOS / Rocky |
|------|-----------------|----------------|
| 包管理 | `apt-get` | `yum` / `dnf` |
| 编译工具 | `build-essential` | `"Development Tools"` |
| mysqldump | `default-mysql-client` 或官方 MySQL 源 | `mysql` / `mysql-community-client` |
| 防火墙 | `ufw` | `firewalld` |
| 服务管理 | `systemd`（同上） | `systemd`（同上） |
| SELinux | 一般可不关 | 若拒绝写 `var/`，需调上下文或临时 `permissive` 排查 |

SELinux 写位点目录示例（CentOS）：

```bash
sudo chcon -Rt var_t /opt/go-mysqlbinlog-to-es/var
# 或排查：sudo ausearch -m avc -ts recent
```

### 2.8 源码编译注意

1. **Go ≥ 1.20**（与 `go.mod` 一致）。  
2. **mysqldump 大版本**尽量匹配目标 MySQL，避免 TLS/协议报错。  
3. `data_dir` 需对运行用户可写。  
4. 升级时：停服务 → 覆盖二进制 → 保留 `configs/` 与 `var/` → 启动。  
5. 无 Docker 时 Admin 无法「一键重启容器」；用 `systemctl restart go-mysqlbinlog-to-es`。

---

## 三、安装后检查清单

1. [ ] `river.toml` 中 `[[source]].tables` 与每条 `[[rule]]` 一致  
2. [ ] MySQL 账号可 `SHOW MASTER STATUS` / 读业务表  
3. [ ] ES 可连通（curl 或 Admin 保存后观察日志）  
4. [ ] 首次建议全量：确认日志有 `dump MySQL and parse OK`  
5. [ ] Admin `12802`、metrics 端口已按安全策略放行  
6. [ ] 生产建议先建 ES mapping，再同步重要索引  

---

## 四、相关文档

| 文档 | 内容 |
|------|------|
| [deploy.md](./deploy.md) | Docker 排障、skip dump、mysqldump TLS |
| [configuration.md](./configuration.md) | `river.toml` 字段 |
| [admin-ui.md](./admin-ui.md) | 配置界面、全量 dump、原始 TOML |
| [architecture.md](./architecture.md) | 架构说明 |
