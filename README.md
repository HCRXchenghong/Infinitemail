# InfiniteMail（悦享邮箱）

InfiniteMail 是为公司内部自建邮箱设计的邮件工作台，面向员工、运营团队和公司用户，提供邮箱激活、邮件收发、联系人沉淀、通知邮件模板和账号设置等能力。

项目默认开发脚本接入 Go BFF，可由管理后台保存邮箱域名、登录注册策略、注册码、短信和邮件服务配置；用户端和管理端默认不会走本地假数据。

## 功能概览

- 邮件工作台：收件箱、星标邮件、已发送、草稿箱、垃圾箱等常用视图。
- 邮件检索：支持关键字搜索，并可按全部、未读、重要、附件邮件筛选。
- 邮件操作：支持回复、转发、附件、已读同步、星标、归档、移入垃圾箱、发送邮件和保存草稿。
- 联系人沉淀：自动聚合邮件联系人，展示联系人详情、标签和往来记录。
- 通知邮件模板：内置账号开通、协作沟通和客服通知模板，发送仍走真实邮件底座。
- 邮箱设置：支持默认发件人、签名、自动回复和账号安全信息展示。
- 管理后台：支持邮箱域名、邮箱名前缀、后台管理员密码/API Token、OAuth 展示名、邮箱/手机号登录注册、注册码注册和短信验证码配置。
- 域名交付检查：后台生成 MX/SPF/DKIM/DMARC 建议记录，并由 BFF 执行 DNS 验证、保存检查状态。
- 邮箱底座接入：后台可配置邮件服务地址、开通接口路径、账号生命周期接口和管理令牌，支持连接测试、真实开通请求、禁用/启用/重置密码同步、账号开通状态展示和失败重试，任务中心可手动或自动处理开通队列。
- 邮件数据面：用户端发信、附件、保存草稿、收件列表、详情、星标、移动、已读状态和投递追踪已走 BFF；后台可配置 HTTP 邮件数据面，也可直接配置自建 SMTP/IMAP，由 BFF 通过协议读写真实邮箱。
- 账号生命周期：后台可禁用/启用邮箱账号、重置临时密码，并自动清理被禁用或重置账号的会话。
- 审计日志：记录后台配置、注册码、验证码、注册、登录、邮箱激活和设置变更等关键动作。
- 接口契约：`packages/contracts` 内置平台 HTTP、身份、上传和邮箱接口约定。

## 技术栈

- React 19
- Vite 5
- Tailwind CSS 4
- lucide-react
- DOMPurify
- 本地契约包：`@infinitech/contracts`

## 快速开始

```bash
npm install
npm run dev
```

开发服务默认运行在：

```text
用户端：http://localhost:1788
管理端：http://localhost:1888
API/BFF：http://localhost:1666
```

本地分别启动：

```bash
npm run dev:api
npm run dev:user
npm run dev:admin
```

## 常用命令

```bash
# 启动开发服务
npm run dev

# 构建生产版本
npm run build

# 本地预览生产构建
npm run preview
```

## 环境变量

项目提供 `.env.example` 作为配置模板。本地开发时可以复制为 `.env.local`：

```bash
cp .env.example .env.local
```

