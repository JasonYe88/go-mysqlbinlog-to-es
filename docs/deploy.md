# 部署与排障

完整安装步骤（Docker / Ubuntu / Debian / CentOS 源码编译与 systemd）见 **[install.md](./install.md)**。

## Docker Compose（推荐）

```bash
cd deploy
# 先改好 ../configs/river.toml
docker compose build
docker compose up -d
docker compose logs -f go-mysqlbinlog-to-es
```

端口：

| 端口 | 用途 |
|------|------|
| 12802 | Admin UI |
| 12801 | Sync metrics（容器内 12800） |

配置目录挂载为整个 `../configs`（不是单文件），以便 Admin 保存时在同目录写入：

- `river.toml.bak.1`：最近一次保存前的版本  
- `river.toml.bak.2` / `.bak.3`：更早的版本（最多 3 个）  

Admin 与 sync 均挂载 `../var`，以便界面勾选「全量 dump」时删除共享的 `master.info`。

## 重新全量同步

```bash
cd deploy
docker compose stop go-mysqlbinlog-to-es
rm -f ../var/master.info
docker compose up -d go-mysqlbinlog-to-es
docker logs -f go-mysqlbinlog-to-es
```

成功标志：

```text
dump MySQL and parse OK
start binlog replication at (...)
```

## 常见问题

### 1. `exec: "mysqldump": executable file not found`

镜像未包含 mysqldump。本仓库 `Dockerfile.sync` 基于 `mysql:5.7`，已自带客户端。

### 2. `TLS/SSL error: unsupported protocol`

Alpine/MariaDB 客户端连旧 MySQL 常见问题。请使用本仓库提供的 `mysql:5.7` 运行镜像，不要用纯 Alpine + `apk add mysql-client`。

### 3. `rule fms, xxx not defined in source`

`[[rule]]` 有表，但 `[[source]].tables` 没有。两边必须一致。

### 4. `bare keys cannot contain '.'`

```toml
# 错误
dataJson.sku = "sku"
# 正确
"dataJson.sku" = "sku"
```

### 5. 没有全量，直接 skip dump

检查：

- `mysqldump` 是否配置  
- `master.info` 是否仍在（有位点会跳过 dump）  
- 日志是否 `skip dump, use last binlog...`

### 6. 只想同步一张测试表

```toml
[[source]]
schema = "fms"
tables = ["test_mysql_go_to_es"]
```

并删除其它 `[[rule]]`。这**不会**影响该表的 binlog 同步，只是不再同步其它表。

## 生产建议

1. `server_id` 全局唯一  
2. 重要 index 先建 mapping，再开同步  
3. 大表全量避开业务高峰  
4. 定期备份 `master.info`  
5. 用 metrics 观察 `mysql2es_canal_state` / delay / insert 计数
