import React, { useMemo, useRef } from "react";
import { FileText, Paperclip, Send, Trash2 } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Button } from "../components/ui/Button";
import { sanitizeMailHtml } from "../lib/html/sanitizeMailHtml";
import { cn } from "../lib/utils/cn";

const MAX_ATTACHMENT_SIZE_BYTES = 5 * 1024 * 1024;
const MAX_TOTAL_ATTACHMENT_BYTES = 25 * 1024 * 1024;

function formatAttachmentSize(bytes) {
  const size = Number(bytes || 0);
  if (size >= 1024 * 1024) {
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }
  if (size >= 1024) {
    return `${Math.ceil(size / 1024)} KB`;
  }
  return `${size} B`;
}

function readFileAsAttachment(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const dataUrl = String(reader.result || "");
      const [, contentBase64 = ""] = dataUrl.split(",");
      resolve({
        id: `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        name: file.name || "未命名附件",
        type: file.name?.split(".").pop()?.toLowerCase() || "file",
        contentType: file.type || "application/octet-stream",
        sizeBytes: file.size,
        sizeLabel: formatAttachmentSize(file.size),
        contentEncoding: "base64",
        contentBase64,
      });
    };
    reader.onerror = () => reject(new Error("附件读取失败"));
    reader.readAsDataURL(file);
  });
}

export function ComposeView() {
  const { compose, templates, actions } = usePostOffice();
  const attachmentInputRef = useRef(null);
  const activeTemplate = useMemo(
    () => templates.find((item) => item.role === compose.inviteRole) || templates[0],
    [compose.inviteRole, templates],
  );
  const attachmentTotalBytes = useMemo(
    () => (compose.attachments || []).reduce((total, item) => total + Number(item?.sizeBytes || 0), 0),
    [compose.attachments],
  );
  const hasDraftContent = useMemo(() => (
    Boolean(compose.recipients || compose.subject || compose.body || compose.attachments?.length)
  ), [compose.attachments, compose.body, compose.recipients, compose.subject]);

  async function handleAttachmentChange(event) {
    const files = Array.from(event.target.files || []);
    event.target.value = "";
    const availableBytes = Math.max(0, MAX_TOTAL_ATTACHMENT_BYTES - attachmentTotalBytes);
    const acceptedFiles = files.filter((file) => file.size <= MAX_ATTACHMENT_SIZE_BYTES);
    const attachments = [];
    let nextTotal = 0;
    for (const file of acceptedFiles) {
      if (nextTotal + file.size > availableBytes) {
        break;
      }
      nextTotal += file.size;
      attachments.push(await readFileAsAttachment(file));
    }
    if (typeof actions.uploadComposeAttachments === "function") {
      await actions.uploadComposeAttachments(attachments);
    } else {
      actions.addComposeAttachments(attachments);
    }
  }

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
            通知模板
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
                <div className="flex items-center gap-3 border-b border-slate-100 pb-4 mb-4 text-slate-500">
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-md border border-slate-200 bg-white px-3 py-1.5 text-sm font-medium text-slate-600 hover:bg-slate-50 hover:text-slate-900"
                    onClick={() => attachmentInputRef.current?.click()}
                    title="添加附件"
                  >
                    <Paperclip size={16} />
                    添加附件
                  </button>
                  <input
                    ref={attachmentInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={handleAttachmentChange}
                  />
                </div>
                {compose.attachments?.length ? (
                  <div className="mb-4 flex flex-wrap gap-2">
                    {compose.attachments.map((attachment) => (
                      <div key={attachment.id} className="flex max-w-[260px] items-center gap-2 rounded-md border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">
                        <FileText size={15} className="shrink-0 text-slate-400" />
                        <span className="truncate">{attachment.name}</span>
                        <span className="shrink-0 text-xs text-slate-400">{attachment.sizeLabel}</span>
                        <button
                          type="button"
                          className="shrink-0 text-slate-400 hover:text-red-500"
                          onClick={() => actions.removeComposeAttachment(attachment.id)}
                          title="移除附件"
                        >
                          x
                        </button>
                      </div>
                    ))}
                  </div>
                ) : null}
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
                  <h3 className="text-sm font-medium text-slate-900 mb-3">模板类型</h3>
                  <select
                    className="w-full border-slate-300 rounded-md text-sm focus:ring-[#009BF5] focus:border-[#009BF5]"
                    value={compose.inviteRole}
                    onChange={(event) => actions.updateComposeField("inviteRole", event.target.value)}
                  >
                    <option value="account">账号开通通知</option>
                    <option value="collaboration">协作沟通邀请</option>
                    <option value="notice">客服通知模板</option>
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
                    <Send size={16} /> 发送通知邮件
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
              <Button
                type="button"
                variant="ghost"
                className="text-slate-400 hover:text-red-600"
                onClick={actions.discardCompose}
                disabled={!hasDraftContent}
                title="清空当前编辑"
              >
                <Trash2 size={18} />
              </Button>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