| 变量名 | 默认值 | 说明 |
| --- | --- | --- |
| `VITE_APP_SURFACE` | `user` | 当前构建目标，用户端为 `user`，管理端为 `admin` |
| `VITE_API_PROXY_TARGET` | `http://127.0.0.1:1666` | Vite 开发代理转发到的 Go BFF 地址 |
| `VITE_USER_APP_ORIGIN` | `http://127.0.0.1:1788` | 用户端访问地址，后台生成邀请链接时使用 |
| `VITE_ADMIN_APP_ORIGIN` | `http://127.0.0.1:1888` | 管理端访问地址 |
| `VITE_MAIL_API_BASE_URL` | `/api/v1/post-office` | 邮箱接口基础路径 |
| `VITE_ADMIN_API_TOKEN` | 空 | 管理端调用 BFF 后台接口时附带的可选令牌；生产建议改用管理员登录会话 |
| `VITE_MAILBOX_DOMAIN` | `yuexiang.com` | 邮箱域名 |
| `VITE_MAIL_API_TIMEOUT` | `10000` | 接口请求超时时间，单位毫秒 |
| `HTTP_ADDR` | `:1666` | Go BFF 监听地址 |
| `USER_APP_ORIGIN` | `http://127.0.0.1:1788` | BFF 生成注册链接时使用的用户端地址 |
| `DATA_PATH` | `bff/.data/infinitemail-bff.json` | BFF JSON store 本地持久化路径 |
| `ATTACHMENT_DIR` | `bff/.data/attachments` | BFF 附件文件目录；发信、草稿和 Webhook 收信会把内联附件落成鉴权下载文件 |
| `ADMIN_API_TOKEN` | 空 | BFF 后台接口自动化令牌；设置后请求可带 `X-Admin-Token` 或 Bearer token |
| `ADMIN_USERNAME` | `admin` | 管理员登录账号 |
| `ADMIN_PASSWORD` | 空 | 管理员登录密码；设置后管理端会先进入登录页 |
| `ADMIN_PASSWORD_HASH` | 空 | Argon2id 管理员密码哈希，优先级高于 `ADMIN_PASSWORD` |
| `DATABASE_URL` | 空 | PostgreSQL 连接串；设置后 BFF 自动执行 `bff/migrations` 并使用规范化业务表持久化 |
| `OPS_WORKER_TICK_SECONDS` | `30` | BFF 自动巡检 worker 的轮询秒数；是否自动执行由管理端任务中心保存 |
| `MAIL_WEBHOOK_TOKEN` | 空 | 邮件底座调用 BFF 收信/投递状态 Webhook 的鉴权令牌；请求可使用 `X-Mail-Webhook-Token` 或 Bearer token |
| `MAILBOX_CREDENTIAL_KEY` | 空 | 邮箱开通后保存 IMAP/SMTP 用户凭据的加密密钥；生产环境请设置为足够长的随机字符串 |
| `INFINITEMAIL_PRODUCTION_STRICT` | `false` | 生产严格模式；开启后 `/readyz` 会把 PostgreSQL、邮箱凭据加密密钥、后台保护、DNS、真实邮件数据面、短信和 OAuth 缺口作为阻塞项返回 |
| `REQUIRE_POSTGRES` | `false` | 单独要求必须使用 PostgreSQL |
| `REQUIRE_MAIL_DATA_PLANE` | `false` | 单独要求邮件列表、详情、发信、草稿、星标、移动、已读必须接入真实 HTTP 数据面或 SMTP/IMAP 协议 |
| `REQUIRE_MAIL_WEBHOOK` | `false` | 单独要求必须配置邮件 Webhook 鉴权令牌 |
| `REQUIRE_REAL_SMS` | `false` | 单独要求短信验证码必须走完整阿里云配置 |
| `REQUIRE_REAL_OAUTH` | `false` | 单独要求 OAuth 必须配置真实 Provider |
| `ALLOW_LOCAL_SMS_DEBUG` | `false` | 仅本地开发允许不接阿里云时生成调试验证码；正式运行默认禁止 |
| `HIDE_LOCAL_SMS_CODES` | `false` | 本地调试通道是否也隐藏验证码明文；生产严格模式会自动隐藏 |

`.env.local` 已被 `.gitignore` 排除，请不要提交包含令牌、手机号或私有接口地址的本地配置。

管理后台支持两种生产保护方式：可以在“登录注册 / 后台安全”中保存管理员密码和后台 API Token，`1888` 会显示管理员登录页并使用短期后台会话；也可以通过环境变量设置 `ADMIN_PASSWORD`/`ADMIN_PASSWORD_HASH` 或 `ADMIN_API_TOKEN` 给 CI、内网脚本、受控网关使用。

## BFF 接入模式

默认开发脚本固定接入 Go BFF，用户端和管理端会通过 `/api` 代理连接 `1666`。BFF 不可用时，管理端不会再假装已登录，用户端也不会自动切到前端本地数据。

