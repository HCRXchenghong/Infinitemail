# InfiniteMail（悦享邮箱）

InfiniteMail 是为悦享生态设计的邮件工作台，面向平台用户、商户、骑手与运营团队，提供邮箱激活、邮件收发、联系人沉淀、业务邀请函和账号设置等能力。

项目默认使用本地 Mock 数据，可以独立运行和演示；也支持切换到真实接口，接入悦享平台统一账号、SSO 和邮箱服务。

## 功能概览

- 邮件工作台：收件箱、星标邮件、已发送、草稿箱、垃圾箱等常用视图。
- 邮件检索：支持关键字搜索，并可按全部、未读、重要、附件邮件筛选。
- 邮件操作：支持回复、转发、星标、归档、移入垃圾箱、发送邮件和保存草稿。
- 联系人沉淀：自动聚合生态联系人，展示联系人详情、协作关系、标签和往来记录。
- 业务邀请函：内置商户入驻、骑手招募、核心用户内测等运营模板。
- 邮箱设置：支持默认发件人、签名、自动回复和账号安全信息展示。
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
http://localhost:5178
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
| `VITE_MAIL_API_MODE` | `mock` | 邮箱接口模式，`mock` 使用本地数据，`live` 使用真实接口 |
| `VITE_PLATFORM_API_BASE_URL` | `/api` | 平台接口基础路径 |
| `VITE_MAIL_API_BASE_URL` | `/api/v1/post-office` | 邮箱接口基础路径 |
| `VITE_MAILBOX_DOMAIN` | `yuexiang.com` | 邮箱域名 |
| `VITE_PLATFORM_SSO_ENTRY_URL` | 空 | 平台统一登录入口 |
| `VITE_PLATFORM_SSO_BRIDGE_NAME` | `__YUEXIANG_POST_OFFICE_SSO__` | 平台 SSO 桥接对象名 |
| `VITE_MAIL_API_TIMEOUT` | `10000` | 接口请求超时时间，单位毫秒 |
| `VITE_DEV_SSO_ENABLED` | `false` | 是否启用本地开发 SSO 辅助桥接 |
| `VITE_DEV_SSO_AUTO_LOGIN` | `false` | 是否在开发模式自动登录 |

`.env.local` 已被 `.gitignore` 排除，请不要提交包含令牌、手机号或私有接口地址的本地配置。

## Mock 与 Live 模式

默认模式为 `mock`，适合前端开发、功能演示和离线调试。

如需接入真实接口，请在 `.env.local` 中设置：

```text
VITE_MAIL_API_MODE=live
VITE_PLATFORM_API_BASE_URL=/api
VITE_MAIL_API_BASE_URL=/api/v1/post-office
```

开发服务器已在 `vite.config.mjs` 中配置 `/api` 代理，默认转发到：

```text
http://127.0.0.1:25500
```

如果后端服务地址不同，请同步调整代理配置或环境变量。

## 项目结构

```text
.
├── packages/contracts          # 平台与邮箱接口契约
├── src
│   ├── app                     # 应用入口容器
│   ├── components              # 通用布局、邮箱和 UI 组件
│   ├── dev                     # 本地开发 SSO 辅助逻辑
│   ├── lib                     # 配置、HTML 净化和工具函数
│   ├── services                # API、适配器、Mock 数据和接口契约封装
│   ├── state                   # 邮箱工作台状态管理
│   └── views                   # 登录、写信、联系人、设置等页面
├── .env.example                # 环境变量模板
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
