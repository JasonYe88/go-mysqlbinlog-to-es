# 开源归属与许可证

## 许可证

本项目采用 **MIT License**。

## 上游项目

核心同步能力源自：

- 仓库：[siddontang/go-mysql-elasticsearch](https://github.com/siddontang/go-mysql-elasticsearch)
- 许可证：MIT
- 版权：Copyright (c) 2015 siddontang

本仓库在 LICENSE 中保留上述版权声明，并追加本项目贡献者版权。

## 本项目相对上游的主要变更

1. 工程重命名为 `go-mysqlbinlog-to-es`，模块路径为 `github.com/JasonYe88/go-mysqlbinlog-to-es`  
   仓库：https://github.com/JasonYe88/go-mysqlbinlog-to-es  

2. 增加 JSON 路径字段映射与相关单测  
3. 增加 Web 配置管理界面（Admin）  
4. 补充 Docker 部署（兼容旧 MySQL 的 mysqldump 镜像方案）  
5. 完善中文文档  

## 使用与再分发建议

你可以：

- 复制、修改、商用、再发布  

但必须：

- 保留 MIT 许可证与原作者版权声明  
- 不建议宣称“完全从零原创且与上游无关”

## 依赖致谢

同步链路还依赖（不完全列表）：

- `github.com/siddontang/go-mysql`（canal / binlog）
- `github.com/BurntSushi/toml`
- `github.com/prometheus/client_golang`
- `github.com/go-sql-driver/mysql`（Admin 读库结构）

请遵循各依赖自身许可证。
