# Mod List Discovery

为 Minecraft 服务器准备统一的玩家 MOD 环境。服主在 Web 后台上传文件、搭配并启用环境版本；玩家在游戏内检查、下载和同步 MOD，重启后生效。

项目包含 **Fabric 客户端 MOD** 和 **Go 管理服务**。管理服务内嵌中文 Web UI，支持公开/私密服务器、邀请码、版本切换和 Local / S3 / OSS 文件存储。

## 下载与兼容性

在本仓库 **Releases** 页面下载正式版本；开发构建在 **Actions → Build and release → 对应运行 → Artifacts** 中下载。

| 文件 | 用途 |
| --- | --- |
| `mod-list-discovery-<版本>+mc26.2.jar` | 放入玩家游戏实例的 `mods/` |
| `mod-list-discovery-server-<版本>-linux-amd64` | Linux x86_64 管理服务，下载后需 `chmod +x` |
| `config.example.yaml` | 服务端配置模板 |
| `SHA256SUMS` | 下载校验清单 |

当前 JAR **面向 Minecraft 26.2，不是全版本通用**。要求 Java 25+、Fabric Loader 0.19.0+，并额外安装匹配游戏版本的 Fabric API。已测试的 Loader / API 构建版本分别为 0.19.5 / 0.155.2+26.2。声明的最低版本不代表验证过所有后续版本；不支持直接用于 Forge、NeoForge 或其他 Minecraft 版本。

## 快速开始

服主下载二进制与配置模板，先将二进制重命名为 `mod-list-discovery-server`，然后在该目录运行：

```sh
chmod +x mod-list-discovery-server
mkdir -p data
cp config.example.yaml data/config.yaml
./mod-list-discovery-server init --config data/config.yaml
./mod-list-discovery-server serve --config data/config.yaml
```

初始化会要求设置管理员密码。默认用户名 `admin`，没有默认密码。访问 `http://127.0.0.1:8080/admin/`，按页面引导操作：

1. **创建服务器**：填写玩家熟悉的名称，选择公开或私密。
2. **上传 MOD**：上传玩家客户端需要的 Fabric JAR。
3. **搭配环境版本**：按名称选择必装、自选和禁止规则，检查后封存。
4. **启用并分享**：将已封存版本启用到正式版 `stable`，复制地址发给玩家；私密服务器额外创建邀请码。

玩家安装本客户端和 Fabric API 后，从游戏标题界面打开 **Mod List Discovery**，添加服主提供的地址、检查并确认同步，然后重启。分享地址用于 MOD 同步，不是多人游戏服务器的 IP。

生产部署必须设置玩家可访问的 HTTPS 地址；反向代理、配置、备份和升级步骤见 [部署指南](docs/DEPLOYMENT.md)。

## 文档导航

| 你要做什么 | 文档 |
| --- | --- |
| 查看完整文档目录 | [文档首页](docs/README.md) |
| 搭建和维护服务 | [部署与配置](docs/DEPLOYMENT.md) |
| 学习后台字段和发布流程 | [服主入门指南](docs/ADMIN_GUIDE.md) |
| 安装客户端、同步和排障 | [玩家指南](docs/CLIENT_GUIDE.md) |
| 理解模块、签名和同步设计 | [技术架构](docs/ARCHITECTURE.md) |
| 对接接口 | [协议与 API](docs/API.md) |
| 修改代码和本地测试 | [开发指南](docs/DEVELOPMENT.md) |
| 使用 Actions 构建及标签发布 | [构建与发布](docs/RELEASING.md) |
| 了解覆盖范围与验证边界 | [测试与验收](docs/VALIDATION.md) |
| 查找 Fabric、Go、GitHub 等官方资料 | [参考资料](docs/REFERENCES.md) |

## 从源码构建

需要 Go 1.27、JDK 25、Python 3 和 Make；Gradle Wrapper 已包含。服务运行时无需 Python、Node.js 或单独的前端构建。

```sh
# JAVA_HOME 指向 JDK 25
make test
make package
```

本地版本默认读取 [VERSION](VERSION)，可使用 `make package VERSION=0.2.0` 覆盖。可发布文件在 `dist/release/`；`make server` 和 `make client` 仍可分别构建。

GitHub Actions 在分支推送、Pull Request 和手动运行时测试并上传产物；推送 `v0.2.0` 一类标签后自动发布 Release，`v0.2.0-rc.1` 自动标记为预发布。详见 [发布操作说明](docs/RELEASING.md)。

## 功能边界

工具不会自动安装 Minecraft、Java 或 Fabric Loader，不热加载 MOD，也不自动更新自身。它会在文件校验成功后再执行同步；未知冲突文件需要玩家确认处理。切换环境不等于恢复世界存档，建议在更新 MOD 前备份存档。只有获得分发授权的 MOD 才应上传和分享。
