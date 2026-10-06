# 开发指南

[文档首页](README.md) · [架构](ARCHITECTURE.md) · [测试](VALIDATION.md) · [发布](RELEASING.md)

## 工具链

| 工具 | 本项目基线 |
| --- | --- |
| Go | 由 `server/go.mod` 声明，目前 1.27.0 |
| JDK | 25，设置 `JAVA_HOME` |
| Gradle | Wrapper 固定 9.4.0，带分发包 SHA256 校验 |
| Fabric Loom | 1.16.3 |
| Minecraft / Loader / API | 26.2 / 0.19.5 / 0.155.2+26.2 |
| 辅助工具 | Make、Python 3；浏览器验收额外使用 Node 22+、Chrome |

官方安装入口见 [参考资料](REFERENCES.md)。不需要全局安装 Gradle。普通编译不需要 Node.js；前端是内嵌的 HTML、CSS、原生 JavaScript 和本地 Bootstrap。

## 常用命令

在仓库根目录执行：

```sh
make test                 # 发布脚本测试、Go 检查、JUnit、真实 Go/Java 互通
make build                # Go Linux amd64 与客户端生产 JAR
make package              # 构建后筛选发布附件，生成 SHA256SUMS
make package VERSION=0.2.0-rc.1
python3 tools/check-docs.py
```

`make test` 在 Linux amd64 上启动刚构建的服务端做互通测试。其他宿主系统可分别执行 Go 和 Java 单元测试，但当前 Makefile 的服务端目标始终交叉编译到 Linux amd64，不能直接在 macOS / Windows 上运行该二进制。GitHub CI 使用 Ubuntu amd64。

本地只调试服务端时，可在 `server/` 执行 `go run ./cmd/mod-list-discovery-server ...`；这种直接 Go 构建使用源码中的开发版本默认值，不应用于正式发布。完整版本注入通过 Makefile 执行。

单独执行 Java 测试：

```sh
cd client
./gradlew --no-daemon test
```

不设置 `MLD_SERVER_BINARY` 时，真实服务端互通测试会跳过。完整验证应从根目录执行 `make test`。

## 版本来源与产物

默认项目版本取自根目录 [VERSION](../VERSION)。`make ... VERSION=X.Y.Z` 同时控制：

- Go 的 `app.Version` 和 `version` 命令输出，通过 linker `-X` 注入。
- JAR 文件名与 `fabric.mod.json` 的 `X.Y.Z+mc26.2`。
- Gradle 生成的 `BuildVersion.VERSION`，供客户端最低版本检查使用。

`BuildVersion.java` 位于 `client/build/generated/`，不要手工创建或提交。不要只修改 JAR 文件名冒充另一个版本；打包会校验二进制和 JAR 内部元数据。

生产 JAR 由当前 Loom 的 `jar` 任务生成并内嵌 Gson。`*-sources.jar` 用于源码查看，不是玩家安装包，也不会进入发布目录。游戏依赖不会被打包成整套 Minecraft。

## 目录与维护

`server/` 是 Go 模块；`client/` 是 Gradle/Fabric 项目；`tools/` 是发布、文档检查与浏览器验收脚本；`docs/` 是用户与开发文档；`.github/workflows/build.yml` 是构建和发布入口。

修改 Go 后执行 gofmt、vet、测试；修改客户端协议后至少执行客户端测试与真实互通测试。UI 变更执行 `node tools/ui-smoke.mjs`，截图保存在 `.tools/ui-smoke/`，不要作为正式文档内容提交。

依赖版本和 UI 第三方资源应明确固定并保留许可证。Bootstrap 的来源、版本与校验值记录在 `server/internal/webui/static/vendor/README.txt`。更改 API、兼容性或配置时，同步维护对应文档。
