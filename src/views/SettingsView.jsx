import React, { useEffect, useState } from "react";
import { Clock3, Info, LogOut, MonitorSmartphone, RefreshCcw, Settings, Shield } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { postOfficeApi } from "../services/postOfficeApi";

function formatUpdatedAt(value) {
  const date = new Date(value || "");
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  return `${date.getMonth() + 1}月${date.getDate()}日 ${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

function formatSessionTime(value) {
  const date = new Date(value || "");
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return `${date.getMonth() + 1}月${date.getDate()}日 ${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

export function SettingsView() {
  const { settings, profile, actions } = usePostOffice();
  const [securityOpen, setSecurityOpen] = useState(false);
  const [securitySessions, setSecuritySessions] = useState({ items: [], isLoading: false, errorMessage: "" });
  const [form, setForm] = useState({
    defaultSenderName: "",
    signature: "",
    autoReplyEnabled: false,
    autoReplyMessage: "",
  });

  useEffect(() => {
    if (settings.data) {
      setForm(settings.data);
    }
  }, [settings.data]);

  function updateField(field, value) {
    setForm((current) => ({
      ...current,
      [field]: value,
    }));
  }

  function persist(field) {
    if (settings.data?.[field] === form[field]) {
      return;
    }
    actions.saveSettings({
      [field]: form[field],
    });
  }

  async function loadSecuritySessions() {
    setSecuritySessions((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const payload = await postOfficeApi.listSecuritySessions();
      setSecuritySessions({
        items: Array.isArray(payload?.items) ? payload.items : [],
        isLoading: false,
        errorMessage: "",
      });
    } catch (error) {
      setSecuritySessions((current) => ({
        ...current,
        isLoading: false,
        errorMessage: error?.message || "登录设备加载失败，请稍后重试",
      }));
    }
  }

  async function logoutOtherSessions() {
    setSecuritySessions((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const payload = await postOfficeApi.logoutOtherSecuritySessions();
      setSecuritySessions({
        items: Array.isArray(payload?.items) ? payload.items : [],
        isLoading: false,
        errorMessage: "",
      });
    } catch (error) {
      setSecuritySessions((current) => ({
        ...current,
        isLoading: false,
        errorMessage: error?.message || "退出其他设备失败，请稍后重试",
      }));
    }
  }

  async function revokeSession(sessionId) {
    setSecuritySessions((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const payload = await postOfficeApi.revokeSecuritySession(sessionId);
      setSecuritySessions({
        items: Array.isArray(payload?.items) ? payload.items : [],
        isLoading: false,
        errorMessage: "",
      });
    } catch (error) {
      setSecuritySessions((current) => ({
        ...current,
        isLoading: false,
        errorMessage: error?.message || "移除登录设备失败，请稍后重试",
      }));
    }
  }

  function openSecuritySessions() {
    setSecurityOpen(true);
    loadSecuritySessions();
  }

  const otherSessionCount = securitySessions.items.filter((session) => !session.current).length;

  return (
    <div className="h-full bg-white overflow-y-auto">
      <div className="max-w-5xl mx-auto p-8">
        <div className="flex items-center justify-between gap-6 mb-8">
          <h2 className="text-2xl font-bold text-slate-900">邮箱设置</h2>
          <div className="text-sm text-slate-500 whitespace-nowrap">
            {settings.isLoading
              ? "正在保存..."
              : settings.data?.updatedAt
                ? `最近保存 ${formatUpdatedAt(settings.data.updatedAt)}`
                : "变更将自动保存"}
          </div>
        </div>

        <div className="space-y-8">
          <section className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
            <div className="px-6 py-5 border-b border-slate-100 flex items-center gap-2 bg-slate-50/50">
              <Shield className="text-[#009BF5]" size={20} />
              <h3 className="text-lg font-medium text-slate-800">账号与安全</h3>
            </div>
            <div className="p-6 space-y-6">
              <div className="flex items-center justify-between pb-6 border-b border-slate-50">
                <div>
                  <div className="font-medium text-slate-900">当前邮箱地址</div>
                  <div className="text-sm text-slate-500 mt-1">此地址为您的唯一标识，不可修改。</div>
                </div>
                <div className="text-slate-800 font-medium bg-slate-100 px-3 py-1 rounded">{profile?.email || "myname@yuexiang.com"}</div>
              </div>

              <div className="flex items-start justify-between pb-6 border-b border-slate-50 gap-6">
                <div>
                  <div className="font-medium text-slate-900 flex items-center gap-2">
                    邮箱账号绑定 <Badge color="green">已绑定</Badge>
                  </div>
                  <div className="text-sm text-slate-500 mt-1 max-w-lg">
                    您的邮箱已与公司账号({profile?.unifiedAccountPhone || "未绑定手机号"})绑定。后台可按配置启用邮箱、手机号验证码或 OAuth 登录。
                  </div>
                  <div className="mt-3 p-3 bg-[#E5F5FF] text-[#007ACC] text-xs rounded-md flex items-start gap-2 border border-[#B3E0FF]">
                    <Info size={14} className="mt-0.5 shrink-0" />
                    安全规则：账号被后台禁用、启用或重置密码时，BFF 会同步真实邮件服务，避免本地状态与邮箱底座不一致。
                  </div>
                </div>
                <Badge color="blue">系统托管</Badge>
              </div>

              <div className="flex items-center justify-between gap-6">
                <div>
                  <div className="font-medium text-slate-900">登录设备管理</div>
                  <div className="text-sm text-slate-500 mt-1">查看最近登录的设备及 IP 异常。</div>
                </div>
                <Button variant="ghost" onClick={openSecuritySessions}>
                  <MonitorSmartphone size={16} />
                  查看记录
                </Button>
              </div>

              {securityOpen ? (
                <div className="rounded-lg border border-slate-200 overflow-hidden">
                  <div className="px-4 py-3 bg-slate-50 border-b border-slate-200 flex items-center justify-between gap-4">
                    <div>
                      <div className="text-sm font-medium text-slate-900">登录设备记录</div>
                      <div className="text-xs text-slate-500 mt-1">共 {securitySessions.items.length} 个有效登录设备</div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Button
                        type="button"
                        variant="ghost"
                        className="!px-3 !py-1.5"
                        onClick={loadSecuritySessions}
                        disabled={securitySessions.isLoading}
                      >
                        <RefreshCcw size={15} />
                        刷新
                      </Button>
                      <Button
                        type="button"
                        variant="secondary"
                        className="!px-3 !py-1.5"
                        onClick={logoutOtherSessions}
                        disabled={securitySessions.isLoading || otherSessionCount === 0}
                      >
                        <LogOut size={15} />
                        退出其他设备
                      </Button>
                    </div>
                  </div>
                  {securitySessions.errorMessage ? (
                    <div className="border-b border-red-100 bg-red-50 px-4 py-3 text-sm text-red-600">
                      {securitySessions.errorMessage}
                    </div>
                  ) : null}
                  <div className="divide-y divide-slate-100">
                    {securitySessions.isLoading && securitySessions.items.length === 0 ? (
                      <div className="px-4 py-5 text-sm text-slate-500">正在加载登录设备...</div>
                    ) : securitySessions.items.length === 0 ? (
                      <div className="px-4 py-5 text-sm text-slate-500">当前没有有效登录设备。</div>
                    ) : (
                      securitySessions.items.map((session) => (
                        <div key={session.id} className="px-4 py-4 flex items-center justify-between gap-5">
                          <div className="min-w-0 flex items-start gap-3">
                            <div className="mt-0.5 h-9 w-9 shrink-0 rounded-md bg-[#E5F5FF] text-[#009BF5] flex items-center justify-center">
                              <MonitorSmartphone size={18} />
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center gap-2">
                                <div className="font-medium text-slate-900 truncate">{session.device}</div>
                                {session.current ? <Badge color="green">当前设备</Badge> : null}
                              </div>
                              <div className="mt-1 text-xs text-slate-500 truncate">IP：{session.ip || "-"}</div>
                              <div className="mt-1 flex items-center gap-1 text-xs text-slate-400">
                                <Clock3 size={13} />
                                最近使用 {formatSessionTime(session.lastSeenAt)}
                              </div>
                            </div>
                          </div>
                          {session.current ? null : (
                            <Button
                              type="button"
                              variant="ghost"
                              className="shrink-0 !px-3 !py-1.5 text-slate-500"
                              onClick={() => revokeSession(session.id)}
                              disabled={securitySessions.isLoading}
                            >
                              <LogOut size={15} />
                              移除
                            </Button>
                          )}
                        </div>
                      ))
                    )}
                  </div>
                </div>
              ) : null}
            </div>
          </section>

          <section className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
            <div className="px-6 py-5 border-b border-slate-100 flex items-center gap-2 bg-slate-50/50">
              <Settings className="text-slate-600" size={20} />
              <h3 className="text-lg font-medium text-slate-800">邮件收发设置</h3>
            </div>
            <div className="p-6 space-y-6">
              <div className="flex items-start justify-between pb-6 border-b border-slate-50">
                <div className="w-full max-w-2xl">
                  <div className="font-medium text-slate-900 mb-3">默认发件人名称</div>
                  <input
                    type="text"
                    value={form.defaultSenderName}
                    onChange={(event) => updateField("defaultSenderName", event.target.value)}
                    onBlur={() => persist("defaultSenderName")}
                    className="w-full border-slate-300 rounded-md text-sm focus:ring-[#009BF5] focus:border-[#009BF5]"
                  />
                </div>
              </div>

              <div className="flex items-start justify-between pb-6 border-b border-slate-50">
                <div className="w-full max-w-2xl">
                  <div className="font-medium text-slate-900 mb-3">邮件签名</div>
                  <textarea
                    value={form.signature}
                    onChange={(event) => updateField("signature", event.target.value)}
                    onBlur={() => persist("signature")}
                    className="w-full border-slate-300 rounded-md text-sm focus:ring-[#009BF5] focus:border-[#009BF5] min-h-[100px]"
                  ></textarea>
                </div>
              </div>

              <div className="flex items-start justify-between pb-6 border-b border-slate-50">
                <div className="w-full max-w-2xl">
                  <div className="font-medium text-slate-900 mb-3">自动回复内容</div>
                  <textarea
                    value={form.autoReplyMessage}
                    onChange={(event) => updateField("autoReplyMessage", event.target.value)}
                    onBlur={() => persist("autoReplyMessage")}
                    className="w-full border-slate-300 rounded-md text-sm focus:ring-[#009BF5] focus:border-[#009BF5] min-h-[100px]"
                  ></textarea>
                </div>
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-slate-900">自动回复</div>
                  <div className="text-sm text-slate-500 mt-1">在您休假或无法及时回复时自动响应发件人。</div>
                </div>
                <div className="relative inline-block w-10 mr-2 align-middle select-none transition duration-200 ease-in">
                  <input
                    type="checkbox"
                    checked={form.autoReplyEnabled}
                    onChange={(event) => {
                      if (settings.data?.autoReplyEnabled === event.target.checked) {
                        updateField("autoReplyEnabled", event.target.checked);
                        return;
                      }
                      updateField("autoReplyEnabled", event.target.checked);
                      actions.saveSettings({ autoReplyEnabled: event.target.checked });
                    }}
                    name="toggle"
                    id="toggle"
                    className="toggle-checkbox absolute block w-5 h-5 rounded-full bg-white border-4 border-slate-300 appearance-none cursor-pointer transition-transform duration-200 ease-in-out"
                  />
                  <label htmlFor="toggle" className="toggle-label block overflow-hidden h-5 rounded-full bg-slate-300 cursor-pointer"></label>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
