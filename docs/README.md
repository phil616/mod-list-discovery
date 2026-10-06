# 文档中心

[返回项目首页](../README.md)

按角色阅读：

| 读者 | 推荐顺序 |
| --- | --- |
| 首次部署的服主 | [部署配置](DEPLOYMENT.md) → [后台使用](ADMIN_GUIDE.md) → [玩家指南](CLIENT_GUIDE.md) |
| 加入服务器的玩家 | [安装与同步](CLIENT_GUIDE.md) |
| 开发者 | [本地开发](DEVELOPMENT.md) → [技术架构](ARCHITECTURE.md) → [协议 API](API.md) |
| 发布维护者 | [GitHub Actions 与 Release](RELEASING.md) → [测试与验收](VALIDATION.md) |
| 查阅资料 | [官方参考资料](REFERENCES.md) → [技术架构](ARCHITECTURE.md) |

## 文档维护约定

- README 是下载与上手入口，详细说明按使用、部署、技术、发布分开维护。
- 当前实现的构建约束以 `server/go.mod`、`client/build.gradle`、`fabric.mod.json` 为准；技术文档描述当前实现，变更时与代码同步维护。
- 服务端配置变更同步更新 [配置模板](../config.example.yaml) 与部署指南；接口变更同步更新 API 文档和互通测试。
- 发布版本来自根目录 `VERSION` 或 `v` 标签。Minecraft 兼容性是独立维度，改变项目版本号不会自动适配新游戏版本。
- 调试截图不嵌入公开文档，验收记录以步骤、测试范围和限制为主；浏览器脚本将截图输出到被忽略的 `.tools/ui-smoke/`。
- 本地链接检查：`python3 tools/check-docs.py`。CI 同样执行此检查；它不验证外部网站可用性或 Markdown 锚点。
