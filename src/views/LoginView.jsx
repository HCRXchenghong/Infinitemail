import React, { useEffect, useMemo, useState } from "react";
import { AlertCircle, ArrowLeft, Eye, EyeOff, KeyRound, Lock, Mail, Phone, ShieldCheck, User } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Button } from "../components/ui/Button";
import { runtimeConfig } from "../lib/config/runtime";

function TextField({ label, value, onChange, type = "text", placeholder = "", autoComplete = "off", large = false, icon: Icon, trailing }) {
  return (
    <label className="block">
      <span className={large ? "block text-xl font-bold leading-tight text-slate-700 sm:text-[28px]" : "block text-sm font-medium text-slate-700"}>{label}</span>
      <div className="relative mt-1">
        {Icon ? <Icon size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" /> : null}
        <input
          type={type}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          autoComplete={autoComplete}
          placeholder={placeholder}
          className={[
            "block w-full rounded-md border border-slate-300 bg-white shadow-sm focus:border-[#009BF5] focus:outline-none focus:ring-2 focus:ring-[#009BF5]/20",
            Icon ? "pl-9" : "px-3",
            trailing ? "pr-10" : "pr-3",
            large ? "h-16 text-xl font-semibold placeholder:text-slate-400 sm:h-[84px] sm:text-[30px]" : "h-10 text-sm",
          ].join(" ")}
        />
        {trailing}
      </div>
    </label>
  );
}

function AuthModeButton({ active, children, onClick }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        "flex-1 rounded-md px-3 py-2 text-sm font-semibold transition-colors",
        active ? "bg-[#009BF5] text-white shadow-sm" : "bg-white text-slate-600 hover:text-slate-900",
      ].join(" ")}
    >
      {children}
    </button>
  );
}

function MethodButton({ active, children, onClick }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        "rounded-md border px-3 py-1.5 text-sm font-medium transition-colors",
        active ? "border-[#009BF5] bg-[#E5F5FF] text-[#009BF5]" : "border-slate-200 bg-white text-slate-500 hover:text-slate-900",
      ].join(" ")}
    >
      {children}
    </button>
  );
}

function normalizeAuthConfig(authConfig = {}) {
  const phoneLoginEnabled = authConfig.phoneLoginEnabled !== false;
  const emailLoginEnabled = Boolean(authConfig.emailLoginEnabled);
  const passwordLoginEnabled = authConfig.passwordLoginEnabled !== false && (phoneLoginEnabled || emailLoginEnabled);
  let loginLandingMode = authConfig.loginLandingMode === "account" || authConfig.loginLandingMode === "password" ? "account" : "oauth";
  if (loginLandingMode === "oauth" && authConfig.oauthEnabled === false && passwordLoginEnabled) {
    loginLandingMode = "account";
  }
  if (loginLandingMode === "account" && !passwordLoginEnabled && authConfig.oauthEnabled !== false) {
    loginLandingMode = "oauth";
  }
  return {
    oauthEnabled: authConfig.oauthEnabled !== false,
    oauthProviderName: authConfig.oauthProviderName || "悦享账号",
    passwordLoginEnabled,
    phoneLoginEnabled,
    emailLoginEnabled,
    registrationEnabled: authConfig.registrationEnabled !== false,
    inviteRequired: authConfig.inviteRequired !== false,
    loginLandingMode,
  };
}

function provisioningTitle(status) {
  switch (status) {
    case "provisioned":
      return "邮箱已开通";
    case "queued":
      return "邮箱正在开通";
    case "failed":
      return "邮箱开通失败";
    default:
      return "等待后台配置邮件服务";
  }
}

function provisioningDescription(status) {
  switch (status) {
    case "provisioned":
      return "系统正在进入您的邮箱工作台。";
    case "queued":
      return "后台开通任务正在处理中，完成后即可进入邮箱。";
    case "failed":
      return "后台需要重新执行邮箱开通或检查邮件服务配置。";
    default:
      return "邮箱名已保留，管理员配置真实邮件服务后即可开通。";
  }
}

