# GitHub Actions 构建与标签发布

[文档首页](README.md) · [开发指南](DEVELOPMENT.md) · [部署指南](DEPLOYMENT.md)

Workflow 为 [.github/workflows/build.yml](../.github/workflows/build.yml)，显示名称 **Build and release**。目标是 GitHub.com 托管的 Ubuntu 24.04 amd64 runner。

## 触发规则

| 事件 | 行为 | 版本来源 |
| --- | --- | --- |
| 推送任意分支 | 测试、构建、上传 Actions Artifact | 根目录 `VERSION` |
| Pull Request | 同样测试构建，不发布 Release | PR 中的 `VERSION` |
| Actions 手动 Run workflow | 测试构建，不发布 Release | 分支用 `VERSION`；选择标签时用标签 |
| 推送 `vX.Y.Z` 标签 | 测试构建后自动创建正式 Release | 去掉标签前缀 `v` |
| 推送 `vX.Y.Z-rc.1` 等标签 | 测试构建后自动创建 Pre-release | 去掉 `v`，保留预发布后缀 |

标签必须是合法的三段版本，不允许前导零、空格、`+` 构建元数据或额外路径。例如 `v0.2.0`、`v0.2.0-beta.1` 有效，`v0.2`、`v01.2.3` 无效。`+mc26.2` 由构建系统添加到客户端版本，不应写进发布标签。

普通分支与 PR 构建可取消被后续提交替代的同一事件/引用运行；同一标签的发布排队执行，不中途取消。

## 流水线执行内容

1. 校验版本，安装 `go.mod` 声明的 Go 和 JDK 25，配置 Gradle 缓存。
2. 检查文档本地链接，运行发布脚本测试、Go format/vet/test。
3. 构建 Go Linux amd64 二进制，执行 JUnit 与真实 Go/Java 互通测试。
4. 构建生产 JAR，把项目版本注入服务端、客户端内部常量和 MOD 元数据。
5. 验证 ELF 架构、服务端版本、JAR 版本与内嵌 Gson，只将白名单文件复制到 `dist/release/`。
6. 生成 SHA256SUMS，上传 Actions Artifact，默认保留 30 天。失败时尽可能保留 Java 测试报告 7 天。
7. 标签推送再执行独立发布作业：下载本次产物、验证校验和、创建草稿、上传附件，全部成功后公开。

构建作业仅需 `contents: read`。发布作业使用自带 `GITHUB_TOKEN` 并单独声明 `contents: write`，无需配置个人访问令牌。仓库或组织策略若禁止发布权限，需要仓库管理员调整相应策略；PR 不会获得发布权限。

## 如何发布

先将项目上传到 GitHub 仓库并确认分支构建成功。以下示例发布 0.2.0：

```sh
# 修改根目录 VERSION 为 0.2.0，并提交代码、文档与 Workflow
# 将准备发布的提交推送到仓库实际使用的分支

git tag -a v0.2.0 -m "Release 0.2.0"
git push origin v0.2.0
```

Actions 完成后，在仓库 Releases 页面查看产物。预发布使用独立标签，例如：

```sh
git tag -a v0.3.0-rc.1 -m "Release candidate 0.3.0-rc.1"
git push origin v0.3.0-rc.1
```

预发布会显式标记 Pre-release，不设置为 Latest。标签是该次发布版本的权威来源，不要求先手改所有代码常量；更新 `VERSION` 是为了使后续分支构建也显示预期版本。

## 下载文件

例如 `v0.2.0` 发布包含：

```text
mod-list-discovery-0.2.0+mc26.2.jar
mod-list-discovery-server-0.2.0-linux-amd64
config.example.yaml
SHA256SUMS
```

GitHub 自动附带的 Source code 压缩包是仓库源代码，不是编译好的玩家 MOD。开发构建在 Actions 运行详情的 Artifacts 中提供，名称类似 `mod-list-discovery-0.2.0-linux-amd64`，解压后得到以上文件。

GitHub Artifact 下载可能不保留二进制可执行权限，使用前运行 `chmod +x mod-list-discovery-server-0.2.0-linux-amd64`。校验清单覆盖二进制、JAR 和配置模板；不上传整个 `dist/`，其中可能含本地运行数据。

## 失败、重试与不可变性

- 测试或构建失败：不会执行发布作业；查看 Actions 日志与测试报告。
- 标签格式错误：在构建前失败，修正为新标签后再推送。
- 上传中断：Release 留在草稿状态，重跑失败作业可补齐/替换草稿附件，再公开。
- 已成功公开：重跑发现公开 Release 后直接跳过，不覆盖其附件。
- 更改已发布代码：使用新版本标签，不移动旧标签或用新二进制覆盖旧版。

自动说明包含下载用途和兼容性，并附 GitHub 生成的变更说明。当前本地验证可以证明构建、版本和发布脚本分支行为；真正的 GitHub token、组织策略和线上上传仍需第一次仓库运行确认。

发布流程使用 [GitHub Releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases) 和 [GitHub CLI release create](https://cli.github.com/manual/gh_release_create) 的草稿与标签校验机制；官方入口汇总于 [参考资料](REFERENCES.md)。
