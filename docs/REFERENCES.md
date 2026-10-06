# 官方资料与相关地址

[文档首页](README.md)

以下是工具和协议的官方入口，便于安装、开发和排障。第三方文档的最新示例可能面向不同版本；本项目具体依赖以仓库构建文件为准。

## Minecraft 与 Fabric

| 资料 | 用途 |
| --- | --- |
| [Fabric 安装器](https://fabricmc.net/use/installer/) | 为游戏实例安装 Fabric Loader |
| [Fabric 玩家文档](https://docs.fabricmc.net/players/) | Java、Loader、MOD 安装与常见问题 |
| [Fabric 开发文档](https://docs.fabricmc.net/develop/) | 项目结构、游戏 API 与构建开发 |
| [Fabric API 源码](https://github.com/FabricMC/fabric) | API 版本与上游实现 |
| [Fabric Loader 源码](https://github.com/FabricMC/fabric-loader) | MOD 加载器和依赖解析 |
| [Fabric Loom 源码](https://github.com/FabricMC/fabric-loom) | Gradle 开发与打包插件 |

## 构建与发布

| 资料 | 用途 |
| --- | --- |
| [Go 官方文档](https://go.dev/doc/) | Go 安装、模块和开发 |
| [Gradle Wrapper](https://docs.gradle.org/current/userguide/gradle_wrapper.html) | 固定 Gradle 版本与分发包校验 |
| [GitHub Actions 文档](https://docs.github.com/en/actions) | Workflow、runner、权限和触发事件 |
| [GitHub Releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases) | 标签、发布版本和下载附件 |
| [GitHub CLI：创建 Release](https://cli.github.com/manual/gh_release_create) | 草稿、标签验证和自动说明 |
| [GitHub CLI：修改 Release](https://cli.github.com/manual/gh_release_edit) | 草稿发布和预发布状态 |
| [setup-go](https://github.com/actions/setup-go) | 根据 go.mod 安装工具链 |
| [setup-java](https://github.com/actions/setup-java) | 安装 JDK |
| [Gradle Actions](https://github.com/gradle/actions) | Gradle 缓存与构建集成 |
| [upload-artifact](https://github.com/actions/upload-artifact) | 保存与下载 Actions 产物 |

## 存储与管理界面

| 资料 | 用途 |
| --- | --- |
| [SQLite 在线备份](https://www.sqlite.org/backup.html) | 一致性备份数据库 |
| [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2) | 本项目 S3 接口所用 SDK |
| [阿里云 OSS Go SDK](https://github.com/aliyun/aliyun-oss-go-sdk) | 本项目 OSS 接口所用 SDK |
| [Bootstrap 5.3 文档](https://getbootstrap.com/docs/5.3/getting-started/introduction/) | 后台样式与组件 |

本项目的安装步骤见 [部署指南](DEPLOYMENT.md) 和 [玩家指南](CLIENT_GUIDE.md)；协议和实现设计见 [技术架构](ARCHITECTURE.md) 与 [API](API.md)。