当前 BFF 模式已支持公司内部账号闭环：管理员在 `http://localhost:1888` 配置 OAuth/手机号/邮箱登录注册策略和注册码要求，用户在 `http://localhost:1788` 按后台配置进入 OAuth 首屏或账号登录注册入口，注册成功后进入邮箱工作台。

如需调整本地 API 地址，请在 `.env.local` 中设置：

```text
VITE_API_PROXY_TARGET=http://127.0.0.1:1666
VITE_MAIL_API_BASE_URL=/api/v1/post-office
```

开发服务器已在 `vite.config.mjs` 中配置 `/api` 代理，默认转发到：

```text
http://127.0.0.1:1666
```

如果后端服务地址不同，请同步调整代理配置或环境变量。

Go BFF 当前已经支持 live 闭环：后台配置、域名 DNS 验证、邮件服务连接测试、真实邮箱开通请求、账号禁用/启用/重置密码同步、注册码链接、短信验证码、邮箱/手机号注册登录、真实 OAuth/OIDC 跳转与回调、Session、邮箱 Profile、基础邮件列表、发信、附件、草稿、详情、星标、移动、已读状态、投递追踪、设置接口、账号生命周期、邮箱开通队列、任务中心、自动巡检 worker 和审计日志。短信验证码服务端保存哈希，用户端公开配置不会包含后台短信日志、注册码、审计和账号列表；正式运行默认要求后台完整配置阿里云短信，只有显式设置 `ALLOW_LOCAL_SMS_DEBUG=true` 时才允许本地调试验证码。开发期默认使用 JSON store；设置 `DATABASE_URL` 后会自动执行 `bff/migrations`，并把后台配置、账号、注册码、短信日志、审计日志、会话、邮箱设置、基础邮件、自动巡检配置和开通任务写入 PostgreSQL 规范化表，同时保留快照作为过渡兜底。

邮件服务接口在管理端“域名邮箱 / 邮件服务”里配置：`服务地址` 用于健康检查，`开通接口路径` 可以填相对路径 `/api/v1/mailboxes` 或完整 URL，`账号生命周期接口` 用于同步 `disable`、`enable`、`reset_password`，`收件列表接口`、`邮件详情接口`、`草稿接口`、`星标接口`、`移动接口` 和 `已读接口` 用于对接真实邮件数据面。若已有自建邮件服务器并开放 SMTP 25/587/465，可直接配置 `SMTP 主机`、`SMTP 端口`、`SMTP TLS`、账号密码和“启用 SMTP 发信”；此时发信可以不依赖 HTTP 发信接口，BFF 会生成标准 MIME 邮件并通过 SMTP 投递 To/Cc/Bcc。若邮件服务器开放 IMAP 143/993，可配置 `IMAP 主机`、`IMAP 端口`、`IMAP TLS`、账号模板和密码并启用 IMAP 收件；没有 HTTP 数据面时，BFF 会通过 IMAP 拉取收件列表和详情，同步星标、已读、移动，并把草稿追加到 Drafts。BFF 会携带 `Idempotency-Key`、`Authorization: Bearer <管理令牌>` 和账号 payload；开通接口返回 `2xx` 视为成功，`409` 视为账号已存在并按幂等成功处理。开启 `INFINITEMAIL_PRODUCTION_STRICT=true`、`REQUIRE_MAIL_DATA_PLANE=true` 或后台“生产严格数据面”后，所有邮件数据面缺口都会直接报错；SMTP+IMAP 已配置时可替代对应 HTTP 发信/收信接口。

如果邮件底座更适合主动回写 BFF，可以配置 `MAIL_WEBHOOK_TOKEN` 后调用：

```text
POST /api/v1/post-office/webhooks/mail/inbound
POST /api/v1/post-office/webhooks/mail/delivery
```

