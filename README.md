# go-mysqlbinlog-to-es

**MySQL → Elasticsearch 实时同步工具**（全量 dump + binlog 增量，附带 Web 配置界面）。

仓库：[https://github.com/JasonYe88/go-mysqlbinlog-to-es](https://github.com/JasonYe88/go-mysqlbinlog-to-es)

```bash
git clone https://github.com/JasonYe88/go-mysqlbinlog-to-es.git
cd go-mysqlbinlog-to-es
```

基于 [siddontang/go-mysql-elasticsearch](https://github.com/siddontang/go-mysql-elasticsearch)（MIT）二次开发，保留上游版权，详见 [LICENSE](./LICENSE) 与 [docs/attribution.md](./docs/attribution.md)。

---

## 它能做什么

把 MySQL 表数据同步到 Elasticsearch，适合检索、报表、业务读库等场景：

1. **全量**：`mysqldump` 导入历史数据  
2. **增量**：订阅 binlog，实时同步 INSERT / UPDATE / DELETE  
3. **可视化配置**：浏览器里选库表、配字段映射、保存 `river.toml`、重启同步、触发全量  

特别强化了 **JSON 列** 的用法：可整列同步，也可只抽路径（如 `dataJson.sku → data.sku`）。

---

## 功能一览

| 能力 | 说明 |
|------|------|
| 全量 + 增量 | dump 建底，binlog 追新 |
| 表 / 字段映射 | `river.toml` 的 `[[source]]` / `[[rule]]` / `[rule.field]` |
| JSON 路径映射 | `"dataJson.sku" = "sku"` 或 `"dataJson.sku" = "data.sku"` |
| Web Admin | 读表结构、抽样 JSON 键、批量粘贴映射、原始 TOML、备份回退 |
| 全量 dump 开关 | 界面勾选后「先停 → 删位点 → 再启」，避免位点被写回 |
| 日志可视化 | 进程内缓冲 + Docker 日志，可过滤 |
| Metrics | Prometheus（默认宿主机 `12801`） |

---

## 5 分钟快速开始（Docker）

**前提：** 本机已装 Docker Compose；MySQL 已开 binlog（`ROW`/`MIXED`）；能访问 ES。

```bash
# 1. 从脱敏模板生成配置（勿把真实密码提交到 Git）
cp configs/river.toml.example configs/river.toml
vi configs/river.toml   # 改 my_* / es_* / server_id / source / rule

# 2. 启动
cd deploy
docker compose build
docker compose up -d

# 3. 看日志（首次应出现 dump MySQL and parse OK）
docker compose logs -f go-mysqlbinlog-to-es
```

| 入口 | 地址 |
|------|------|
| 配置界面 Admin | http://127.0.0.1:12802/ |
| Metrics / 进程日志 API | http://127.0.0.1:12801/metrics |

更完整的 Ubuntu / Debian / CentOS 源码安装见 [docs/install.md](./docs/install.md)。

---

## 配置长什么样

最小示意（密码请用真实值，完整注释模板见 [`configs/river.toml.example`](./configs/river.toml.example)）：

```toml
server_id = 1001
my_addr = "127.0.0.1:3306"
my_user = "repl_user"
my_pass = "CHANGE_ME"
es_addr = "127.0.0.1:9200"
es_user = "elastic"
es_pass = "CHANGE_ME"
data_dir = "/app/var"
flavor = "mysql"
mysqldump = "mysqldump"

[[source]]
schema = "fms"
tables = ["test_mysql_go_to_es"]

[[rule]]
schema = "fms"
table = "test_mysql_go_to_es"
index = "test_mysql_go_to_es"
type = "_doc"
[rule.field]
id = "id"
"dataJson.sku" = "data.sku"
"dataJson.skuName" = "skuName"
```

说明文档：[docs/configuration.md](./docs/configuration.md) · JSON 三种模式：[docs/json-mapping.md](./docs/json-mapping.md)

---

## 架构简图

```text
MySQL (binlog / mysqldump)
        │
        ▼
  sync 进程 (canal + bulk)
        │
        ▼
  Elasticsearch
        ▲
        │ 读写 river.toml / 重启 / 日志
  Admin UI (:12802)
```

目录要点：

```text
cmd/go-mysqlbinlog-to-es   # 同步主程序
cmd/admin                  # 配置界面 API
river/                     # 规则、位点、同步核心
web/admin/static           # 前端页面
configs/                   # river.toml.example（模板）
deploy/                    # Docker Compose
docs/                      # 中文文档
```

---

## 文档导航

| 文档 | 适合谁 |
|------|--------|
| [docs/install.md](./docs/install.md) | 第一次上服务器（Docker / 源码 / systemd） |
| [docs/configuration.md](./docs/configuration.md) | 写 `river.toml` |
| [docs/json-mapping.md](./docs/json-mapping.md) | JSON 列怎么映射 |
| [docs/admin-ui.md](./docs/admin-ui.md) | 用网页改配置、全量 dump、日志 |
| [docs/deploy.md](./docs/deploy.md) | 排障（skip dump、mysqldump TLS 等） |
| [docs/architecture.md](./docs/architecture.md) | 想看内部结构 |
| [docs/attribution.md](./docs/attribution.md) | 开源归属 |

---

## 使用注意

1. `server_id` 必须唯一，且不要和其他同步工具冲突  
2. 每个 `[[rule]]` 的表必须写进某个 `[[source]].tables`  
3. 含密码的 `configs/river.toml` 不要提交仓库（已在 `.gitignore`）  
4. 生产重要索引建议先建 ES mapping，再开同步  
5. Admin「重启同步容器」依赖挂载 `docker.sock`；纯二进制部署请用 systemd 重启  

---

## License

[MIT](./LICENSE)。使用与再分发时请保留原作者 `siddontang` 及本项目版权声明。
