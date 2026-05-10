# 悦享邮箱

悦享邮箱是一个基于 React、Vite 和 Tailwind CSS 的悦享生态邮件工作台。项目面向悦享 e 食平台内的用户、商户、骑手与平台运营场景，提供邮箱激活、邮件收发、联系人沉淀、业务邀请函和账号设置等能力。

## 项目亮点

- 邮件工作台：支持收件箱、星标、已发送、草稿箱、垃圾箱等常用邮箱视图。
- 邮件筛选与搜索：支持按全部、未读、重要、附件邮件筛选，也可以搜索邮件主题、发件人和内容摘要。
- 邮件操作：支持回复、转发、星标、归档、移入垃圾箱、发送邮件和保存草稿。
- 联系人管理：自动聚合生态联系人，并展示联系人详情、协作关系、标签和往来邮件记录。
- 业务邀请函：内置商户入驻、骑手招募、核心用户内测等邀请模板，适合平台运营批量邀约。
- 邮箱设置：支持默认发件人、签名、自动回复和账号安全信息展示。
- Mock / Live 双模式：默认使用本地模拟数据，也可以通过环境变量切换到真实接口。
- 合同化接口：`packages/contracts` 中沉淀了平台 HTTP、身份、上传和邮箱接口约定，便于前后端协作。

## 技术栈

- React 19
- Vite 5
- Tailwind CSS 4
- lucide-react 图标库
- DOMPurify 邮件 HTML 内容净化
- 本地 workspace 包：`@infinitech/contracts`

## 目录结构

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
├── index.html
├── package.json
└── vite.config.mjs
```

## 快速开始

请先确认本机已安装 Node.js 和 npm。

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
# 启动本地开发服务
npm run dev

# 构建生产版本
npm run build

# 本地预览生产构建
npm run preview
```

## 环境变量

项目提供了 `.env.example` 作为配置模板。首次本地运行时可以复制一份为 `.env.local`：

```bash
cp .env.example .env.local
```

常用配置说明：

| 变量名 | 默认值 | 说明 |
| --- | --- | --- |
| `VITE_MAIL_API_MODE` | `mock` | 邮箱接口模式，`mock` 使用本地模拟数据，`live` 使用真实接口 |
| `VITE_PLATFORM_API_BASE_URL` | `/api` | 平台接口基础路径 |
| `VITE_MAIL_API_BASE_URL` | `/api/v1/post-office` | 邮箱接口基础路径 |
| `VITE_MAILBOX_DOMAIN` | `yuexiang.com` | 邮箱域名 |
| `VITE_PLATFORM_SSO_ENTRY_URL` | 空 | 平台统一登录入口 |
| `VITE_MAIL_API_TIMEOUT` | `10000` | 接口请求超时时间，单位毫秒 |
| `VITE_DEV_SSO_ENABLED` | `false` | 是否启用本地开发 SSO 辅助桥接 |
| `VITE_DEV_SSO_AUTO_LOGIN` | `false` | 是否在开发模式自动登录 |

`.env.local` 属于本地配置文件，可能包含令牌或个人账号信息，已在 `.gitignore` 中排除，不建议上传到 GitHub。

## Mock 与真实接口

默认模式是 `mock`，适合独立演示和前端开发，不依赖后端服务。

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

如后端服务地址不同，请同步调整代理配置或环境变量。

## 上传 GitHub 前检查

本项目已准备好基础忽略规则，上传前建议确认：

- `node_modules/` 不上传，依赖由 `package-lock.json` 和 `package.json` 还原。
- `dist/` 不上传，生产包可以通过 `npm run build` 重新生成。
- `.env.local` 不上传，避免泄露本地令牌、手机号或接口地址。
- `.DS_Store`、`__MACOSX/`、`._*` 等 macOS 冗余文件不上传。

如果当前目录还不是 Git 仓库，可以执行：

```bash
git init
git add .
git commit -m "初始化悦享邮箱项目"
```

等 GitHub 仓库创建好后，再添加远程地址并推送：

```bash
git remote add origin <你的 GitHub 仓库地址>
git branch -M main
git push -u origin main
```

## 许可

当前项目使用 `ISC` 许可，详见 `package.json`。
