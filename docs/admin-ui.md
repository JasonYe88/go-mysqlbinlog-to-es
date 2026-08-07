# 配置界面（Admin UI）

## 访问地址

默认：http://127.0.0.1:12802/

Metrics（不是配置页）：http://127.0.0.1:12801/metrics

## 能力

1. 编辑 MySQL / ES 连接信息（左右分栏）  
2. 管理多条 `[[rule]]`（新增 / 删除表规则）  
3. 从 MySQL 读取库、表、列  
4. 对 JSON 列抽样子路径，一键加入映射  
5. `[rule.field]` 表格编辑 + **批量粘贴**（合并进当前规则）  
6. **原始 TOML** 页签：粘贴/编辑整份 `river.toml`，保存为最新文件  
7. 保存前自动备份（最多 3 个版本：`river.toml.bak.1`~`.bak.3`），可一键回退  
8. 一键重启同步容器；可选 **下次启动先全量 dump**（删除 `data_dir/master.info`）  
9. 查看同步日志（Docker / 进程缓冲）  

## 推荐操作流程

1. 打开 Admin  
2. 「刷新库表」→ 选择库表 →「加载字段」  
3. 「应用到当前规则」  
4. 点列/JSON chip，或批量粘贴映射  
5. 「保存到 river.toml」（或切到「原始 TOML」保存原文）  
6. 需要全量时勾选「下次启动先全量 dump」，再「重启同步容器」  

## 全量 dump

- 有 `data_dir/master.info` 时，普通重启会跳过 dump，从 binlog 继续。  
- 勾选「下次启动先全量 dump」后点重启：Admin 会 **先停容器 → 再删 master.info → 再启动**（避免进程退出时把位点写回）。  
- 日志应出现 `dump MySQL and parse OK`，而不是 `skip dump`。  
- Compose 需同时挂载 `../var` 到 admin 与 sync。  
- 若 ES 索引尚不存在，先全量 dump（或先建 index）；仅追 binlog 时遇到 delete 会报 `index_not_found`。  

## 本地启动

```bash
go build -o bin/admin ./cmd/admin
./bin/admin \
  -addr=:12802 \
  -config=./configs/river.toml \
  -static=./web/admin/static \
  -restart-container=go-mysqlbinlog-to-es
```

## API 一览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 健康检查 |
| GET/PUT | `/api/config` | 读写完整配置（PUT 时先备份再写入） |
| GET/PUT | `/api/config/raw` | 读写 TOML 原文；PUT body：`{"content":"..."}` |
| GET | `/api/config/backups` | 列出可用备份（最多 3 个） |
| POST | `/api/config/restore` | 回退：`{"slot":1}`，1=最近 |
| GET | `/api/sync-state` | 位点状态（是否存在 `master.info`） |
| GET | `/api/schemas` | 列出业务库 |
| GET | `/api/tables?schema=` | 列出表 |
| GET | `/api/columns?schema=&table=` | 列信息 + JSON 抽样 |
| GET | `/api/json-keys?schema=&table=&column=` | 指定列 JSON 键 |
| POST | `/api/restart` | 重启；可选 `{"full_dump":true}` 先删位点 |

## 同步日志可视化

配置页顶栏有 **查看日志** 按钮，打开：

- 地址：http://127.0.0.1:12802/logs.html

| 来源 | 说明 |
|------|------|
| 进程缓冲 (buffer) | 同步进程内存环形缓冲 |
| Docker 日志 (docker) | 通过 Docker API 读取容器 stdout/stderr |

```http
GET /api/logs?source=buffer&tail=300&filter=error
GET /api/logs?source=docker&tail=300&filter=dump
```
