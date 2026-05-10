import React, { useEffect, useState } from "react";
import { Info, Settings, Shield } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";

function formatUpdatedAt(value) {
  const date = new Date(value || "");
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  return `${date.getMonth() + 1}月${date.getDate()}日 ${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

export function SettingsView() {
  const { settings, profile, actions } = usePostOffice();
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
                    平台账号绑定 <Badge color="green">已绑定</Badge>
                  </div>
                  <div className="text-sm text-slate-500 mt-1 max-w-lg">
                    您的邮箱已与悦享e食主账号({profile?.unifiedAccountPhone || "138****8888"})深度绑定。支持一键 SSO 登录及生态消息互通。
                  </div>
                  <div className="mt-3 p-3 bg-[#E5F5FF] text-[#007ACC] text-xs rounded-md flex items-start gap-2 border border-[#B3E0FF]">
                    <Info size={14} className="mt-0.5 shrink-0" />
                    安全规则：当您注销悦享e食主账号时，为保障数据安全，此邮箱及所有数据将被自动回收并永久销毁。
                  </div>
                </div>
                <Button variant="secondary" disabled>管理授权</Button>
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-slate-900">登录设备管理</div>
                  <div className="text-sm text-slate-500 mt-1">查看最近登录的设备及 IP 异常。</div>
                </div>
                <Button variant="ghost">查看记录</Button>
              </div>
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