`inbound` 用于把真实收件写入用户邮箱，payload 至少包含收件账号标识或收件地址、发件人、主题和正文；`delivery` 用于把真实投递结果回写到已发送邮件，按 `messageId` 或 `providerMessageId` 匹配。两个接口都必须携带 `X-Mail-Webhook-Token: <MAIL_WEBHOOK_TOKEN>` 或 `Authorization: Bearer <MAIL_WEBHOOK_TOKEN>`。

附件由 BFF 统一落盘到 `ATTACHMENT_DIR`：用户端上传或发送时，BFF 会把内联 base64 写成私有文件，并在邮件中只保留 `assetId` 和鉴权下载地址 `GET /api/v1/post-office/attachments/{assetId}/download`；调用真实发信接口前，BFF 会按当前账号重新读取附件内容并回填给邮件底座。

管理端“总览 / 真实接入检查”会展示当前部署是否满足上线要求。生产部署建议至少配置：

```text
INFINITEMAIL_PRODUCTION_STRICT=true
DATABASE_URL=postgres://...
ADMIN_PASSWORD_HASH=argon2id:...
```

如果公司登录、短信和邮件底座已经准备好，再在后台补齐 OAuth Provider、阿里云短信、DNS 和邮件服务各接口路径，直到真实接入检查没有阻塞项。

BFF 已将新密码改为 Argon2id 强哈希，旧的 salted SHA256 密码哈希仍可验证以支持平滑迁移；服务端会话在数据库中使用 token hash 作为主键，避免把原始 session token 作为查询键落库。

## Docker 部署

仓库内置 `docker-compose.yml`，会启动 PostgreSQL、Go BFF、用户端和管理端：

```bash
docker compose up --build
```

默认端口仍保持：

```text
API/BFF: http://localhost:1666
用户端:  http://localhost:1788
管理端:  http://localhost:1888
```

需要连自建邮件服务器时，可以一起启动 Stalwart 邮件底座 profile：

```bash
docker compose --profile mailserver up --build
```

该 profile 会额外开放 SMTP `25/465/587`、IMAP `143/993` 和 Stalwart WebUI `8080`，并挂载 `stalwart-etc`、`stalwart-data` 两个持久化卷。Stalwart 官方建议生产镜像固定到小版本 tag，本项目默认使用 `stalwartlabs/stalwart:v0.16`，后续升级时按官方升级文档调整。第一次启动后进入 Stalwart WebUI 完成域名、管理员和 DKIM 初始化，再回到越想邮局后台“域名邮箱 / 邮件服务”点击“套用 Stalwart 预设”，补充管理员令牌并测试连接。

生产部署先复制 `deploy/.env.production.example`，设置 PostgreSQL 密码、管理员密码哈希、Webhook token、OAuth、短信和邮件底座参数后再启动：

```bash
cp deploy/.env.production.example .env.production
docker compose --env-file .env.production up --build -d
```

## 项目结构

```text
.
├── bff                         # Go BFF、PostgreSQL migrations 和邮件/账号后端
├── deploy                      # Dockerfile、Nginx 配置和生产 env 模板
├── packages/contracts          # 平台与邮箱接口契约
├── src
│   ├── app                     # 应用入口容器
│   ├── components              # 通用布局、邮箱和 UI 组件
│   ├── dev                     # 本地开发 SSO 辅助逻辑
│   ├── lib                     # 配置、HTML 净化和工具函数
│   ├── services                # API、适配器和接口契约封装
│   ├── state                   # 邮箱工作台状态管理
│   └── views                   # 登录、写信、联系人、设置等页面
├── .env.example                # 环境变量模板
├── docker-compose.yml          # PostgreSQL + API + 用户端 + 管理端部署入口
├── index.html                  # Vite HTML 入口
├── package.json                # 项目脚本与依赖
└── vite.config.mjs             # Vite 配置
```

## 构建说明

生产构建会输出到 `dist/`：

```bash
npm run build
```

`dist/`、`node_modules/`、`.env.local` 和 macOS 隐藏文件均已通过 `.gitignore` 排除，仓库只保留源码、契约、配置模板和锁定依赖版本所需文件。

## 许可

当前项目使用 ISC License。
