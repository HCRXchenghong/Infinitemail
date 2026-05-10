import React, { useEffect, useState } from "react";
import { AlertCircle, Mail } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Button } from "../components/ui/Button";

export function LoginView() {
  const { authFlow, actions } = usePostOffice();
  const [step, setStep] = useState(1);
  const [emailPrefix, setEmailPrefix] = useState("");

  useEffect(() => {
    if (authFlow.hasAuthenticatedSession && authFlow.requiresActivation) {
      setStep(2);
      return;
    }
    setStep(1);
  }, [authFlow.hasAuthenticatedSession, authFlow.requiresActivation]);

  async function handleLogin() {
    const result = await actions.beginUnifiedLogin();
    if (result?.isAuthenticated && result?.requiresActivation) {
      setStep(2);
    }
  }

  async function handleActivate() {
    await actions.activateMailbox(emailPrefix);
  }

  return (
    <div className="min-h-screen bg-white flex flex-col justify-center py-12 sm:px-6 lg:px-8">
      <div className="sm:mx-auto sm:w-full sm:max-w-md">
        <div className="flex justify-center text-[#009BF5]">
          <Mail size={48} strokeWidth={1.5} />
        </div>
        <h2 className="mt-6 text-center text-3xl font-extrabold text-slate-900">悦享邮局</h2>
        <p className="mt-2 text-center text-sm text-slate-600">专业、安全、互通的生态邮箱系统</p>
      </div>

      <div className="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
        {step === 1 ? (
          <div className="space-y-6 px-4 sm:px-0 mt-4">
            <Button
              className="w-full h-11 text-base bg-[#009BF5] hover:bg-[#008AE6] focus:ring-[#009BF5] shadow-sm"
              onClick={handleLogin}
              disabled={authFlow.isLoading}
            >
              使用 悦享e食 账号登录
            </Button>
            <div className="relative pt-4">
              <div className="absolute inset-0 flex items-center pt-4"><div className="w-full border-t border-slate-200" /></div>
              <div className="relative flex justify-center text-sm pt-4">
                <span className="px-2 bg-white text-slate-500">一个生态，一个账号</span>
              </div>
            </div>
            {authFlow.errorMessage ? (
              <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
                {authFlow.errorMessage}
              </div>
            ) : null}
          </div>
        ) : (
          <div className="bg-white py-8 px-4 shadow sm:rounded-lg sm:px-10 border border-slate-100">
            <div className="space-y-6">
              <div>
                <h3 className="text-lg font-medium text-slate-900">开通您的专属邮箱</h3>
                <p className="text-sm text-slate-500 mt-1">检测到您是首次使用，请设置您的邮箱地址。</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700">邮箱地址</label>
                <div className="mt-1 flex rounded-md shadow-sm">
                  <span className="inline-flex items-center px-3 rounded-l-md border border-r-0 border-slate-300 bg-slate-50 text-slate-500 sm:text-sm">
                    {authFlow.rolePrefix}
                  </span>
                  <input
                    type="text"
                    value={emailPrefix}
                    onChange={(event) => setEmailPrefix(event.target.value)}
                    className="flex-1 min-w-0 block w-full px-3 py-2 border border-slate-300 focus:ring-[#009BF5] focus:border-[#009BF5] sm:text-sm z-10"
                    placeholder="输入姓名拼音"
                  />
                  <span className="inline-flex items-center px-3 rounded-r-md border border-l-0 border-slate-300 bg-slate-50 text-slate-500 sm:text-sm">
                    @yuexiang.com
                  </span>
                </div>
                <p className="mt-2 text-xs text-slate-500 flex items-start gap-1">
                  <AlertCircle size={14} className="mt-0.5 flex-shrink-0" />
                  提示：开通后不可修改。当您的悦享e食主账号注销时，此邮箱将自动回收并清空数据。
                </p>
              </div>
              {authFlow.errorMessage ? (
                <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
                  {authFlow.errorMessage}
                </div>
              ) : null}
              <Button className="w-full" onClick={handleActivate} disabled={!emailPrefix || authFlow.isLoading}>
                确认开通并进入邮箱
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
