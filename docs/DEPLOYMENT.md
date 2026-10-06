# 部署、配置与运维

[文档首页](README.md) · [后台操作](ADMIN_GUIDE.md) · [发布产物](RELEASING.md)

## 下载与初始化

服务端 Release 产物是 Linux amd64 / x86_64 二进制，使用 `CGO_ENABLED=0` 构建。它内嵌 Web UI 和 SQLite 驱动，无需安装 Java、Node.js 或单独的数据库服务。

下载对应二进制、配置模板及校验清单。如果下载了清单列出的全部文件，可先执行 `sha256sum --check SHA256SUMS`；只下载部分文件时核对对应行。

将二进制重命名为 `mod-list-discovery-server`，在准备好的工作目录运行：

```sh
chmod +x mod-list-discovery-server
mkdir -p data
cp config.example.yaml data/config.yaml
./mod-list-discovery-server init --config data/config.yaml
./mod-list-discovery-server serve --config data/config.yaml
```

`init` 创建管理员、数据库和签名私钥。管理员用户名为 `admin`，密码交互输入，至少 12 字节；非交互环境可通过 `MLD_ADMIN_PASSWORD` 注入。不要将密码写入仓库或发布附件。已经初始化的数据不要再次执行 `init`，它会拒绝覆盖。

默认后台为 `http://127.0.0.1:8080/admin/`。健康检查为 `GET /healthz`，检查进程与 SQLite，不代表云存储始终可用。`serve` 支持 SIGINT / SIGTERM 优雅退出。

## 生产访问地址

在配置中设置：

```yaml
server:
  listen: 127.0.0.1:8080
  public_base_url: https://mods.example.com
  data_dir: /var/lib/mod-list-discovery
  trusted_reverse_proxy: true
```

由反向代理提供 TLS，将该域名的根路径代理到本服务。`public_base_url` 必须是 origin，不能带子路径、查询参数或账号密码。生成的玩家地址是 `https://mods.example.com/<server-id>`。

只有服务确实位于可信代理后，且代理会正确覆盖客户端 IP 头时才开启 `trusted_reverse_proxy`。让代理处理完整请求路径，并为上传设置不低于服务端上限的请求体限制和足够的超时；不要缓存后台和私密清单响应。

建议以专用普通用户运行，授予数据目录写入权限；由 systemd 或其他进程管理器重启服务。配置中的相对路径按**进程工作目录**解析，生产环境使用绝对路径可以避免启动位置改变导致找不到数据。

## 配置速查

完整模板见 [config.example.yaml](../config.example.yaml)。以下为默认值或用途：

| 配置路径 | 默认 / 说明 |
| --- | --- |
| `server.listen` | `127.0.0.1:8080` |
| `server.public_base_url` | `http://127.0.0.1:8080`；生产环境修改为 HTTPS origin |
| `server.data_dir` | `./data` |
| `server.trusted_reverse_proxy` | `false` |
| `database.path` | 未指定时为 `<data_dir>/mld.sqlite3` |
| `security.session_ttl` | `12h`，后台登录有效期 |
| `security.enrollment_token_ttl` | `24h`，邀请码默认有效期 |
| `security.artifact_url_ttl` | `5m`，云存储签名下载链接有效期 |
| `security.max_upload_size` | `536870912` 字节，即 512 MiB |
| `security.allow_private_import_urls` | `false`，禁止从内网地址导入文件 |
| `storage.type` | `local`，也支持 `s3`、`oss` |
| `storage.local.root` | 未指定时为 `<data_dir>/artifacts` |

时间采用 Go duration 格式，例如 `30m`、`12h`、`168h`，不要填写 `7d`。

所有 YAML 字段都可以通过 `MLD_` 开头、路径转大写并以 `_` 连接的环境变量覆盖：

```text
MLD_SERVER_PUBLIC_BASE_URL=https://mods.example.com
MLD_SERVER_DATA_DIR=/var/lib/mod-list-discovery
MLD_DATABASE_PATH=/var/lib/mod-list-discovery/mld.sqlite3
MLD_SECURITY_MAX_UPLOAD_SIZE=536870912
MLD_STORAGE_TYPE=local
```

配置在启动时读取，修改后重启服务。

## 文件存储

| 后端 | 配置与行为 |
| --- | --- |
| Local | 默认本机磁盘，支持 Range 下载；备份时包含文件目录 |
| S3 | 设置 `storage.s3.bucket/region`，可选自定义 `endpoint` 和 `force_path_style` |
| OSS | 设置 `storage.oss.endpoint/bucket`，endpoint 使用 HTTPS |

凭证通过环境变量提供：S3 默认使用 `MLD_S3_ACCESS_KEY` / `MLD_S3_SECRET_KEY`，OSS 默认使用 `MLD_OSS_ACCESS_KEY` / `MLD_OSS_SECRET_KEY`。对应配置的 `access_key_env` / `secret_key_env` 指定**环境变量名称**，不是填写密钥本身。

对象键格式为 `sha256/aa/bb/<sha256>.jar`，可附加配置的前缀。云下载先校验设备授权和已封存版本归属，再生成短期签名地址。已有文件时切换存储后端会被拒绝，需要先离线迁移对象和相应元数据。

从链接导入默认仅允许 HTTPS 公网目标，每次连接和重定向都检查目标地址。明确开启内网导入选项后才允许内网 HTTP/HTTPS；不要仅为排除一个下载错误就放开这一限制。

## 备份、升级与恢复

持久数据包含：

```text
data/
  mld.sqlite3
  signing/ed25519.key
  artifacts/              # Local 后端
  tmp/
```

停服后备份数据库、配置、签名私钥和 Local 文件；S3/OSS 对象需通过对应存储方式备份。在线数据库备份必须使用 SQLite 一致性备份机制，不能只复制正在写入的主数据库文件。私钥默认权限为 `0600`，丢失会导致玩家原有服务器信任失效。

升级流程：备份 → 停服 → 替换二进制 → 使用原配置启动 → 检查版本、健康接口、后台和一次玩家同步。不要用下载的模板覆盖正在使用的配置，也不要重新初始化。程序降级前应核实数据库格式兼容性，保留升级前完整备份。

```sh
./mod-list-discovery-server version
./mod-list-discovery-server admin set-password --config data/config.yaml
```

重置管理员密码会注销已有后台会话；它不等同于撤销玩家设备授权。
