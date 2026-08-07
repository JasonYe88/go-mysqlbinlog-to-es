# go-mysqlbinlog-to-es

MySQL binlog → Elasticsearch 实时同步工具（含 Web 配置界面）。

**仓库：** [https://github.com/JasonYe88/go-mysqlbinlog-to-es](https://github.com/JasonYe88/go-mysqlbinlog-to-es)

```bash
git clone https://github.com/JasonYe88/go-mysqlbinlog-to-es.git
cd go-mysqlbinlog-to-es
```

> 本项目基于 [siddontang/go-mysql-elasticsearch](https://github.com/siddontang/go-mysql-elasticsearch)（MIT）二次开发与重组，保留原作者版权声明，详见 [LICENSE](./LICENSE)。

## 功能特性

- MySQL 全量 `mysqldump` + 增量 binlog 同步到 Elasticsearch
- 表级 / 字段级映射（`river.toml`）
- **JSON 路径映射**：`"dataJson.sku" = "sku"` 或 `"dataJson.sku" = "data.sku"`
- 整列 JSON 同步（结构与 MySQL 一致，新字段可动态进入 ES）
- Web 配置界面：读 MySQL 表结构、抽样 JSON 键、保存配置、重启同步、全量 dump
- **同步日志可视化**：进程内环形缓冲 + Docker 容器日志
- Prometheus metrics

## 快速开始

### Docker（推荐）

```bash
cp configs/river.toml.example configs/river.toml
# 编辑 configs/river.toml，填写真实 MySQL/ES 密码与库表
cd deploy
docker compose build
docker compose up -d
```

标准配置说明：[docs/configuration.md](./docs/configuration.md) · 脱敏模板：[configs/river.toml.example](./configs/river.toml.example)


| 服务 | 地址 |
|------|------|
| 配置界面 | http://127.0.0.1:12802/ |
| Metrics | http://127.0.0.1:12801/metrics |

### 本地编译

```bash
go build -mod=mod -o bin/sync ./cmd/go-mysqlbinlog-to-es
go build -mod=mod -o bin/admin ./cmd/admin

./bin/sync -config=./configs/river.toml
./bin/admin -config=./configs/river.toml -static=./web/admin/static
```

## 文档索引

| 文档 | 说明 |
|------|------|
| [docs/install.md](./docs/install.md) | 服务器安装：Docker / Ubuntu / Debian / CentOS |
| [docs/architecture.md](./docs/architecture.md) | 架构设计 |
| [docs/configuration.md](./docs/configuration.md) | 配置说明 |
| [docs/json-mapping.md](./docs/json-mapping.md) | JSON 映射三种模式 |
| [docs/admin-ui.md](./docs/admin-ui.md) | 配置界面使用 |
| [docs/deploy.md](./docs/deploy.md) | 部署与排障 |
| [docs/attribution.md](./docs/attribution.md) | 开源归属与许可证 |

## License

MIT。请保留原作者 `siddontang` 版权声明。
