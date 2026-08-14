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

## 同步异常中断处理

### 全量同步中断

全量同步（mysqldump）过程中若进程异常终止：

| 情况 | 表现 | 处理方式 |
|------|------|----------|
| 进程崩溃/被 kill | 无 master.info，下次从头开始 | 重新启动即可 |
| 手动 Ctrl+C | 无 master.info，下次从头开始 | 重新启动即可 |
| 机器重启 | 无 master.info，下次从头开始 | 重新启动即可 |

**原因**：全量 dump 阶段尚未获得 binlog position，因此不会写入 `master.info`。进度保存在内存中，中断后会丢失。

**Docker 模式**：

```bash
# 判断是否完成了全量
cat ../var/master.info

# 无内容 → 未完成全量，需要重新启动
docker compose restart go-mysqlbinlog-to-es
```

**源码安装模式（Debian）**：

```bash
# 判断是否完成了全量
cat /app/var/master.info

# 无内容 → 未完成全量，需要重新启动
./go-mysqlbinlog-to-es -config=configs/river.toml > logs/sync.log 2>&1 &
```

---

### 增量同步中断

增量同步（binlog 订阅）过程中若进程异常终止：

| 情况 | 表现 | 处理方式 |
|------|------|----------|
| 进程崩溃/被 kill | master.info 有位点，从断点继续 | 直接重新启动 |
| 手动重启 | master.info 有位点，从断点继续 | 直接重新启动 |
| Docker 容器重启 | master.info 有位点，从断点继续 | docker compose restart |
| binlog 被清理 | 报错 "could not find first log file" | 见下方"binlog 丢失" |

**Docker 模式**：

```bash
# 增量同步中断后，直接重启即可
docker compose restart go-mysqlbinlog-to-es

# 观察日志，确认从断点继续
docker compose logs -f go-mysqlbinlog-to-es | grep binlog
```

**源码安装模式（Debian）**：

```bash
# 增量同步中断后，直接重启即可
pkill -f go-mysqlbinlog-to-es || true
nohup ./go-mysqlbinlog-to-es -config=configs/river.toml > logs/sync.log 2>&1 &

# 观察日志，确认从断点继续
tail -f logs/sync.log | grep binlog
```

---

### binlog 被清理（最严重情况）

如果 `master.info` 记录的 binlog 文件已被 MySQL 清理，会报错：

```
binlog position is not available, you may need to specify a new binlog name and position
```

**处理方式**：必须重新全量同步

**Docker 模式**：

```bash
# 方法 1：通过 Admin UI（推荐）
# 访问 http://127.0.0.1:12802/ → 点击「重启同步容器」→ 勾选「全量 dump」→ 确认

# 方法 2：手动操作
docker compose stop go-mysqlbinlog-to-es
rm -f ../var/master.info
docker compose up -d go-mysqlbinlog-to-es
```

**源码安装模式（Debian）**：

```bash
# 方法 1：通过 Admin UI（推荐）
# 访问 http://127.0.0.1:12802/ → 点击「重启同步服务」→ 勾选「全量 dump」→ 确认

# 方法 2：手动操作
pkill -f go-mysqlbinlog-to-es || true
rm -f /app/var/master.info
nohup ./go-mysqlbinlog-to-es -config=configs/river.toml > logs/sync.log 2>&1 &
```

---

### 增量同步延迟过大

如果观察到 `es_ok_docs` 增长缓慢，可能是：

1. **ES 写入瓶颈**：增大 `bulk_size`
2. **MySQL binlog 读取慢**：检查网络延迟
3. **大表全量占用带宽**：分时段同步

```toml
# 在 river.toml 中调整
bulk_size = 500        # 每批数量（可增大到 1000-2000）
flush_bulk_time = "2s" # 批量超时（可缩短到 1s）
```

## 重新全量同步

**Docker 模式**：

```bash
cd deploy
docker compose stop go-mysqlbinlog-to-es
rm -f ../var/master.info
docker compose up -d go-mysqlbinlog-to-es
docker logs -f go-mysqlbinlog-to-es
```

