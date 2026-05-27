import React, { useEffect, useMemo, useState } from "react";
import { Copy, FileClock, Gauge, KeyRound, Lock, Mail, MessageSquareText, Settings2, ShieldCheck, Unlock, Users, Workflow } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Button } from "../components/ui/Button";
import { cn } from "../lib/utils/cn";
import { runtimeConfig } from "../lib/config/runtime";

const adminNav = [
  { id: "overview", label: "总览", icon: Gauge },
  { id: "mailbox", label: "域名邮箱", icon: Mail },
  { id: "auth", label: "登录注册", icon: ShieldCheck },
  { id: "sms", label: "短信验证码", icon: MessageSquareText },
  { id: "invites", label: "注册码", icon: KeyRound },
  { id: "accounts", label: "账号记录", icon: Users },
  { id: "ops", label: "任务中心", icon: Workflow },
  { id: "audit", label: "审计日志", icon: FileClock },
];

const navTitle = adminNav.reduce((result, item) => {
  result[item.id] = item.label;
  return result;
}, {});
const adminNavIds = new Set(adminNav.map((item) => item.id));
const adminNavStorageKey = "infinitemail.admin.activeNav";

function readInitialAdminNav() {
  if (typeof window === "undefined") {
    return "overview";
  }
  const hashNav = window.location.hash.replace(/^#/, "");
  if (adminNavIds.has(hashNav)) {
    return hashNav;
  }
  let saved = "";
  try {
    saved = window.sessionStorage?.getItem(adminNavStorageKey) || "";
  } catch {
    saved = "";
  }
  return adminNavIds.has(saved) ? saved : "overview";
}

function inputClass(extra = "") {
  return cn(
    "mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-[#009BF5] focus:outline-none focus:ring-2 focus:ring-[#009BF5]/20",
    extra,
  );
}

function normalizeList(value) {
  if (Array.isArray(value)) {
    return value;
  }
  return String(value || "")
    .split(/[\s,，、]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function Field({ label, value, onChange, placeholder = "", type = "text" }) {
  return (
    <label className="block">
      <span className="block text-sm font-medium text-slate-700">{label}</span>
      <input
        type={type}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className={inputClass()}
      />
    </label>
  );
}

function Toggle({ label, checked, onChange }) {
  return (
    <label className="flex items-center justify-between rounded-md border border-slate-200 bg-white px-3 py-2.5">
      <span className="text-sm font-medium text-slate-700">{label}</span>
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="h-4 w-4 rounded border-slate-300 text-[#009BF5] focus:ring-[#009BF5]"
      />
    </label>
  );
}

function SelectField({ label, value, onChange, options }) {
  return (
    <label className="block">
      <span className="block text-sm font-medium text-slate-700">{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value)} className={inputClass()}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </label>
  );
}

function NavItem({ item, activeNav, onNavigate }) {
  const Icon = item.icon;
  const active = activeNav === item.id;
  return (
    <button
      type="button"
      onClick={() => onNavigate(item.id)}
      className={cn(
        "w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors text-left",
        active ? "bg-[#E5F5FF] text-[#009BF5]" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900",
      )}
    >
      <Icon size={18} className={active ? "text-[#009BF5]" : "text-slate-400"} />
      {item.label}
    </button>
  );
}

function Panel({ title, action, children }) {
  return (
    <section className="rounded-lg border border-slate-200 bg-white shadow-sm">
      <div className="flex items-center justify-between gap-3 border-b border-slate-200 px-5 py-4">
        <h2 className="text-base font-bold text-slate-900">{title}</h2>
        {action}
      </div>
      <div className="p-5">{children}</div>
    </section>
  );
}

function Stat({ label, value }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">{label}</div>
      <div className="mt-2 truncate text-xl font-bold text-slate-900">{value}</div>
    </div>
  );
}

function Empty({ children }) {
  return <div className="rounded-md bg-slate-50 px-4 py-8 text-center text-sm text-slate-400">{children}</div>;
}

function Pill({ children, active = false }) {
  return (
    <span className={cn("rounded-full px-2 py-0.5 text-xs font-medium", active ? "bg-[#009BF5] text-white" : "bg-slate-100 text-slate-500")}>
      {children}
    </span>
  );
}

function AdminLoginScreen({ adminAuth, loginDraft, setLoginDraft, onLogin }) {
  return (
    <div className="h-screen w-full bg-slate-50 text-slate-900 antialiased">
      <div className="flex h-14 items-center justify-between border-b border-slate-200 bg-white px-6">
        <div className="flex items-center gap-2 text-[#009BF5] font-bold text-lg tracking-wide">
          <Mail size={22} strokeWidth={2.5} />
          <span className="text-slate-800">悦享邮局</span>
        </div>
        <Pill active>后台</Pill>
      </div>
      <main className="flex min-h-[calc(100vh-56px)] items-center justify-center px-6 py-10">
        <form onSubmit={onLogin} className="w-full max-w-sm rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
          <div className="mb-5 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-md bg-[#E5F5FF] text-[#009BF5]">
              <Lock size={20} />
            </div>
            <div>
              <h1 className="text-lg font-bold text-slate-900">管理后台登录</h1>
              <p className="text-sm text-slate-500">InfiniteMail Admin</p>
            </div>
          </div>
          <div className="space-y-4">
            <Field
              label="管理员账号"
              value={loginDraft.username}
              onChange={(value) => setLoginDraft((current) => ({ ...current, username: value }))}
              placeholder={adminAuth.username || "admin"}
            />
            <Field
              label="管理员密码"
              type="password"
              value={loginDraft.password}
              onChange={(value) => setLoginDraft((current) => ({ ...current, password: value }))}
            />
            {adminAuth.errorMessage ? (
              <div className="rounded-md border border-red-100 bg-red-50 px-3 py-2 text-sm text-red-600">{adminAuth.errorMessage}</div>
            ) : null}
            <Button type="submit" disabled={adminAuth.isLoading} className="w-full justify-center">
              {adminAuth.isLoading ? "登录中..." : "登录"}
            </Button>
          </div>
        </form>
      </main>
    </div>
  );
}

function dnsStatusLabel(status) {
  switch (status) {
    case "verified":
      return "已通过";
    case "partial":
      return "部分通过";
    default:
      return "待验证";
  }
}

function mailServerStatusLabel(status) {
  switch (status) {
    case "online":
      return "已连通";
    case "offline":
      return "未连通";
    case "unknown":
      return "待测试";
    default:
      return "未配置";
  }
}

function mailboxStatusLabel(status) {
  switch (status) {
    case "provisioned":
      return "已开通";
    case "queued":
      return "开通中";
    case "failed":
      return "开通失败";
    default:
      return "待配置";
  }
}

function provisionJobStatusLabel(status) {
  switch (status) {
    case "succeeded":
      return "已完成";
    case "failed":
      return "失败";
    case "blocked":
      return "待配置";
    case "running":
      return "执行中";
    default:
      return "排队中";
  }
}

function opsStatusLabel(status) {
  switch (status) {
    case "success":
      return "正常";
    case "warning":
      return "需处理";
    case "failed":
      return "失败";
    case "running":
      return "执行中";
    default:
      return "未运行";
  }
}

function deploymentStatusLabel(status) {
  switch (status) {
    case "ready":
      return "可上线";
    case "blocked":
      return "待补齐";
    default:
      return "待检查";
  }
}

function checkStatusLabel(status) {
  switch (status) {
    case "ok":
      return "已接入";
    case "blocking":
      return "必填";
    default:
      return "待完善";
  }
}

function isInviteActive(invite) {
  if (!invite || invite.usedAt) return false;
  if (!invite.expiresAt) return true;
  const expiresAt = new Date(invite.expiresAt).getTime();
  return Number.isNaN(expiresAt) || Date.now() <= expiresAt;
}

function buildStalwartPresetServer(current = {}) {
  return {
    ...current,
    provider: "stalwart",
    enabled: true,
    strictDataPlane: true,
    baseUrl: current.baseUrl || "http://stalwart:8080",
    provisionPath: "",
    lifecyclePath: "",
    messageListPath: "",
    messageDetailPath: "",
    messageSendPath: "",
    draftPath: "",
    messageStarPath: "",
    messageMovePath: "",
    messageReadPath: "",
    smtpEnabled: true,
    smtpHost: current.smtpHost || "stalwart",
    smtpPort: Number(current.smtpPort || 25),
    smtpUsername: current.smtpUsername || "",
    smtpTlsMode: current.smtpTlsMode || "auto",
    imapEnabled: true,
    imapHost: current.imapHost || "stalwart",
    imapPort: Number(current.imapPort || 993),
    imapUsername: current.imapUsername || "{email}",
    imapTlsMode: current.imapTlsMode || "tls",
  };
}

function activeAccountCount(adminConfig) {
  const users = (adminConfig.registeredUsers || []).filter((user) => user.status !== "disabled").length;
  const invites = (adminConfig.invites || []).filter(isInviteActive).length;
  return users + invites;
}

function formatTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function OverviewPage({ adminConfig }) {
  const activeAccounts = activeAccountCount(adminConfig);
  const deployment = adminConfig.deployment || {};
  const checks = Array.isArray(deployment.checks) ? deployment.checks : [];

  return (
    <div className="space-y-5">
      <div className="grid gap-4 md:grid-cols-3 xl:grid-cols-6">
        <Stat label="邮箱域名" value={`@${adminConfig.mailbox?.domain || "-"}`} />
        <Stat label="允许前缀" value={(adminConfig.mailbox?.allowedPrefixes || []).length} />
        <Stat label="注册码" value={adminConfig.invites?.length || 0} />
        <Stat label="注册账号" value={adminConfig.registeredUsers?.length || 0} />
        <Stat label="用户总数" value={activeAccounts} />
        <Stat label="部署状态" value={deploymentStatusLabel(deployment.status)} />
      </div>

      <Panel title="真实接入检查">
        <div className="mb-4 grid gap-3 md:grid-cols-4">
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">运行模式</div>
            <div className="mt-1 font-semibold text-slate-900">{deployment.strict ? "生产严格" : "标准真实接入"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">存储</div>
            <div className="mt-1 font-semibold text-slate-900">{deployment.store || "json"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">阻塞项</div>
            <div className="mt-1 font-semibold text-slate-900">{deployment.blockingCount || 0}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">整体状态</div>
            <div className="mt-1 font-semibold text-slate-900">{deployment.ready ? (deployment.strict ? "已满足上线检查" : "开发可用") : "仍需补齐配置"}</div>
          </div>
        </div>
        {checks.length ? (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] text-left text-sm">
              <thead className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                <tr className="border-b border-slate-200">
                  <th className="px-3 py-2">项目</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">要求</th>
                  <th className="px-3 py-2">说明</th>
                </tr>
              </thead>
              <tbody>
                {checks.map((check) => (
                  <tr key={check.id} className="border-b border-slate-100">
                    <td className="px-3 py-3 font-medium text-slate-900">{check.label}</td>
                    <td className="px-3 py-3"><Pill active={check.status === "ok"}>{checkStatusLabel(check.status)}</Pill></td>
                    <td className="px-3 py-3 text-slate-500">{check.required ? "生产必需" : "可选"}</td>
                    <td className="px-3 py-3 text-slate-500">{check.message || "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : <Empty>等待 BFF 返回部署检查</Empty>}
      </Panel>

      <Panel title="当前配置">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="font-medium text-slate-900">邮箱格式</div>
            <div className="mt-1 text-slate-500">{adminConfig.mailbox?.prefixPolicyEnabled === false ? "账号@域名" : "前缀-账号@域名"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="font-medium text-slate-900">注册策略</div>
            <div className="mt-1 text-slate-500">{adminConfig.auth?.inviteRequired === false ? "开放注册" : "注册码注册"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="font-medium text-slate-900">OAuth 名称</div>
            <div className="mt-1 text-slate-500">{adminConfig.auth?.oauthProviderName || "-"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="font-medium text-slate-900">短信通道</div>
            <div className="mt-1 text-slate-500">{adminConfig.sms?.aliyunEnabled ? "阿里云短信" : "待接入阿里云短信"}</div>
          </div>
        </div>
      </Panel>
    </div>
  );
}

function MailboxPage({ mailbox, setMailbox, allowedPrefixText, onSave, onApplyStalwartPreset, onVerifyDNS, onTestMailServer, loading, copy }) {
  const prefixes = normalizeList(mailbox.allowedPrefixes);
  const previewLocal = mailbox.prefixPolicyEnabled === false ? "chenghong" : `${mailbox.defaultPrefix || prefixes[0] || "user"}-chenghong`;
  const dns = mailbox.dns || {};
  const dnsRecords = dns.records?.length ? dns.records : dns.recommended || [];
  const server = mailbox.server || {};
  return (
    <div className="space-y-5">
      <div className="grid gap-5 xl:grid-cols-[1fr_320px]">
        <Panel title="域名邮箱" action={<Button onClick={onSave} disabled={loading}>保存</Button>}>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="邮箱域名" value={mailbox.domain || ""} onChange={(value) => setMailbox((current) => ({ ...current, domain: value }))} placeholder="example.com" />
            <Field label="默认前缀" value={mailbox.defaultPrefix || ""} onChange={(value) => setMailbox((current) => ({ ...current, defaultPrefix: value }))} placeholder="user" />
            <label className="block md:col-span-2">
              <span className="block text-sm font-medium text-slate-700">允许前缀</span>
              <input
                value={allowedPrefixText}
                onChange={(event) => setMailbox((current) => ({ ...current, allowedPrefixes: event.target.value }))}
                className={inputClass()}
                placeholder="user, admin, support"
              />
            </label>
            <Toggle
              label="强制前缀"
              checked={mailbox.prefixPolicyEnabled !== false}
              onChange={(checked) => setMailbox((current) => ({ ...current, prefixPolicyEnabled: checked }))}
            />
          </div>
        </Panel>

        <Panel title="预览">
          <div className="rounded-md bg-slate-50 p-3 text-sm font-medium text-slate-900">
            {previewLocal}@{mailbox.domain || "example.com"}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {prefixes.map((prefix) => <Pill key={prefix} active={prefix === mailbox.defaultPrefix}>{prefix}-</Pill>)}
          </div>
        </Panel>
      </div>

      <Panel
        title="DNS 验证"
        action={<Button type="button" variant="secondary" onClick={onVerifyDNS} disabled={loading}>验证 DNS</Button>}
      >
        <div className="mb-4 grid gap-3 md:grid-cols-3">
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">当前状态</div>
            <div className="mt-1 font-semibold text-slate-900">{dnsStatusLabel(dns.status)}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">通过记录</div>
            <div className="mt-1 font-semibold text-slate-900">{dns.verifiedRecords || 0}/{dns.totalRecords || 4}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">上次检查</div>
            <div className="mt-1 font-semibold text-slate-900">{formatTime(dns.checkedAt)}</div>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-left text-sm">
            <thead className="text-xs font-semibold uppercase tracking-wider text-slate-400">
              <tr className="border-b border-slate-200">
                <th className="px-3 py-2">类型</th>
                <th className="px-3 py-2">主机记录</th>
                <th className="px-3 py-2">建议值</th>
                <th className="px-3 py-2">状态</th>
                <th className="px-3 py-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {dnsRecords.map((record) => (
                <tr key={`${record.type}-${record.host}`} className="border-b border-slate-100">
                  <td className="px-3 py-3 font-medium text-slate-900">{record.type}</td>
                  <td className="px-3 py-3 text-slate-500">{record.host}</td>
                  <td className="px-3 py-3 text-slate-500">{record.actual || record.expected}</td>
                  <td className="px-3 py-3"><Pill active={record.verified}>{record.verified ? "已通过" : (record.message || "待配置")}</Pill></td>
                  <td className="px-3 py-3 text-right">
                    <button type="button" title="复制建议值" className="text-[#009BF5] hover:text-[#008AE6]" onClick={() => copy(record.expected)}>
                      <Copy size={16} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      <Panel
        title="邮件服务"
        action={(
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="secondary" onClick={onApplyStalwartPreset} disabled={loading}>套用 Stalwart 预设</Button>
            <Button type="button" variant="secondary" onClick={onTestMailServer} disabled={loading}>测试连接</Button>
          </div>
        )}
      >
        <div className="grid gap-4 md:grid-cols-2">
          <Field
            label="服务类型"
            value={server.provider || "stalwart"}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), provider: value } }))}
            placeholder="stalwart"
          />
          <Field
            label="服务地址"
            value={server.baseUrl || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), baseUrl: value } }))}
            placeholder="https://mail.example.com"
          />
          <Field
            label="开通接口路径"
            value={server.provisionPath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), provisionPath: value } }))}
            placeholder="/api/v1/mailboxes"
          />
          <Field
            label="账号生命周期接口"
            value={server.lifecyclePath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), lifecyclePath: value } }))}
            placeholder="/api/v1/mailboxes/lifecycle"
          />
          <Field
            label="收件列表接口"
            value={server.messageListPath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), messageListPath: value } }))}
            placeholder="/api/v1/messages"
          />
          <Field
            label="邮件详情接口"
            value={server.messageDetailPath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), messageDetailPath: value } }))}
            placeholder="/api/v1/messages/{messageId}"
          />
          <Field
            label="发信接口"
            value={server.messageSendPath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), messageSendPath: value } }))}
            placeholder="/api/v1/messages/send"
          />
          <Field
            label="草稿接口"
            value={server.draftPath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), draftPath: value } }))}
            placeholder="/api/v1/drafts"
          />
          <Field
            label="星标接口"
            value={server.messageStarPath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), messageStarPath: value } }))}
            placeholder="/api/v1/messages/{messageId}/star"
          />
          <Field
            label="移动接口"
            value={server.messageMovePath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), messageMovePath: value } }))}
            placeholder="/api/v1/messages/{messageId}/move"
          />
          <Field
            label="已读接口"
            value={server.messageReadPath || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), messageReadPath: value } }))}
            placeholder="/api/v1/messages/{messageId}/read"
          />
          <Field
            label="管理令牌"
            type="password"
            value={server.adminToken || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), adminToken: value } }))}
            placeholder={server.adminTokenSet ? "已保存，留空不变" : ""}
          />
          <Field
            label="SMTP 主机"
            value={server.smtpHost || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), smtpHost: value } }))}
            placeholder="127.0.0.1"
          />
          <Field
            label="SMTP 端口"
            type="number"
            value={server.smtpPort || 25}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), smtpPort: Number(value) || 25 } }))}
            placeholder="25"
          />
          <Field
            label="SMTP 账号"
            value={server.smtpUsername || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), smtpUsername: value } }))}
            placeholder="可留空"
          />
          <Field
            label="SMTP 密码"
            type="password"
            value={server.smtpPassword || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), smtpPassword: value } }))}
            placeholder={server.smtpPasswordSet ? "已保存，留空不变" : ""}
          />
          <SelectField
            label="SMTP TLS"
            value={server.smtpTlsMode || "auto"}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), smtpTlsMode: value } }))}
	            options={[
	              { value: "auto", label: "自动 STARTTLS" },
	              { value: "none", label: "不启用 TLS" },
	              { value: "starttls", label: "必须 STARTTLS" },
	              { value: "tls", label: "隐式 TLS / 465" },
	            ]}
	          />
          <Field
            label="IMAP 主机"
            value={server.imapHost || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), imapHost: value } }))}
            placeholder="127.0.0.1"
          />
          <Field
            label="IMAP 端口"
            type="number"
            value={server.imapPort || 993}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), imapPort: Number(value) || 993 } }))}
            placeholder="993"
          />
          <Field
            label="IMAP 账号模板"
            value={server.imapUsername || "{email}"}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), imapUsername: value } }))}
            placeholder="{email}"
          />
          <Field
            label="IMAP 密码"
            type="password"
            value={server.imapPassword || ""}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), imapPassword: value } }))}
            placeholder={server.imapPasswordSet ? "已保存，留空不变" : ""}
          />
          <SelectField
            label="IMAP TLS"
            value={server.imapTlsMode || "tls"}
            onChange={(value) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), imapTlsMode: value } }))}
            options={[
              { value: "tls", label: "隐式 TLS / 993" },
              { value: "auto", label: "自动 STARTTLS" },
              { value: "starttls", label: "必须 STARTTLS" },
              { value: "none", label: "不启用 TLS" },
            ]}
          />
          <Toggle
            label="启用邮箱开通"
            checked={Boolean(server.enabled)}
            onChange={(checked) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), enabled: checked } }))}
          />
          <Toggle
            label="启用 SMTP 发信"
            checked={Boolean(server.smtpEnabled)}
            onChange={(checked) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), smtpEnabled: checked } }))}
          />
          <Toggle
            label="启用 IMAP 收件"
            checked={Boolean(server.imapEnabled)}
            onChange={(checked) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), imapEnabled: checked } }))}
          />
          <Toggle
            label="强制真实数据面"
            checked={Boolean(server.strictDataPlane)}
            onChange={(checked) => setMailbox((current) => ({ ...current, server: { ...(current.server || {}), strictDataPlane: checked } }))}
          />
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-4">
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">连接状态</div>
            <div className="mt-1 font-semibold text-slate-900">{mailServerStatusLabel(server.status)}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">上次测试</div>
            <div className="mt-1 font-semibold text-slate-900">{formatTime(server.lastCheckedAt)}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">错误信息</div>
            <div className="mt-1 truncate font-semibold text-slate-900">{server.lastError || "-"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">数据面模式</div>
            <div className="mt-1 font-semibold text-slate-900">{server.strictDataPlane ? "严格真实接入" : "标准真实接入"}</div>
          </div>
        </div>
      </Panel>
    </div>
  );
}

