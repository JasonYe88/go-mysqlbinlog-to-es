# JSON 字段映射指南

假设表结构：

```sql
CREATE TABLE `test_mysql_go_to_es` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `dataJson` json NOT NULL,
  PRIMARY KEY (`id`)
);
```

示例 JSON：

```json
{"sku":"123123","skuName":"Jig & Scroll Saws","suggestSellPrice":12.99}
```

---

## 模式 1 / 2：整列同步（推荐做动态字段）

```toml
[rule.field]
id = "id"
dataJson = "dataJson"
```

### ES 文档

```json
{
  "id": 1,
  "dataJson": {
    "sku": "123123",
    "skuName": "Jig & Scroll Saws",
    "suggestSellPrice": 12.99
  }
}
```

### 特点

- 推送结构与 MySQL 一致  
- JSON 内新增字段（如 `maintainerUserName`）会随整列进入 ES  
- 一般**不必**预先创建 ES mapping（可用动态 mapping）  
- 生产若要精确类型，仍建议手写 mapping  

---

## 模式 3：只推部分 JSON 字段

```toml
[rule.field]
id = "id"
"dataJson.sku" = "sku"
"dataJson.skuName" = "skuName"
```

### ES 文档

```json
{
  "id": 1,
  "sku": "123123",
  "skuName": "Jig & Scroll Saws"
}
```

### 特点

- 未映射的键（如 `suggestSellPrice`）不会进入 ES  
- 以后 JSON 新增键**不会自动出现**，需追加映射  
- 只有路径映射、没有 `dataJson = "dataJson"` 时，**不会**再写整段 `dataJson` 对象  

### TOML 注意

```toml
# ❌ 错误
dataJson.sku = "sku"

# ✅ 正确
"dataJson.sku" = "sku"
```

---

## 嵌套目标路径

```toml
"dataJson.sku" = "data.sku"
```

生成：

```json
{ "id": 1, "data": { "sku": "123123" } }
```

---

## 如何选择

| 需求 | 模式 |
|------|------|
| 和 MySQL 一样 / JSON 自动扩展 | 1/2 整列 |
| 只要白名单字段、扁平化到顶层 | 3 路径映射 |
| 既要整列又要额外拍平字段 | 同时配置 `dataJson = "dataJson"` 与路径映射 |
