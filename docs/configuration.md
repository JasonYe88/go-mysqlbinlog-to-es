# 配置说明（标准模板解读）

配置文件路径：`configs/river.toml`（运行时）  
仓库内脱敏模板：[`configs/river.toml.example`](../configs/river.toml.example)

```bash
cp configs/river.toml.example configs/river.toml
# 再编辑真实地址与密码
```

> **安全提示**：`my_pass` / `es_pass` 为明文；请勿把含真实密码的 `river.toml` 提交到 Git。仓库已建议忽略该文件，对外只保留 `.example`。

---

## 1. 配置结构总览

```text
river.toml
├── 全局连接与运行参数
│   ├── MySQL（my_* / server_id / flavor / mysqldump）
│   ├── Elasticsearch（es_*）
│   └── 位点与监控（data_dir / stat_*）
├── [[source]] × N     # 关注哪些库表（dump + binlog）
└── [[rule]] × N       # 每张表如何写入 ES
    └── [rule.field]   # 列 / JSON 路径 → ES 字段
```

约束：**每个 `[[rule]]` 的 `schema.table` 必须出现在某个 `[[source]].tables` 中**，否则启动报错  
`rule xxx not defined in source`。

---

## 2. 全局参数

| 字段 | 必填 | 说明 | 示例 / 建议 |
|------|------|------|-------------|
| `server_id` | 是 | 伪从库 ID，全局唯一且 > 0 | `1001`（勿与其它同步实例冲突） |
| `my_addr` | 是 | MySQL `host:port` | `192.168.1.10:3306` |
| `my_user` / `my_pass` | 是 | 需复制权限 + 读业务表 | 使用专用账号，勿用弱口令上生产 |
| `my_charset` | 否 | 连接字符集 | `utf8mb4` |
| `es_addr` | 是 | ES `host:port`，**不要**带 `http://` | `es.example.com:9200` |
| `es_user` / `es_pass` | 视集群 | ES 认证 | 无认证可留空（视集群策略） |
| `es_https` | 否 | 是否 HTTPS | `true` / `false` |
| `data_dir` | 是 | 位点目录，生成 `master.info` | Docker：`/app/var`；宿主机：绝对路径 |
| `stat_addr` | 建议 | metrics 监听 | Docker：`0.0.0.0:12800` |
| `stat_path` | 建议 | metrics 路径 | `/metrics` |
| `flavor` | 是 | `mysql` 或 `mariadb` | `mysql` |
| `mysqldump` | 建议 | 全量工具；`""` 跳过 dump | `mysqldump` |
| `skip_master_data` | 否 | 无 `--master-data` 权限时 | `true` |
| `bulk_size` | 否 | Bulk 条数；`0`=默认 128 | `128` |
| `flush_bulk_time` | 否 | 批量刷新超时；`0`=默认 200ms | `"200ms"` |
| `skip_no_pk_table` | 否 | 跳过无主键表 | `false` |

### 与本项目测试环境对应关系（已脱敏）

| 项 | 模板占位 | 含义 |
|----|----------|------|
| MySQL | `CHANGE_ME_MYSQL_PASSWORD` | 勿把真实密码写进 Git |
| ES | `CHANGE_ME_ES_PASSWORD` | 同上 |
| 示例库表 | `fms.test_mysql_go_to_es` | 可按业务替换 |
| 示例索引 | `test_mysql_go_to_es` | 与 `[[rule]].index` 一致 |

---

## 3. `[[source]]`：同步范围

```toml
[[source]]
schema = "fms"
tables = ["test_mysql_go_to_es"]
```

| 字段 | 说明 |
|------|------|
| `schema` | MySQL 库名 |
| `tables` | 表名列表；支持上游风格正则/通配 |

作用：

1. **全量 dump** 导出这些表  
2. **binlog** 只处理这些表的变更  

---

## 4. `[[rule]]`：表 → ES

```toml
[[rule]]
schema = "fms"
table = "test_mysql_go_to_es"
index = "test_mysql_go_to_es"
type = "_doc"
```

| 字段 | 说明 |
|------|------|
| `schema` / `table` | 必须与 source 对应 |
| `index` | ES 索引名 |
| `type` | ES7+ 常用 `_doc` |
| `filter` | 可选，列白名单 |
| `id` | 可选，自定义文档 `_id` 列；默认主键 |
| `pipeline` | 可选，ES ingest pipeline |
| `[rule.field]` | 字段映射，见下节 |

---

## 5. `[rule.field]`：字段映射

### 5.1 语法

```toml
[rule.field]
# 普通列
id = "id"
title = "es_title"

# 修饰符：list=按逗号拆数组；date=按日期解析
tags = "tags,list"
created_at = ",date"

# JSON 路径（左边带点必须加引号）
"dataJson.sku" = "sku"           # 拍平到顶层 sku
"dataJson.sku" = "data.sku"      # 嵌套到 data.sku
dataJson = "dataJson"            # 整列 JSON 对象写入
```

### 5.2 与示例模板一致的映射

| MySQL 源 | ES 目标 | 含义 |
|----------|---------|------|
| `id` | `id` | 主键进文档 |
| `dataJson.sku` | `data.sku` | JSON 内 sku → 嵌套字段 |
| `dataJson.skuName` | `skuName` | JSON 内 skuName → 顶层 |

对应写入 ES 的文档形态大致为：

```json
{
  "id": 1,
  "skuName": "...",
  "data": { "sku": "..." }
}
```

未出现在 `field` 中的 JSON 键（如仅路径映射模式）**不会**推送。整列模式见 [json-mapping.md](./json-mapping.md)。

---

## 6. 推荐落地流程

1. 复制模板：`cp configs/river.toml.example configs/river.toml`  
2. 填写 MySQL / ES 真实连接与密码  
3. 配置 `[[source]]` + `[[rule]]` + `[rule.field]`  
4. （建议）先在 ES 建好 index/mapping  
5. 启动同步；首次全量确认日志：`dump MySQL and parse OK`  
6. 改配置后重启进程；Admin 可勾选「下次启动先全量 dump」  

---

## 7. 重新全量

1. 停止同步进程  
2. 删除 `data_dir/master.info`  
3. 确保 `mysqldump` 可用且版本兼容  
4. 启动，日志应出现 `dump MySQL and parse OK`  

有位点时重启会出现 `skip dump`（增量续跑），属正常。

---

## 8. 相关文档

| 文档 | 内容 |
|------|------|
| [river.toml.example](../configs/river.toml.example) | 脱敏标准模板（含注释） |
| [json-mapping.md](./json-mapping.md) | JSON 三种同步模式 |
| [admin-ui.md](./admin-ui.md) | 界面改配置 / 全量 dump |
| [install.md](./install.md) | 服务器安装 |
| [deploy.md](./deploy.md) | 部署排障 |