**源码安装模式（Debian）**：

```bash
pkill -f go-mysqlbinlog-to-es || true
rm -f /app/var/master.info
nohup ./go-mysqlbinlog-to-es -config=configs/river.toml > logs/sync.log 2>&1 &
tail -f logs/sync.log
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

### 7. MySQL 表新增字段

#### 问题根源

同步进程启动时会缓存表结构到 `rule.TableInfo`。binlog 事件中的数据（`e.Rows`）使用当前 MySQL 表结构，但映射时用的是缓存的旧结构。

**如果新增字段时未按正确顺序操作**：
- 新字段数据比旧结构多
- `values[idx]` 访问错误索引
- 可能导致解析崩溃或数据错位

#### 正确操作顺序

| 顺序 | 操作 | 说明 |
|------|------|------|
| 1 | **停止同步进程** | 防止处理不完整的 binlog 事件 |
| 2 | 修改 `river.toml` | 添加 `[rule.field]` 映射（或加入 `filter` 白名单） |
| 3 | MySQL 执行 `ALTER TABLE` | 添加新字段 |
| 4 | 重启同步进程 | 加载新表结构到 `rule.TableInfo` |

#### 操作示例

**Docker 模式**：

```bash
# 1. 停止同步
docker compose stop go-mysqlbinlog-to-es

# 2. 修改 configs/river.toml，添加字段映射
# [[rule]]
# [rule.field]
# new_field = "new_field"

# 3. MySQL 添加字段（通常由 DBA 执行）
ALTER TABLE test_table ADD COLUMN new_field VARCHAR(64) NOT NULL DEFAULT '';

# 4. 重启同步
docker compose up -d go-mysqlbinlog-to-es
```

**源码模式（Debian）**：

```bash
# 1. 停止同步
pkill -f go-mysqlbinlog-to-es || true

# 2. 修改 configs/river.toml，添加字段映射
# [[rule]]
# [rule.field]
# new_field = "new_field"

# 3. MySQL 添加字段（通常由 DBA 执行）
ALTER TABLE test_table ADD COLUMN new_field VARCHAR(64) NOT NULL DEFAULT '';

# 4. 重启同步
nohup ./go-mysqlbinlog-to-es -config=configs/river.toml > logs/sync.log 2>&1 &
```

#### ES 索引 mapping（如果需要）

```bash
# 先添加 ES mapping（如果字段类型特殊）
curl -X PUT "localhost:9200/test_index/_mapping?pretty" \
  -u elastic:password \
  -H "Content-Type: application/json" \
  -d '{"properties":{"new_field":{"type":"keyword"}}}'
```

#### 故障处理

| 故障情况 | 原因 | 解决方法 |
|----------|------|----------|
| 进程崩溃 | 添加字段时进程未停止，结构不匹配 | 按正确顺序重做 |
| 数据错位 | `values[idx]` 索引对应错误 | 删除 `master.info` 重新全量同步 |
| ES 写入失败 | ES strict mapping 拒绝新字段 | 先添加 ES mapping |

**紧急回退**：

```bash
# 方法 1：恢复位点（不重新全量，新字段历史数据丢失）
mv /app/var/master.info.bak /app/var/master.info 2>/dev/null || true
nohup ./go-mysqlbinlog-to-es -config=configs/river.toml > logs/sync.log 2>&1 &

# 方法 2：强制重新全量（数据完整，但耗时较长）
rm -f /app/var/master.info
nohup ./go-mysqlbinlog-to-es -config=configs/river.toml > logs/sync.log 2>&1 &
```

> **重要**：建议在操作前备份 `master.info`，便于回退。

## 生产建议

1. `server_id` 全局唯一  
2. 重要 index 先建 mapping，再开同步  
3. 大表全量避开业务高峰  
4. 定期备份 `master.info`  
5. 用 metrics 观察 `mysql2es_canal_state` / delay / insert 计数