function AuthPage({ auth, setAuth, security, setSecurity, onSave, loading }) {
  const oauthScopesText = Array.isArray(auth.oauthScopes) ? auth.oauthScopes.join(" ") : (auth.oauthScopes || "");
  return (
    <div className="space-y-5">
      <Panel title="登录注册" action={<Button onClick={onSave} disabled={loading}>保存</Button>}>
        <div className="grid gap-3 md:grid-cols-2">
          <Toggle label="OAuth 登录" checked={auth.oauthEnabled !== false} onChange={(checked) => setAuth((current) => ({ ...current, oauthEnabled: checked }))} />
          <Toggle label="手机号登录注册" checked={auth.phoneLoginEnabled !== false} onChange={(checked) => setAuth((current) => ({ ...current, phoneLoginEnabled: checked }))} />
          <Toggle label="邮箱登录注册" checked={Boolean(auth.emailLoginEnabled)} onChange={(checked) => setAuth((current) => ({ ...current, emailLoginEnabled: checked }))} />
          <Toggle label="开放注册" checked={auth.registrationEnabled !== false} onChange={(checked) => setAuth((current) => ({ ...current, registrationEnabled: checked }))} />
          <Toggle label="注册必须使用注册码" checked={auth.inviteRequired !== false} onChange={(checked) => setAuth((current) => ({ ...current, inviteRequired: checked }))} />
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <SelectField
            label="用户端首页入口"
            value={auth.loginLandingMode || "oauth"}
            onChange={(value) => setAuth((current) => ({ ...current, loginLandingMode: value }))}
            options={[
              { value: "oauth", label: "OAuth 一键登录按钮" },
              { value: "account", label: "账号登录注册入口" },
            ]}
          />
          <Field label="OAuth 展示名称" value={auth.oauthProviderName || ""} onChange={(value) => setAuth((current) => ({ ...current, oauthProviderName: value }))} />
        </div>
        <div className="mt-5 border-t border-slate-100 pt-5">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <div className="text-sm font-semibold text-slate-900">真实 OAuth Provider</div>
              <div className="mt-1 text-xs text-slate-500">配置后用户端一键登录会跳转到真实授权页；生产严格模式下未配置会被拦截。</div>
            </div>
            <Pill active={Boolean(auth.oauthClientId && auth.oauthAuthorizeUrl && auth.oauthTokenUrl && auth.oauthUserInfoUrl)}>OAuth</Pill>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Client ID" value={auth.oauthClientId || ""} onChange={(value) => setAuth((current) => ({ ...current, oauthClientId: value }))} />
            <Field label="Client Secret" type="password" value={auth.oauthClientSecret || ""} onChange={(value) => setAuth((current) => ({ ...current, oauthClientSecret: value }))} placeholder={auth.oauthClientSecretSet ? "已保存，留空不变" : ""} />
            <Field label="授权地址" value={auth.oauthAuthorizeUrl || ""} onChange={(value) => setAuth((current) => ({ ...current, oauthAuthorizeUrl: value }))} placeholder="https://id.example.com/oauth/authorize" />
            <Field label="Token 地址" value={auth.oauthTokenUrl || ""} onChange={(value) => setAuth((current) => ({ ...current, oauthTokenUrl: value }))} placeholder="https://id.example.com/oauth/token" />
            <Field label="用户信息地址" value={auth.oauthUserInfoUrl || ""} onChange={(value) => setAuth((current) => ({ ...current, oauthUserInfoUrl: value }))} placeholder="https://id.example.com/oauth/userinfo" />
            <Field label="回调地址" value={auth.oauthRedirectUrl || ""} onChange={(value) => setAuth((current) => ({ ...current, oauthRedirectUrl: value }))} placeholder="http://localhost:1666/api/v1/post-office/auth/oauth/callback" />
            <Field label="Scope" value={oauthScopesText} onChange={(value) => setAuth((current) => ({ ...current, oauthScopes: normalizeList(value) }))} placeholder="openid profile email phone" />
            <Field label="Subject 字段" value={auth.oauthSubjectField || "sub"} onChange={(value) => setAuth((current) => ({ ...current, oauthSubjectField: value }))} />
            <Field label="手机号字段" value={auth.oauthPhoneField || "phone"} onChange={(value) => setAuth((current) => ({ ...current, oauthPhoneField: value }))} />
            <Field label="邮箱字段" value={auth.oauthEmailField || "email"} onChange={(value) => setAuth((current) => ({ ...current, oauthEmailField: value }))} />
            <Field label="姓名字段" value={auth.oauthNameField || "name"} onChange={(value) => setAuth((current) => ({ ...current, oauthNameField: value }))} />
          </div>
        </div>
      </Panel>

      <Panel title="后台安全" action={<Button onClick={onSave} disabled={loading}>保存</Button>}>
        <div className="mb-4 grid gap-3 md:grid-cols-3">
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">管理员密码</div>
            <div className="mt-1 font-semibold text-slate-900">{security.passwordSet ? "已设置" : "未设置"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">API Token</div>
            <div className="mt-1 font-semibold text-slate-900">{security.apiTokenSet ? (security.apiTokenMasked || "已设置") : "未设置"}</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="text-slate-500">管理员账号</div>
            <div className="mt-1 font-semibold text-slate-900">{security.username || "admin"}</div>
          </div>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="管理员账号" value={security.username || "admin"} onChange={(value) => setSecurity((current) => ({ ...current, username: value }))} />
          <Field label="新管理员密码" type="password" value={security.newPassword || ""} onChange={(value) => setSecurity((current) => ({ ...current, newPassword: value }))} placeholder={security.passwordSet ? "已设置，留空不变" : "至少 8 位"} />
          <Field label="后台 API Token" type="password" value={security.apiToken || ""} onChange={(value) => setSecurity((current) => ({ ...current, apiToken: value }))} placeholder={security.apiTokenSet ? "已保存，留空不变" : "至少 16 位"} />
          <Toggle label="清空后台 API Token" checked={Boolean(security.clearApiToken)} onChange={(checked) => setSecurity((current) => ({ ...current, clearApiToken: checked }))} />
        </div>
      </Panel>
    </div>
  );
}

function SmsPage({ adminConfig, sms, setSms, onSave, loading }) {
  return (
    <div className="grid gap-5 xl:grid-cols-[1fr_360px]">
      <Panel title="短信验证码" action={<Button onClick={onSave} disabled={loading}>保存</Button>}>
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="AccessKey ID" value={sms.accessKeyId || ""} onChange={(value) => setSms((current) => ({ ...current, accessKeyId: value }))} />
          <Field label="AccessKey Secret" type="password" value={sms.accessKeySecret || ""} onChange={(value) => setSms((current) => ({ ...current, accessKeySecret: value }))} placeholder={sms.accessKeySecretSet ? "已保存，留空不变" : ""} />
          <Field label="短信签名" value={sms.signName || ""} onChange={(value) => setSms((current) => ({ ...current, signName: value }))} />
          <Field label="模板 Code" value={sms.templateCode || ""} onChange={(value) => setSms((current) => ({ ...current, templateCode: value }))} />
          <Field label="有效期（分钟）" type="number" value={sms.codeTtlMinutes || 5} onChange={(value) => setSms((current) => ({ ...current, codeTtlMinutes: value }))} />
          <Toggle label="启用阿里云短信" checked={Boolean(sms.aliyunEnabled)} onChange={(checked) => setSms((current) => ({ ...current, aliyunEnabled: checked }))} />
        </div>
      </Panel>

      <Panel title="验证码日志">
        <div className="space-y-2">
          {(adminConfig.smsLogs || []).length ? adminConfig.smsLogs.map((log) => (
            <div key={log.id} className="rounded-md border border-slate-200 p-3 text-sm">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium text-slate-900">{log.phone}</span>
                <Pill active>{log.code || log.codeMasked || "已隐藏"}</Pill>
              </div>
              <div className="mt-1 text-xs text-slate-500">{log.purpose} · {log.provider} · {formatTime(log.createdAt)}</div>
            </div>
          )) : <Empty>暂无验证码日志</Empty>}
        </div>
      </Panel>
    </div>
  );
}

function InvitesPage({ adminConfig, inviteDraft, selectedPrefix, setInviteDraft, createdInvite, createInvite, copy }) {
  const allowedPrefixes = Array.isArray(adminConfig.mailbox?.allowedPrefixes) ? adminConfig.mailbox.allowedPrefixes : ["user"];
  return (
    <div className="grid gap-5 xl:grid-cols-[360px_1fr]">
      <Panel title="生成注册码">
        <form className="space-y-4" onSubmit={createInvite}>
          {adminConfig.mailbox?.prefixPolicyEnabled !== false ? (
            <label className="block">
              <span className="block text-sm font-medium text-slate-700">邮箱前缀</span>
              <select value={selectedPrefix} onChange={(event) => setInviteDraft((current) => ({ ...current, prefix: event.target.value }))} className={inputClass()}>
                {allowedPrefixes.map((prefix) => <option key={prefix} value={prefix}>{prefix}-</option>)}
              </select>
            </label>
          ) : null}
          <Field label="邮箱名" value={inviteDraft.emailPrefix} onChange={(value) => setInviteDraft((current) => ({ ...current, emailPrefix: value }))} placeholder="例如 chenghong" />
          <Field label="绑定手机号" value={inviteDraft.phone} onChange={(value) => setInviteDraft((current) => ({ ...current, phone: value }))} placeholder="可选" />
          <Field label="有效期天数" type="number" value={inviteDraft.expiresInDays} onChange={(value) => setInviteDraft((current) => ({ ...current, expiresInDays: value }))} />
          <Field label="备注" value={inviteDraft.note} onChange={(value) => setInviteDraft((current) => ({ ...current, note: value }))} />
          <Button className="w-full" type="submit">生成注册码</Button>
        </form>
        {createdInvite ? (
          <div className="mt-4 rounded-md border border-[#B3E0FF] bg-[#E5F5FF] p-3">
            <div className="text-sm font-medium text-slate-900">{createdInvite.email}</div>
            <div className="mt-1 break-all text-xs text-slate-500">{createdInvite.url}</div>
            <Button type="button" variant="secondary" className="mt-3 w-full" onClick={() => copy(createdInvite.url)}>
              <Copy size={16} /> 复制链接
            </Button>
          </div>
        ) : null}
      </Panel>

      <Panel title="注册码记录">
        {(adminConfig.invites || []).length ? (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[680px] text-left text-sm">
              <thead className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                <tr className="border-b border-slate-200">
                  <th className="px-3 py-2">邮箱</th>
                  <th className="px-3 py-2">注册码</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">创建时间</th>
                  <th className="px-3 py-2 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {adminConfig.invites.map((invite) => (
                  <tr key={invite.id} className="border-b border-slate-100">
                    <td className="px-3 py-3 font-medium text-slate-900">{invite.email}</td>
                    <td className="px-3 py-3 text-slate-500">{invite.code}</td>
                    <td className="px-3 py-3"><Pill active={!invite.usedAt}>{invite.usedAt ? "已使用" : "未使用"}</Pill></td>
                    <td className="px-3 py-3 text-slate-500">{formatTime(invite.createdAt)}</td>
                    <td className="px-3 py-3 text-right">
                      <button type="button" className="text-[#009BF5] hover:text-[#008AE6]" onClick={() => copy(invite.url)}>
                        <Copy size={16} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : <Empty>还没有注册码记录</Empty>}
      </Panel>
    </div>
  );
}

function AccountsPage({ adminConfig, actions }) {
  return (
    <Panel title="账号记录">
      {(adminConfig.registeredUsers || []).length ? (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[980px] text-left text-sm">
            <thead className="text-xs font-semibold uppercase tracking-wider text-slate-400">
              <tr className="border-b border-slate-200">
                <th className="px-3 py-2">邮箱</th>
                <th className="px-3 py-2">手机号</th>
                <th className="px-3 py-2">状态</th>
                <th className="px-3 py-2">邮箱开通</th>
                <th className="px-3 py-2">注册时间</th>
                <th className="px-3 py-2">来源</th>
                <th className="px-3 py-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {adminConfig.registeredUsers.map((user) => {
                const disabled = user.status === "disabled";
                return (
                  <tr key={user.id} className="border-b border-slate-100">
                    <td className="px-3 py-3 font-medium text-slate-900">{user.email}</td>
                    <td className="px-3 py-3 text-slate-500">{user.phone || "-"}</td>
                    <td className="px-3 py-3"><Pill active={!disabled}>{disabled ? "已禁用" : "正常"}</Pill></td>
                    <td className="px-3 py-3"><Pill active={user.mailboxStatus === "provisioned"}>{mailboxStatusLabel(user.mailboxStatus)}</Pill></td>
                    <td className="px-3 py-3 text-slate-500">{formatTime(user.registeredAt)}</td>
                    <td className="px-3 py-3"><Pill>{user.source || "password"}</Pill></td>
                    <td className="px-3 py-3">
                      <div className="flex justify-end gap-2">
                        <button
                          type="button"
                          title={disabled ? "启用账号" : "禁用账号"}
                          className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 text-slate-500 hover:border-[#009BF5] hover:text-[#009BF5]"
                          onClick={() => (disabled ? actions.enableMailboxAccount(user.id) : actions.disableMailboxAccount(user.id))}
                        >
                          {disabled ? <Unlock size={15} /> : <Lock size={15} />}
                        </button>
                        <button
                          type="button"
                          title="重置临时密码"
                          className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 text-slate-500 hover:border-[#009BF5] hover:text-[#009BF5]"
                          onClick={() => actions.resetMailboxAccountPassword(user.id)}
                        >
                          <KeyRound size={15} />
                        </button>
                        <button
                          type="button"
                          title="重试邮箱开通"
                          className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 text-slate-500 hover:border-[#009BF5] hover:text-[#009BF5]"
                          onClick={() => actions.retryMailboxProvision(user.id)}
                        >
                          <Mail size={15} />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : <Empty>暂无注册用户</Empty>}
    </Panel>
  );
}

function OperationsPage({ adminConfig, actions, loading }) {
  const jobs = adminConfig.provisionJobs || [];
  const ops = adminConfig.ops || {};
  const [opsDraft, setOpsDraft] = useState({
    autoRunEnabled: Boolean(ops.autoRunEnabled),
    intervalMinutes: Number(ops.intervalMinutes || 5),
  });
  const queued = jobs.filter((job) => job.status === "queued" || job.status === "running").length;
  const failed = jobs.filter((job) => job.status === "failed" || job.status === "blocked").length;
  const succeeded = jobs.filter((job) => job.status === "succeeded").length;

  useEffect(() => {
    setOpsDraft({
      autoRunEnabled: Boolean(ops.autoRunEnabled),
      intervalMinutes: Number(ops.intervalMinutes || 5),
    });
  }, [ops.autoRunEnabled, ops.intervalMinutes]);

  const saveOps = async () => {
    await actions.saveAdminConfig({
      ops: {
        autoRunEnabled: Boolean(opsDraft.autoRunEnabled),
        intervalMinutes: Math.max(1, Number(opsDraft.intervalMinutes || 5)),
      },
    });
  };

  return (
    <div className="space-y-5">
      <div className="grid gap-4 md:grid-cols-3">
        <Stat label="待开通任务" value={queued} />
        <Stat label="异常任务" value={failed} />
        <Stat label="已完成任务" value={succeeded} />
      </div>

      <Panel
        title="任务巡检"
        action={<Button type="button" onClick={actions.runOperationalTasks} disabled={loading}>执行任务</Button>}
      >
        <div className="grid gap-3 md:grid-cols-3">
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="font-medium text-slate-900">配置巡检</div>
            <div className="mt-1 text-slate-500">刷新邮箱服务与任务状态</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="font-medium text-slate-900">邮箱开通</div>
            <div className="mt-1 text-slate-500">处理注册后进入队列的邮箱账号</div>
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="font-medium text-slate-900">失败重试</div>
            <div className="mt-1 text-slate-500">邮件服务连通后可重新执行异常任务</div>
          </div>
        </div>
        <div className="mt-4 grid gap-4 border-t border-slate-100 pt-4 lg:grid-cols-[1.1fr_0.9fr]">
          <div className="grid gap-3 sm:grid-cols-2">
            <Toggle
              label="自动执行"
              checked={opsDraft.autoRunEnabled}
              onChange={(checked) => setOpsDraft((current) => ({ ...current, autoRunEnabled: checked }))}
            />
            <Field
              label="间隔分钟"
              type="number"
              value={opsDraft.intervalMinutes}
              onChange={(value) => setOpsDraft((current) => ({ ...current, intervalMinutes: value }))}
            />
          </div>
          <div className="rounded-md border border-slate-200 p-3 text-sm">
            <div className="flex items-center justify-between gap-3">
              <span className="font-medium text-slate-900">最近执行</span>
              <Pill active={ops.lastRunStatus === "success"}>{opsStatusLabel(ops.lastRunStatus)}</Pill>
            </div>
            <div className="mt-2 text-slate-500">{formatTime(ops.lastRunAt)} · {ops.lastRunMessage || "-"}</div>
            <Button type="button" className="mt-3" onClick={saveOps} disabled={loading}>保存自动执行</Button>
          </div>
        </div>
      </Panel>

      <Panel title="邮箱开通队列">
        {jobs.length ? (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[860px] text-left text-sm">
              <thead className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                <tr className="border-b border-slate-200">
                  <th className="px-3 py-2">邮箱</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">尝试</th>
                  <th className="px-3 py-2">下次执行</th>
                  <th className="px-3 py-2">更新时间</th>
                  <th className="px-3 py-2">说明</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => (
                  <tr key={job.id} className="border-b border-slate-100">
                    <td className="px-3 py-3 font-medium text-slate-900">{job.email || "-"}</td>
                    <td className="px-3 py-3"><Pill active={job.status === "succeeded"}>{provisionJobStatusLabel(job.status)}</Pill></td>
                    <td className="px-3 py-3 text-slate-500">{job.attempts || 0}</td>
                    <td className="px-3 py-3 text-slate-500">{formatTime(job.nextRunAt)}</td>
                    <td className="px-3 py-3 text-slate-500">{formatTime(job.updatedAt)}</td>
                    <td className="px-3 py-3 text-slate-500">{job.lastError || (job.completedAt ? "已开通" : "-")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : <Empty>暂无开通任务</Empty>}
      </Panel>
    </div>
  );
}

function AuditPage({ adminConfig }) {
  const items = adminConfig.auditLogs || [];
  return (
    <Panel title="审计日志">
      {items.length ? (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-left text-sm">
            <thead className="text-xs font-semibold uppercase tracking-wider text-slate-400">
              <tr className="border-b border-slate-200">
                <th className="px-3 py-2">时间</th>
                <th className="px-3 py-2">角色</th>
                <th className="px-3 py-2">动作</th>
                <th className="px-3 py-2">对象</th>
                <th className="px-3 py-2">说明</th>
                <th className="px-3 py-2">IP</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id} className="border-b border-slate-100">
                  <td className="px-3 py-3 text-slate-500">{formatTime(item.createdAt)}</td>
                  <td className="px-3 py-3"><Pill active={item.actor === "admin"}>{item.actor || "system"}</Pill></td>
                  <td className="px-3 py-3 font-medium text-slate-900">{item.action}</td>
                  <td className="px-3 py-3 text-slate-500">{item.target || "-"}</td>
                  <td className="px-3 py-3 text-slate-500">{item.detail || "-"}</td>
                  <td className="px-3 py-3 text-slate-400">{item.ip || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : <Empty>暂无审计日志</Empty>}
    </Panel>
  );
}

export function AdminConsoleView() {
  const { adminConfig, adminAuth, adminConfigLoading, notice, actions } = usePostOffice();
  const [activeNav, setActiveNav] = useState(readInitialAdminNav);
  const [mailbox, setMailbox] = useState(adminConfig.mailbox || {});
  const [auth, setAuth] = useState(adminConfig.auth || {});
  const [sms, setSms] = useState(adminConfig.sms || {});
  const [security, setSecurity] = useState(adminConfig.security || { username: "admin" });
  const [loginDraft, setLoginDraft] = useState({ username: "admin", password: "" });
  const [inviteDraft, setInviteDraft] = useState({ prefix: "", emailPrefix: "", phone: "", note: "", expiresInDays: "7" });
  const [createdInvite, setCreatedInvite] = useState(null);

  const allowedPrefixText = useMemo(() => normalizeList(mailbox.allowedPrefixes).join(", "), [mailbox.allowedPrefixes]);
  const allowedPrefixes = Array.isArray(adminConfig.mailbox?.allowedPrefixes) ? adminConfig.mailbox.allowedPrefixes : ["user"];
  const selectedPrefix = inviteDraft.prefix || adminConfig.mailbox?.defaultPrefix || allowedPrefixes[0] || "user";

  function navigateAdmin(itemId) {
    setActiveNav(itemId);
    if (typeof window !== "undefined") {
      try {
        window.sessionStorage?.setItem(adminNavStorageKey, itemId);
      } catch {
        // URL hash below is the durable fallback in restricted browsers.
      }
      if (window.location.hash !== `#${itemId}`) {
        window.history.replaceState(null, "", `#${itemId}`);
      }
    }
  }

  useEffect(() => {
    let active = true;
    actions.loadAdminAuthSession().then((authSession) => {
      if (!active) {
        return;
      }
      if (!authSession?.authRequired || authSession?.isAuthenticated) {
        actions.loadAdminConfig();
      }
    });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") {
      return undefined;
    }
    const syncHashNav = () => {
      const hashNav = window.location.hash.replace(/^#/, "");
      if (adminNavIds.has(hashNav)) {
        setActiveNav(hashNav);
        try {
          window.sessionStorage?.setItem(adminNavStorageKey, hashNav);
        } catch {
          // Hash remains the durable navigation state.
        }
      }
    };
    syncHashNav();
    window.addEventListener("hashchange", syncHashNav);
    window.addEventListener("popstate", syncHashNav);
    return () => {
      window.removeEventListener("hashchange", syncHashNav);
      window.removeEventListener("popstate", syncHashNav);
    };
  }, []);

  useEffect(() => {
    setMailbox(adminConfig.mailbox || {});
    setAuth(adminConfig.auth || {});
    setSms(adminConfig.sms || {});
    setSecurity(adminConfig.security || { username: "admin" });
  }, [adminConfig]);

  async function saveMailbox() {
    await actions.saveAdminConfig({
      mailbox: {
        ...mailbox,
        allowedPrefixes: normalizeList(mailbox.allowedPrefixes || allowedPrefixText),
      },
    });
  }

  async function applyStalwartPreset() {
    const nextMailbox = {
      ...mailbox,
      server: buildStalwartPresetServer(mailbox.server || {}),
    };
    setMailbox(nextMailbox);
    await actions.saveAdminConfig({
      mailbox: {
        ...nextMailbox,
        allowedPrefixes: normalizeList(nextMailbox.allowedPrefixes || allowedPrefixText),
      },
    });
  }

  async function saveAuth() {
    await actions.saveAdminConfig({
      auth: {
        ...auth,
        passwordLoginEnabled: auth.phoneLoginEnabled !== false || Boolean(auth.emailLoginEnabled),
      },
      security: {
        username: security.username || "admin",
        newPassword: security.newPassword || "",
        apiToken: security.apiToken || "",
        clearApiToken: Boolean(security.clearApiToken),
      },
    });
  }

  async function createInvite(event) {
    event.preventDefault();
    const invite = await actions.createMailboxInvite({
      ...inviteDraft,
      prefix: selectedPrefix,
      expiresInDays: Number(inviteDraft.expiresInDays || 0),
    });
    if (invite) {
      setCreatedInvite(invite);
      setInviteDraft((current) => ({ ...current, emailPrefix: "", phone: "", note: "" }));
    }
  }

  async function saveSMS() {
    await actions.saveAdminConfig({
      sms: {
        ...sms,
        codeTtlMinutes: Number(sms.codeTtlMinutes || 5),
      },
    });
  }

  function copy(value) {
    navigator.clipboard?.writeText(value);
  }

  async function loginAdmin(event) {
    event.preventDefault();
    const session = await actions.loginAdmin(loginDraft);
    if (session?.isAuthenticated) {
      setLoginDraft((current) => ({ ...current, password: "" }));
    }
  }

  function renderPage() {
    switch (activeNav) {
      case "mailbox":
        return <MailboxPage mailbox={mailbox} setMailbox={setMailbox} allowedPrefixText={allowedPrefixText} onSave={saveMailbox} onApplyStalwartPreset={applyStalwartPreset} onVerifyDNS={actions.verifyMailboxDomain} onTestMailServer={actions.testMailServer} loading={adminConfigLoading} copy={copy} />;
      case "auth":
        return <AuthPage auth={auth} setAuth={setAuth} security={security} setSecurity={setSecurity} onSave={saveAuth} loading={adminConfigLoading} />;
      case "sms":
        return <SmsPage adminConfig={adminConfig} sms={sms} setSms={setSms} onSave={saveSMS} loading={adminConfigLoading} />;
      case "invites":
        return (
          <InvitesPage
            adminConfig={adminConfig}
            inviteDraft={inviteDraft}
            selectedPrefix={selectedPrefix}
            setInviteDraft={setInviteDraft}
            createdInvite={createdInvite}
            createInvite={createInvite}
            copy={copy}
          />
        );
      case "accounts":
        return <AccountsPage adminConfig={adminConfig} actions={actions} />;
      case "ops":
        return <OperationsPage adminConfig={adminConfig} actions={actions} loading={adminConfigLoading} />;
      case "audit":
        return <AuditPage adminConfig={adminConfig} />;
      case "overview":
      default:
        return <OverviewPage adminConfig={adminConfig} />;
    }
  }

  if (!adminAuth.checked) {
    return (
      <div className="flex h-screen w-full items-center justify-center bg-slate-50 text-sm text-slate-500">
        正在连接管理后台...
      </div>
    );
  }

  if (adminAuth.authRequired && !adminAuth.isAuthenticated) {
    return (
      <AdminLoginScreen
        adminAuth={adminAuth}
        loginDraft={loginDraft}
        setLoginDraft={setLoginDraft}
        onLogin={loginAdmin}
      />
    );
  }

  return (
    <div className="h-screen w-full flex overflow-hidden bg-white font-sans text-slate-900 antialiased">
      <aside className="w-64 bg-white border-r border-slate-200 flex flex-col z-10 shrink-0">
        <div className="h-14 flex items-center px-6 border-b border-slate-200">
          <div className="flex items-center gap-2 text-[#009BF5] font-bold text-lg tracking-wide">
            <Mail size={22} strokeWidth={2.5} />
            <span className="text-slate-800">悦享邮局</span>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-3 py-2 space-y-1">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2 px-3 mt-2">后台</div>
          {adminNav.map((item) => (
            <NavItem key={item.id} item={item} activeNav={activeNav} onNavigate={navigateAdmin} />
          ))}
        </div>

        <div className="p-4 border-t border-slate-200">
          {adminAuth.authRequired ? (
            <button
              type="button"
              onClick={actions.logoutAdmin}
              className="mb-2 flex w-full items-center justify-center gap-2 rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50"
            >
              <Unlock size={16} />
              退出后台
            </button>
          ) : null}
          <a href={runtimeConfig.userAppOrigin} className="block rounded-md border border-slate-300 px-3 py-2 text-center text-sm font-medium text-slate-700 hover:bg-slate-50">
            返回用户端
          </a>
        </div>
      </aside>

      <div className="flex-1 flex flex-col min-w-0">
        <header className="h-14 bg-white border-b border-slate-200 flex items-center justify-between px-6 shrink-0">
          <div className="flex items-center gap-3">
            <Settings2 size={18} className="text-slate-400" />
            <h1 className="text-base font-semibold text-slate-900">{navTitle[activeNav] || "后台"}</h1>
          </div>
          <div className="flex items-center gap-2 text-xs text-slate-500">
            <Pill>@{adminConfig.mailbox?.domain}</Pill>
            <Pill active={adminConfig.auth?.inviteRequired !== false}>{adminConfig.auth?.inviteRequired === false ? "开放注册" : "注册码注册"}</Pill>
          </div>
        </header>

        {notice ? (
          <div className="border-b border-[#B3E0FF] bg-[#E5F5FF] px-6 py-2 text-sm text-[#007ACC]">
            {notice.message}
          </div>
        ) : null}

        <main className="flex-1 overflow-y-auto bg-slate-50">
          <div className="mx-auto max-w-6xl px-6 py-6">
            {renderPage()}
          </div>
        </main>
      </div>
    </div>
  );
}
