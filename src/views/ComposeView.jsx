import React, { useMemo } from "react";
import { FileText, Paperclip, Send, Trash2 } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Button } from "../components/ui/Button";
import { sanitizeMailHtml } from "../lib/html/sanitizeMailHtml";
import { cn } from "../lib/utils/cn";

export function ComposeView() {
  const { compose, templates, actions } = usePostOffice();
  const activeTemplate = useMemo(
    () => templates.find((item) => item.role === compose.inviteRole) || templates[0],
    [compose.inviteRole, templates],
  );

  return (
    <div className="h-full bg-white flex flex-col">
      <div className="h-14 border-b border-slate-200 flex items-center px-6 justify-between shrink-0">
        <h2 className="text-lg font-medium text-slate-800">写邮件</h2>
        <div className="flex bg-slate-100 p-1 rounded-lg">
          <button
            className={cn(
              "px-4 py-1.5 text-sm rounded-md transition-colors",
              compose.mode === "normal" ? "bg-white shadow-sm text-slate-800" : "text-slate-600 hover:text-slate-800",
            )}
            onClick={() => actions.setComposeMode("normal")}
          >
            常规写信
          </button>
          <button
            className={cn(
              "px-4 py-1.5 text-sm rounded-md transition-colors",
              compose.mode === "invite" ? "bg-white shadow-sm text-slate-800" : "text-slate-600 hover:text-slate-800",
            )}
            onClick={() => actions.setComposeMode("invite")}
          >
            业务邀请函
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-8 bg-slate-50/30">
        <div className="max-w-4xl mx-auto bg-white border border-slate-200 rounded-xl shadow-sm overflow-hidden">
          {compose.mode === "normal" ? (
            <div className="flex flex-col">
              <div className="px-6 py-4 border-b border-slate-100 flex items-center">
                <span className="text-sm font-medium text-slate-500 w-16">收件人</span>
                <input
                  type="text"
                  value={compose.recipients}
                  onChange={(event) => actions.updateComposeField("recipients", event.target.value)}
                  className="flex-1 border-none focus:ring-0 text-sm p-0"
                  placeholder="多个地址请用逗号分隔"
                />
              </div>
              <div className="px-6 py-4 border-b border-slate-100 flex items-center">
                <span className="text-sm font-medium text-slate-500 w-16">主 题</span>
                <input
                  type="text"
                  value={compose.subject}
                  onChange={(event) => actions.updateComposeField("subject", event.target.value)}
                  className="flex-1 border-none focus:ring-0 text-sm p-0 font-medium"
                  placeholder="邮件主题"
                />
              </div>
              <div className="p-6 min-h-[400px]">
                <div className="flex items-center gap-4 border-b border-slate-100 pb-4 mb-4 text-slate-500">
                  <span className="font-serif font-bold cursor-pointer hover:text-slate-800">B</span>
                  <span className="font-serif italic cursor-pointer hover:text-slate-800">I</span>
                  <span className="font-serif underline cursor-pointer hover:text-slate-800">U</span>
                  <div className="w-px h-4 bg-slate-200"></div>
                  <FileText size={16} className="cursor-pointer hover:text-slate-800" />
                  <Paperclip size={16} className="cursor-pointer hover:text-slate-800" />
                </div>
                <textarea
                  value={compose.body}
                  onChange={(event) => actions.updateComposeField("body", event.target.value)}
                  className="w-full h-[300px] border-none focus:ring-0 resize-none text-slate-700 leading-relaxed"
                  placeholder="在此输入正文..."
                ></textarea>
              </div>
            </div>
          ) : (
            <div className="flex">
              <div className="w-80 border-r border-slate-100 p-6 bg-slate-50 flex flex-col gap-6">
                <div>
                  <h3 className="text-sm font-medium text-slate-900 mb-3">邀请对象类型</h3>
                  <select
                    className="w-full border-slate-300 rounded-md text-sm focus:ring-[#009BF5] focus:border-[#009BF5]"
                    value={compose.inviteRole}
                    onChange={(event) => actions.updateComposeField("inviteRole", event.target.value)}
                  >
                    <option value="merchant">新商户入驻</option>
                    <option value="rider">外卖骑手招募</option>
                    <option value="user">核心用户内测</option>
                  </select>
                </div>
                <div>
                  <h3 className="text-sm font-medium text-slate-900 mb-3">目标邮箱</h3>
                  <textarea
                    value={compose.inviteEmails}
                    onChange={(event) => actions.updateComposeField("inviteEmails", event.target.value)}
                    className="w-full border-slate-300 rounded-md text-sm focus:ring-[#009BF5] focus:border-[#009BF5] min-h-[100px]"
                    placeholder="输入或粘贴目标邮箱地址，每行一个"
                  ></textarea>
                </div>
                <div className="mt-auto">
                  <Button className="w-full" onClick={actions.sendCompose}>
                    <Send size={16} /> 一键发送邀请
                  </Button>
                </div>
              </div>
              <div className="flex-1 p-8 bg-slate-200/50 flex items-center justify-center">
                <div className="w-full max-w-lg bg-white shadow-lg rounded-xl overflow-hidden border border-slate-200">
                  <div dangerouslySetInnerHTML={{ __html: sanitizeMailHtml(activeTemplate?.html || "") }} />
                </div>
              </div>
            </div>
          )}

          {compose.mode === "normal" ? (
            <div className="px-6 py-4 bg-slate-50 border-t border-slate-100 flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Button onClick={actions.sendCompose}><Send size={16} /> 发送</Button>
                <Button variant="secondary" onClick={actions.saveDraft}>保存草稿</Button>
              </div>
              <Button variant="ghost" className="text-slate-400 hover:text-red-600"><Trash2 size={18} /></Button>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
