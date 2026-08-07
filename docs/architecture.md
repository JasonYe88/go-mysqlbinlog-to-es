# 架构设计

## 1. 总体目标

`go-mysqlbinlog-to-es` 将 MySQL 表数据可靠地同步到 Elasticsearch：

1. 可选全量：`mysqldump`
2. 增量：伪装成从库消费 binlog（ROW + FULL image）
3. 按 Rule 转换字段（含 JSON 路径）
4. Bulk 写入 ES，位点落盘可断点续传
5. 提供 Web 界面维护 `river.toml`

## 2. 逻辑架构

```text
                 ┌──────────────────────┐
                 │  Admin Web (12802)   │
                 │  读库表 / 改映射 / 保存 │
                 └──────────┬───────────┘
                            │ 读写
                            ▼
                     configs/river.toml
                            │
                            ▼
┌──────────┐   binlog/dump   ┌─────────────────┐   Bulk   ┌──────────────┐
│  MySQL   │ ──────────────► │ Sync Daemon     │ ───────► │ Elasticsearch│
└──────────┘                 │ (go-mysqlbinlog │          └──────────────┘
                             │  -to-es)        │
                             │  metrics:12800  │
                             └────────┬────────┘
                                      │
                                      ▼
                                 var/master.info
```

## 3. 模块划分

| 模块 | 路径 | 职责 |
|------|------|------|
| Sync 入口 | `cmd/go-mysqlbinlog-to-es` | 加载配置、启动 River、信号处理 |
| Admin 入口 | `cmd/admin` | HTTP API + 静态资源；可选 Docker 重启 |
| Config / Rule | `river/config.go` `river/rule.go` | TOML 解析、表规则 |
| Field Path | `river/field_path.go` | JSON 路径解析、嵌套写入 |
| Sync Loop | `river/sync.go` | Insert/Update/Delete → BulkRequest |
| Orchestrator | `river/river.go` | Canal 初始化、source/rule 校验 |
| Position | `river/master.go` | binlog 位点持久化 |
| Metrics | `river/metrics.go` | Prometheus |
| ES Client | `elastic/` | HTTP Bulk |
| UI | `web/admin/static` | 可视化配置 + 日志面板 |
| Log Ring | `river/logring.go` | 进程内最近 N 行日志，供 `/logs` 查询 |

## 4. 同步数据流

```text
Canal OnRow
  → 查 Rule(schema.table)
  → makeInsert / makeUpdate / makeDelete
      ├─ 普通列映射（rename / list / date）
      ├─ JSON 路径映射（"col.path" → es.path）
      └─ 路径-only 列不写整段 JSON blob
  → syncCh 批量
  → elastic.Bulk
  → 定期 Save(master.info)
```

## 5. 设计约束（与 MySQL/ES 行为相关）

- binlog：`ROW` + `FULL`
- 表需有主键（或 `skip_no_pk_table`）
- `[[rule]]` 中的表必须出现在 `[[source]].tables`
- 带点的 TOML key 必须加引号：`"dataJson.sku" = "sku"`
- 存在路径映射时，Update 使用整文档 Index，避免 ES 浅合并破坏嵌套字段
- dump 客户端建议使用与 MySQL 大版本兼容的 `mysqldump`（见部署文档）

## 6. 扩展点

后续若要增强，优先落在：

1. `river/field_path.go`：更多 JSON 变换
2. `cmd/admin`：规则模板、多环境配置
3. `elastic/`：更高版本 ES API 适配
