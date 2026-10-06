# 协议与 API 参考

[文档首页](README.md) · [技术架构](ARCHITECTURE.md)

本文描述当前实现的主要接口，下载路径包含 Server ID，以便同时校验设备授权和文件归属。

## 玩家接口

| 方法与路径 | 请求 / 返回 |
| --- | --- |
| `GET /{serverId}/mod-list.json` | 获取正式版清单，可用 `?channel=name` 指定通道 |
| `POST /api/v1/servers/{serverId}/enroll` | 请求 `{code, deviceName}`，返回 `{deviceId, accessToken, serverId, signingKey}` |
| `GET /api/v1/servers/{serverId}/artifacts/{sha256}/download` | 验证服务器范围后下载文件；Local 返回内容，云存储使用签名地址 |
| `GET /healthz` | 服务和 SQLite 健康状态 |

私密清单和下载请求携带 `Authorization: Bearer <accessToken>`。未授权时返回认证错误，邀请码只用于 enroll，不作为长期下载令牌。

## 签名信封

协议 schemaVersion 为 1。响应信封包含：

```json
{
  "schemaVersion": 1,
  "payloadEncoding": "base64url",
  "payload": "<编码后的原始 JSON 字节>",
  "signature": {
    "algorithm": "Ed25519",
    "keyId": "<公钥标识>",
    "publicKey": "<公钥>",
    "value": "<签名>"
  }
}
```

先解码 payload，再用 Ed25519 对这些**原始字节**验签，最后解析 JSON；重新序列化会改变签名输入。编码、长度和校验细节以服务端 `signing` 与客户端 `ManifestVerifier` 实现为准。

Payload 包含 `server`、`channel`、`release`、`runtime`、`mods` 和以 SHA256 为键的 `artifacts`。下载信息指向受控 API，不在清单内放入短期云签名 URL。`runtime.minDiscoveryVersion` 是同步客户端最低版本，不是 Minecraft 版本。

## 后台认证

所有后台接口位于 `/api/v1/admin/`。`POST login` 接受 `{username, password}`，设置会话 Cookie 并返回 `{csrfToken}`；刷新页面可通过 `GET session` 获取当前 token。写请求同时携带会话 Cookie 和 `X-CSRF-Token`，`POST logout` 注销会话。

浏览器发送同源请求；后台不支持用玩家访问令牌代替管理员登录。

## 后台资源

以下路径均相对于 `/api/v1/admin/`：

| 方法与路径 | 用途 |
| --- | --- |
| `GET overview`、`GET settings`、`GET audit` | 概览、可公开显示的部署信息、最近操作 |
| `GET/POST servers` | 列出 / 创建服务器 |
| `GET/PATCH/DELETE servers/{id}` | 查询 / 编辑 / 删除服务器 |
| `GET/POST servers/{id}/releases` | 列出 / 创建该服务器草稿 |
| `GET servers/{id}/channels` | 通道列表 |
| `PUT servers/{id}/channels/{name}` | 请求 `{releaseId}`，指向该服务器的已封存版本 |
| `GET/POST servers/{id}/enrollment-codes` | 查看 / 创建邀请码 |
| `GET servers/{id}/clients` | 该服务器设备列表 |
| `GET artifacts`、`GET/DELETE artifacts/{sha256}` | 文件列表、详情、删除 |
| `POST artifacts/upload` | multipart 表单字段 `file` |
| `POST artifacts/import` | 请求 `{url}` 导入 JAR |
| `GET releases`、`GET/PATCH releases/{id}` | 全局版本列表、详情、修改草稿 |
| `POST releases/{id}/mods` | 添加或替换同一 Mod ID 的规则 |
| `DELETE releases/{id}/mods/{modId}` | 移除草稿规则 |
| `GET releases/{id}/validate` | 基础依赖检查，不等同于完整封存校验 |
| `POST releases/{id}/publish` | 封存并锁定，**不自动启用通道** |
| `GET clients`、`GET enrollment-codes` | 全局设备 / 邀请码列表 |
| `POST clients/{id}/revoke` | 撤销设备授权 |
| `POST enrollment-codes/{id}/revoke` | 撤销邀请码 |

主要写入结构：

```json
{
  "id": "friends",
  "displayName": "朋友们的生存服",
  "description": "轻量生存",
  "visibility": "private"
}
```

编辑服务器不传 `id`。服务器标识创建后不可修改。删除包含草稿或设备的服务器需要 `?confirm=true`；存在已封存版本时始终拒绝删除。

```json
{
  "name": "首次开放",
  "minecraft": "26.2",
  "fabricLoader": ">=0.19.0",
  "java": ">=25",
  "minDiscoveryVersion": "0.1.0"
}
```

以上为创建/编辑 Release 的结构，最低客户端版本应按实际支持范围填写。不要把服务器编号放进 body，它已包含在创建路径中。

```json
{
  "modId": "example_mod",
  "name": "示例 MOD",
  "requirement": "optional",
  "artifact": "<64 位 SHA256>",
  "versionConstraint": "*",
  "defaultSelected": true
}
```

`requirement` 为 `required` / `optional` / `forbidden`。禁止规则的 `artifact` 必须为 `null`；安装规则中的 Mod ID 应与文件元数据一致。同步客户端自身不能加入分发规则。

邀请码创建请求如 `{ "maxUses": 1, "ttl": "24h" }`，返回 `{id, code, expiresAt}`。完整 `code` 只在创建时返回，后续列表不会返回它。`ttl` 使用 Go duration，不接受 `7d`，可填 `168h`。

## 错误与客户端处理

错误通常包含 `{error, message, requestId}`，但调用方不应假定所有错误都有 requestId。常见情况：401 未登录或无设备认证；403 CSRF 或授权失败；404 对象不存在；409 状态冲突、已封存或仍有引用；429 触发频率限制。

JSON 请求采用严格字段校验，额外未知字段也可能导致 `invalid_json`。`publish_rejected` 包含检查信息，调用方应展示可理解的错误，不应把 HTTP 请求返回当作业务成功。
