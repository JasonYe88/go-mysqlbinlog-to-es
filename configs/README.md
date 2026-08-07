# configs

| 文件 | 用途 | 是否可提交 Git |
|------|------|----------------|
| [`river.toml.example`](./river.toml.example) | **标准配置模板**（脱敏 + 注释） | ✅ 提交 |
| `river.toml` | 本机/服务器真实运行配置（含密码） | ❌ 勿提交 |

```bash
cp configs/river.toml.example configs/river.toml
vi configs/river.toml   # 填写真实 my_pass / es_pass 等
```

字段说明见 [docs/configuration.md](../docs/configuration.md)。