export function LoginView() {
  const { authFlow, adminConfig, actions } = usePostOffice();
  const [showAccountPage, setShowAccountPage] = useState(false);
  const [authMode, setAuthMode] = useState("login");
  const [accountMethod, setAccountMethod] = useState("phone");
  const [emailPrefix, setEmailPrefix] = useState("");
  const [selectedPrefix, setSelectedPrefix] = useState("");
  const [identifierEmail, setIdentifierEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [smsCode, setSmsCode] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [rememberAccount, setRememberAccount] = useState(true);
  const [displayName, setDisplayName] = useState("");
  const [inviteCode, setInviteCode] = useState("");

  const mailboxConfig = adminConfig?.mailbox || {};
  const authConfig = normalizeAuthConfig(adminConfig?.auth || {});
  const mailboxDomain = mailboxConfig.domain || runtimeConfig.mailboxDomain || "yuexiang.com";
  const allowedPrefixes = useMemo(() => (
    Array.isArray(mailboxConfig.allowedPrefixes) && mailboxConfig.allowedPrefixes.length
      ? mailboxConfig.allowedPrefixes
      : ["user"]
  ), [mailboxConfig.allowedPrefixes]);
  const prefixPolicyEnabled = mailboxConfig.prefixPolicyEnabled !== false;
  const defaultPrefix = mailboxConfig.defaultPrefix || allowedPrefixes[0] || "user";
  const activePrefix = selectedPrefix || defaultPrefix;
  const hasAccountMethods = authConfig.passwordLoginEnabled && (authConfig.phoneLoginEnabled || authConfig.emailLoginEnabled);
  const landingMode = authConfig.loginLandingMode === "account" && hasAccountMethods ? "account" : "oauth";
  const accountEntryLabel = authConfig.phoneLoginEnabled && authConfig.emailLoginEnabled
    ? "使用邮箱/手机号登录注册"
    : authConfig.emailLoginEnabled
      ? "使用邮箱登录注册"
      : "使用手机号登录注册";
  const showActivation = authFlow.hasAuthenticatedSession && authFlow.requiresActivation;
  const showProvisioning = authFlow.hasAuthenticatedSession && authFlow.requiresProvisioning;
  const provisioningStatus = authFlow.provisioningStatus || "pending_config";
  const reservedEmail = authFlow.profile?.email || "";

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("invite") || "";
    if (code) {
      setInviteCode(code.toUpperCase());
      setAuthMode("register");
      setShowAccountPage(true);
    }
  }, []);

  useEffect(() => {
    setSelectedPrefix(defaultPrefix);
  }, [defaultPrefix]);

  useEffect(() => {
    if (!authConfig.phoneLoginEnabled && authConfig.emailLoginEnabled) {
      setAccountMethod("email");
      return;
    }
    if (authConfig.phoneLoginEnabled) {
      setAccountMethod((current) => (current === "email" && !authConfig.emailLoginEnabled ? "phone" : current));
    }
  }, [authConfig.emailLoginEnabled, authConfig.phoneLoginEnabled]);

  async function handleEntryClick() {
    if (landingMode === "account") {
      setShowAccountPage(true);
      return;
    }
    const result = await actions.beginUnifiedLogin();
    if (result?.isAuthenticated && result?.requiresActivation) {
      setShowAccountPage(false);
    }
  }

  async function handleActivate() {
    await actions.activateMailbox({ emailPrefix });
  }

  async function handleRefreshStatus() {
    await actions.refreshSession?.();
  }

  async function handleSendCode() {
    await actions.sendAuthSmsCode({ phone, purpose: authMode === "register" ? "register" : "login" });
  }

  async function handlePasswordSubmit(event) {
    event.preventDefault();
    const payload = {
      loginType: accountMethod,
      identifier: accountMethod === "email" ? identifierEmail : phone,
      email: accountMethod === "email" ? identifierEmail : "",
      phone,
      smsCode,
      password,
      displayName,
      emailPrefix,
      prefix: activePrefix,
      inviteCode,
    };

    if (authMode === "register") {
      await actions.registerWithPassword(payload);
      return;
    }

    await actions.loginWithPassword(payload);
  }

  function renderMailboxNameInput({ large = false } = {}) {
    return (
      <div>
        <label className={large ? "block text-xl font-bold leading-tight text-slate-700 sm:text-[28px]" : "block text-sm font-medium text-slate-700"}>邮箱地址</label>
        <div className="mt-1 flex rounded-md shadow-sm">
          {prefixPolicyEnabled ? (
            <select
              value={activePrefix}
              onChange={(event) => setSelectedPrefix(event.target.value)}
              className={[
                "rounded-l-md border border-r-0 border-slate-300 bg-slate-50 text-slate-500 focus:border-[#009BF5] focus:outline-none",
                large ? "px-2 text-base font-semibold sm:px-4 sm:text-[24px]" : "px-3 py-2 text-sm",
              ].join(" ")}
            >
              {allowedPrefixes.map((prefix) => (
                <option value={prefix} key={prefix}>{prefix}-</option>
              ))}
            </select>
          ) : null}
          <input
            type="text"
            value={emailPrefix}
            onChange={(event) => setEmailPrefix(event.target.value)}
            className={[
              "min-w-0 flex-1 border border-slate-300 focus:border-[#009BF5] focus:outline-none focus:ring-2 focus:ring-[#009BF5]/20",
              prefixPolicyEnabled ? "" : "rounded-l-md",
              large ? "h-16 px-4 text-xl font-semibold placeholder:text-slate-400 sm:h-[84px] sm:px-6 sm:text-[30px]" : "px-3 py-2 text-sm",
            ].join(" ")}
            placeholder="输入邮箱名"
          />
          <span className={[
            "inline-flex items-center rounded-r-md border border-l-0 border-slate-300 bg-slate-50 text-slate-500",
            large ? "px-2 text-base font-semibold sm:px-5 sm:text-[24px]" : "px-3 text-sm",
          ].join(" ")}
          >
            @{mailboxDomain}
          </span>
        </div>
      </div>
    );
  }

  const phoneSubmitDisabled = !phone || !smsCode || !password;
  const emailSubmitDisabled = !identifierEmail || !password;
  const registerNeedsMailbox = authMode === "register" && accountMethod === "phone" && !emailPrefix;
  const submitDisabled = authFlow.isLoading || (accountMethod === "phone" ? phoneSubmitDisabled : emailSubmitDisabled) || registerNeedsMailbox;

  if (showProvisioning) {
    return (
      <div className="flex min-h-screen flex-col justify-center bg-white py-12 sm:px-6 lg:px-8">
        <div className="sm:mx-auto sm:w-full sm:max-w-md">
          <div className="flex justify-center text-[#009BF5]">
            <Mail size={48} strokeWidth={1.5} />
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-slate-900">悦享邮局</h2>
          <p className="mt-2 text-center text-sm text-slate-600">专业、安全、互通的公司邮箱系统</p>
        </div>
        <div className="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
          <div className="rounded-lg border border-slate-100 bg-white px-4 py-8 shadow sm:px-10">
            <div className="space-y-6">
              <div>
                <h3 className="text-lg font-medium text-slate-900">{provisioningTitle(provisioningStatus)}</h3>
                <p className="mt-1 text-sm text-slate-500">{provisioningDescription(provisioningStatus)}</p>
              </div>
              {reservedEmail ? (
                <div className="rounded-md border border-slate-200 bg-slate-50 px-3 py-3">
                  <p className="text-xs font-medium text-slate-500">已保留邮箱</p>
                  <p className="mt-1 break-all text-sm font-semibold text-slate-900">{reservedEmail}</p>
                </div>
              ) : null}
              {authFlow.errorMessage ? (
                <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
                  {authFlow.errorMessage}
                </div>
              ) : null}
              <div className="grid gap-3 sm:grid-cols-2">
                <Button className="w-full" onClick={handleRefreshStatus} disabled={authFlow.isLoading}>
                  刷新状态
                </Button>
                <Button type="button" variant="secondary" className="w-full" onClick={actions.logout} disabled={authFlow.isLoading}>
                  退出登录
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (showActivation) {
    return (
      <div className="flex min-h-screen flex-col justify-center bg-white py-12 sm:px-6 lg:px-8">
        <div className="sm:mx-auto sm:w-full sm:max-w-md">
          <div className="flex justify-center text-[#009BF5]">
            <Mail size={48} strokeWidth={1.5} />
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-slate-900">悦享邮局</h2>
          <p className="mt-2 text-center text-sm text-slate-600">专业、安全、互通的公司邮箱系统</p>
        </div>
        <div className="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
          <div className="rounded-lg border border-slate-100 bg-white px-4 py-8 shadow sm:px-10">
            <div className="space-y-6">
              <div>
                <h3 className="text-lg font-medium text-slate-900">开通您的专属邮箱</h3>
                <p className="mt-1 text-sm text-slate-500">检测到您是首次使用，请设置您的邮箱地址。</p>
              </div>
              <div>
                {renderMailboxNameInput()}
                <p className="mt-2 flex items-start gap-1 text-xs text-slate-500">
                  <AlertCircle size={14} className="mt-0.5 shrink-0" />
                  提示：开通后不可修改。账号停用后，后台可以统一回收邮箱。
                </p>
              </div>
              {authFlow.errorMessage ? (
                <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
                  {authFlow.errorMessage}
                </div>
              ) : null}
              <Button className="w-full" onClick={handleActivate} disabled={!emailPrefix || authFlow.isLoading}>
                确认邮箱名并开通
              </Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (!showAccountPage) {
    return (
      <div className="flex min-h-screen flex-col justify-center bg-white py-12 sm:px-6 lg:px-8">
        <div className="sm:mx-auto sm:w-full sm:max-w-md">
          <div className="flex justify-center text-[#009BF5]">
            <Mail size={48} strokeWidth={1.5} />
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-slate-900">悦享邮局</h2>
          <p className="mt-2 text-center text-sm text-slate-600">专业、安全、互通的公司邮箱系统</p>
        </div>

        <div className="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
          <div className="mt-4 space-y-6 px-4 sm:px-0">
            <Button
              className="h-11 w-full bg-[#009BF5] text-base shadow-sm hover:bg-[#008AE6] focus:ring-[#009BF5] disabled:opacity-60"
              onClick={handleEntryClick}
              disabled={authFlow.isLoading || (landingMode === "oauth" && !authConfig.oauthEnabled)}
            >
              {landingMode === "oauth" ? `使用 ${authConfig.oauthProviderName} 账号登录` : accountEntryLabel}
            </Button>
            <div className="relative pt-4">
              <div className="absolute inset-0 flex items-center pt-4">
                <div className="w-full border-t border-slate-200" />
              </div>
              <div className="relative flex justify-center pt-4 text-sm">
                <span className="bg-white px-2 text-slate-500">一个公司，一个邮箱入口</span>
              </div>
            </div>
            {authFlow.errorMessage ? (
              <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
                {authFlow.errorMessage}
              </div>
            ) : null}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col justify-center bg-white py-12 sm:px-6 lg:px-8">
      <div className="mx-auto w-full max-w-md px-4 sm:px-0">
        <div className="mb-3">
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-md border border-slate-200 px-3 py-1.5 text-sm font-medium text-slate-500 hover:text-slate-900"
            onClick={() => setShowAccountPage(false)}
          >
            <ArrowLeft size={16} />
            返回
          </button>
        </div>

        <div className="rounded-xl border border-slate-200 bg-white px-5 py-7 shadow-[0_18px_42px_rgba(15,23,42,0.08)] sm:px-8">
          <div className="text-center">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-[#009BF5] text-white shadow-sm">
              <Mail size={26} strokeWidth={2.4} />
            </div>
            <h3 className="mt-4 text-2xl font-bold tracking-normal text-slate-900">悦享邮局</h3>
            <p className="mt-2 text-sm text-slate-500">专业、安全、互通的公司邮箱系统</p>
          </div>

          <div className="mt-6 rounded-lg border border-slate-200 bg-white p-1">
            <div className="flex">
              <AuthModeButton active={authMode === "login"} onClick={() => setAuthMode("login")}>登录</AuthModeButton>
              <AuthModeButton active={authMode === "register"} onClick={() => setAuthMode("register")}>注册</AuthModeButton>
            </div>
          </div>

          {authConfig.phoneLoginEnabled && authConfig.emailLoginEnabled ? (
            <div className="mt-4 grid grid-cols-2 gap-2">
              <MethodButton active={accountMethod === "phone"} onClick={() => setAccountMethod("phone")}>手机号</MethodButton>
              <MethodButton active={accountMethod === "email"} onClick={() => setAccountMethod("email")}>邮箱</MethodButton>
            </div>
          ) : null}

          <form onSubmit={handlePasswordSubmit} className="mt-5 space-y-4">
            {authMode === "register" ? (
              <>
                <TextField label="姓名" value={displayName} onChange={setDisplayName} placeholder="请输入姓名" icon={User} />
                {accountMethod === "phone" ? (
                  renderMailboxNameInput()
                ) : (
                  <TextField label="邮箱" value={identifierEmail} onChange={setIdentifierEmail} placeholder="请输入邮箱地址" autoComplete="email" icon={Mail} />
                )}
                {authConfig.inviteRequired ? (
                  <TextField label="注册码" value={inviteCode} onChange={(value) => setInviteCode(value.toUpperCase())} placeholder="请输入注册码" icon={KeyRound} />
                ) : null}
              </>
            ) : null}

            {accountMethod === "phone" ? (
              <>
                <TextField label="手机号" value={phone} onChange={setPhone} placeholder="请输入手机号" autoComplete="tel" icon={Phone} />
                <div>
                  <span className="block text-sm font-medium text-slate-700">短信验证码</span>
                  <div className="mt-1 grid gap-3 sm:grid-cols-[1fr_112px]">
                    <div className="relative">
                      <ShieldCheck size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
                      <input
                        value={smsCode}
                        onChange={(event) => setSmsCode(event.target.value)}
                        placeholder="6 位验证码"
                        className="h-10 w-full rounded-md border border-slate-300 bg-white pl-9 pr-3 text-sm shadow-sm placeholder:text-slate-400 focus:border-[#009BF5] focus:outline-none focus:ring-2 focus:ring-[#009BF5]/20"
                      />
                    </div>
                    <Button type="button" variant="secondary" className="h-10 rounded-md px-3 text-sm font-medium text-[#009BF5]" onClick={handleSendCode}>
                      获取验证码
                    </Button>
                  </div>
                </div>
              </>
            ) : authMode === "login" ? (
              <TextField label="邮箱" value={identifierEmail} onChange={setIdentifierEmail} placeholder="请输入邮箱地址" autoComplete="email" icon={Mail} />
            ) : null}

            <TextField
              label="密码"
              type={showPassword ? "text" : "password"}
              value={password}
              onChange={setPassword}
              placeholder="至少 6 位"
              autoComplete={authMode === "login" ? "current-password" : "new-password"}
              icon={Lock}
              trailing={(
                <button
                  type="button"
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 transition-colors hover:text-slate-600"
                  onClick={() => setShowPassword((current) => !current)}
                  aria-label={showPassword ? "隐藏密码" : "显示密码"}
                >
                  {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              )}
            />

            {authMode === "login" ? (
              <div className="flex items-center justify-between text-sm">
                <label className="inline-flex items-center gap-2 text-slate-500">
                  <input
                    type="checkbox"
                    checked={rememberAccount}
                    onChange={(event) => setRememberAccount(event.target.checked)}
                    className="h-4 w-4 rounded border-slate-300 text-[#009BF5] focus:ring-[#009BF5]"
                  />
                  记住账号
                </label>
                <button type="button" className="font-medium text-[#009BF5] hover:text-[#008AE6]">
                  忘记密码？
                </button>
              </div>
            ) : null}

            <Button
              className="h-11 w-full rounded-md bg-[#009BF5] text-sm font-semibold shadow-sm hover:bg-[#008AE6] disabled:opacity-60"
              type="submit"
              disabled={submitDisabled}
            >
              {authMode === "register" ? "注册邮箱" : "登录邮箱"}
            </Button>
          </form>

          <div className="mt-5 flex items-center justify-center gap-3 text-sm text-slate-500">
            {authMode === "login" ? (
              <>
                <span>还没有账号？</span>
                <button type="button" className="font-semibold text-[#009BF5] hover:text-[#008AE6]" onClick={() => setAuthMode("register")}>
                  注册账号
                </button>
                {authConfig.inviteRequired ? <span className="font-semibold text-[#009BF5]">需要注册码</span> : null}
              </>
            ) : (
              <>
                <span>已有账号？</span>
                <button type="button" className="font-semibold text-[#009BF5] hover:text-[#008AE6]" onClick={() => setAuthMode("login")}>
                  返回登录
                </button>
              </>
            )}
          </div>

          {authFlow.errorMessage ? (
            <div className="mt-5 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
              {authFlow.errorMessage}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
