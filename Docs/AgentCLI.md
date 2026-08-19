# Agent JSON CLI

`anby-wiki` 是供 Agent 调用 Anby Wiki 的 Go CLI。它以 OpenAPI `operationId`
覆盖全部 HTTP 能力，并单独覆盖 Yjs 协作 WebSocket。

## 构建与安装

```sh
make cli-build
# 产物：bin/anby-wiki

make cli-install
# 安装到当前 Go bin
```

CLI 每次只读取一个 JSON 对象，只向 stdout 写一个 JSON 结果。可从 stdin 读取，
也可用 `--input request.json`：

```sh
printf '%s\n' '{"action":"version"}' | bin/anby-wiki
bin/anby-wiki --input request.json
```

成功与失败使用同一个 envelope：

```json
{
  "ok": true,
  "action": "operation.call",
  "data": {},
  "meta": {
    "operation_id": "getSession",
    "http_status": 200,
    "request_id": "..."
  }
}
```

```json
{
  "ok": false,
  "action": "operation.call",
  "error": {
    "code": "validation_failed",
    "message": "request validation failed: ..."
  }
}
```

进程成功退出码为 `0`，输入/契约错误为 `2`，远端或运行错误为 `1`。

## 测试

CLI action、149 个 HTTP operation、multipart、二进制响应和协作 WebSocket 的本地
回归：

```sh
cd backend
go test ./internal/wikicli ./internal/clicontract -count=1
go test -race ./internal/wikicli ./internal/clicontract -count=1
```

在独立 API 环境中，用一次性测试 Token 逐个经过 CLI transport 调用全部 operation：

```sh
cd backend
CLI_E2E_BASE_URL=http://127.0.0.1:14545 \
CLI_E2E_TOKEN='anby_token_...' \
go test ./internal/wikicli \
  -run '^TestAllOperationsAgainstAPI$' -count=1 -v
```

该测试会创建、修改和查询数据，并把 `revokeCurrentCLIToken` 放在最后调用，因此 Token
执行后会被撤销。只能使用独立 database、bucket、Meilisearch index、Redis DB 和专用
测试 Token，禁止指向生产数据。

## 授权

1. 浏览器登录站点。
2. 打开 `/settings/cli`。
3. 输入 Agent 名称和 Token 有效期，生成一次性授权码。
4. 在十分钟内兑换：

```json
{
  "action": "auth.exchange",
  "base_url": "https://anbywiki.example.com",
  "code": "anby_code_..."
}
```

默认把 Token 保存到系统用户配置目录：

- macOS：`~/Library/Application Support/anby-wiki/cli.json`
- Linux：`~/.config/anby-wiki/cli.json`

目录权限为 `0700`，文件权限为 `0600`。可用环境变量覆盖：

- `ANBY_WIKI_CONFIG`
- `ANBY_WIKI_BASE_URL`
- `ANBY_WIKI_TOKEN`

授权码只能兑换一次；服务端只保存授权码和 Token 的 SHA-256。Token 权限实时继承
签发账号的 RBAC 和 PageProtection。后台撤销、账号停用或角色撤销会立即生效。

## HTTP Operation

列出能力：

```json
{"action":"operations.list"}
```

按标签或关键字过滤：

```json
{"action":"operations.list","tag":"knowledge","search":"entity"}
```

查看输入、响应 Schema：

```json
{"action":"operation.describe","operation_id":"createPage"}
```

调用 operation：

```json
{
  "action": "operation.call",
  "operation_id": "createPage",
  "body": {
    "namespace": "main",
    "title": "Agent 创建的页面",
    "language": "zh-Hans",
    "content_model": "block-v1"
  }
}
```

路径、查询和请求头分别放入 `path`、`query`、`headers`：

```json
{
  "action": "operation.call",
  "operation_id": "getPageSection",
  "path": {
    "id": "0198...",
    "section_key": "0198..."
  }
}
```

```json
{
  "action": "operation.call",
  "operation_id": "createProposal",
  "headers": {
    "Idempotency-Key": "0198..."
  },
  "body": {
    "target_type": "wiki",
    "risk_level": "medium"
  }
}
```

CLI 会在发请求前按 OpenAPI 校验路径参数、query、header、JSON body 或 multipart
字段，并在收到 JSON 后校验对应状态码的响应 Schema。未知字段、缺少必填项、非法
UUID/枚举/范围不会发到服务器。

## 文件与二进制

上传使用 `files`，值是本地路径：

```json
{
  "action": "operation.call",
  "operation_id": "createImportUploadJob",
  "headers": {
    "Idempotency-Key": "0198..."
  },
  "body": {
    "title": "导入资料",
    "route_mode": "auto"
  },
  "files": {
    "file": "/absolute/path/source.pdf"
  },
  "timeout_seconds": 600
}
```

非 JSON 响应统一放进 base64：

```json
{
  "encoding": "base64",
  "content": "...",
  "size_bytes": 1234
}
```

## 协作 WebSocket

`collaboration.run` 恢复指定 Page 的 WorkingDocument，并可顺序发送 update、
Presence 或 snapshot。Yjs bytes 必须用 base64。

恢复：

```json
{
  "action": "collaboration.run",
  "page_id": "0198...",
  "client_id": "0198...",
  "last_sequence": 0
}
```

发送幂等 update：

```json
{
  "action": "collaboration.run",
  "page_id": "0198...",
  "client_id": "0198...",
  "last_sequence": 12,
  "messages": [
    {
      "type": "update",
      "update_id": "0198...",
      "data_base64": "..."
    },
    {
      "type": "presence",
      "cursor": {
        "block_id": "0198...",
        "selection": "text"
      }
    }
  ]
}
```

输出中的 snapshot/update 同样以 base64 表示，并包含 durable sequence。

## 凭据状态与撤销

```json
{"action":"auth.status"}
```

撤销当前 Token 并删除本地明文：

```json
{"action":"auth.logout"}
```

网页 `/settings/cli` 也可撤销任意已签发 Token。Token 明文在授权兑换后不会由服务端
再次返回。
