import React, { useEffect, useMemo, useState } from "react";
import { Archive, FileText, Mail, MoreVertical, Paperclip, Reply, Forward, Search, Star, Trash2 } from "lucide-react";
import { usePostOffice } from "../../state/PostOfficeContext";
import { Button } from "../ui/Button";
import { Badge } from "../ui/Badge";
import { sanitizeMailHtml } from "../../lib/html/sanitizeMailHtml";
import { cn } from "../../lib/utils/cn";

const filters = [
  { id: "all", label: "全部邮件" },
  { id: "unread", label: "未读" },
  { id: "important", label: "重要" },
  { id: "attachment", label: "附件邮件" },
];

export function MailWorkspaceView({ folderId }) {
  const { mailbox, selectedMail, profile, actions } = usePostOffice();
  const [searchTerm, setSearchTerm] = useState("");
  const [filter, setFilter] = useState("all");

  useEffect(() => {
    setSearchTerm("");
    setFilter("all");
  }, [folderId]);

  useEffect(() => {
    if (
      mailbox.activeFolder === folderId &&
      mailbox.query.search === searchTerm &&
      mailbox.query.filter === filter &&
      mailbox.items.length > 0
    ) {
      return undefined;
    }

    const timer = window.setTimeout(() => {
      actions.loadFolder(folderId, { search: searchTerm, filter });
    }, 120);

    return () => window.clearTimeout(timer);
  }, [folderId, searchTerm, filter]);

  const recipientLine = useMemo(() => {
    if (!selectedMail) {
      return "";
    }
    if (selectedMail.isOutgoing) {
      return `发至 ${selectedMail.recipients.join(", ")}`;
    }
    return `发至 我 <${profile?.email || "myname@yuexiang.com"}>`;
  }, [selectedMail, profile?.email]);

  return (
    <div className="flex h-full bg-white">
      <div className="w-1/3 min-w-[320px] max-w-[400px] border-r border-slate-200 flex flex-col bg-slate-50">
        <div className="p-4 border-b border-slate-200 bg-white">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400 h-4 w-4" />
            <input
              type="text"
              value={searchTerm}
              onChange={(event) => setSearchTerm(event.target.value)}
              placeholder="搜索邮件、发件人..."
              className="w-full pl-9 pr-3 py-2 bg-slate-100 border-transparent rounded-md text-sm focus:border-[#009BF5] focus:bg-white focus:ring-0 transition-colors"
            />
          </div>
          <div className="flex gap-2 mt-4 overflow-x-auto pb-1 hide-scrollbar">
            {filters.map((item) => (
              <button
                key={item.id}
                onClick={() => setFilter(item.id)}
                className={cn(
                  "whitespace-nowrap px-3 py-1 rounded-full text-xs font-medium transition-colors",
                  filter === item.id ? "bg-[#009BF5] text-white" : "bg-slate-100 text-slate-600 hover:bg-slate-200",
                )}
              >
                {item.label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          <div className="px-4 py-2 text-xs font-medium text-slate-500">今天</div>
          {mailbox.items.map((mail) => (
            <div
              key={mail.id}
              onClick={() => actions.selectMail(mail.id)}
              className={cn(
                "px-4 py-3 border-b border-slate-100 cursor-pointer transition-colors relative group",
                selectedMail?.id === mail.id ? "bg-[#E5F5FF]" : "hover:bg-slate-50 bg-white",
              )}
            >
              {mail.isUnread ? <div className="absolute left-0 top-0 bottom-0 w-1 bg-[#009BF5]"></div> : null}
              <div className="flex justify-between items-start mb-1">
                <div className="flex items-center gap-2">
                  <span className={cn("font-medium text-sm", mail.isUnread ? "text-slate-900" : "text-slate-700")}>
                    {mail.sender}
                  </span>
                  {mail.hasAttachment ? <Paperclip size={12} className="text-slate-400" /> : null}
                </div>
                <span className="text-xs text-slate-400">{mail.time}</span>
              </div>
              <div className={cn("text-sm mb-1 truncate", mail.isUnread ? "font-medium text-slate-800" : "text-slate-600")}>
                {mail.subject}
              </div>
              <div className="text-xs text-slate-500 line-clamp-2 leading-relaxed">{mail.snippet}</div>
            </div>
          ))}

          {!mailbox.isLoading && mailbox.items.length === 0 ? (
            <div className="px-6 py-16 text-center text-slate-400 text-sm">当前条件下没有匹配邮件</div>
          ) : null}
        </div>
      </div>

      <div className="flex-1 flex flex-col min-w-0 bg-white">
        {selectedMail ? (
          <>
            <div className="h-14 border-b border-slate-200 flex items-center justify-between px-6 bg-white shrink-0">
              <div className="flex items-center gap-2">
                <Button variant="ghost" className="!p-2" onClick={() => actions.prepareReply(selectedMail, "reply")}>
                  <Reply size={18} />
                </Button>
                <Button variant="ghost" className="!p-2" onClick={() => actions.prepareReply(selectedMail, "forward")}>
                  <Forward size={18} />
                </Button>
                <div className="w-px h-4 bg-slate-200 mx-2"></div>
                <Button variant="ghost" className="!p-2" onClick={() => actions.moveMessage(selectedMail.id, "archive")}>
                  <Archive size={18} />
                </Button>
                <Button
                  variant="ghost"
                  className="!p-2 text-slate-400 hover:text-red-600 hover:bg-red-50"
                  onClick={() => actions.moveMessage(selectedMail.id, "trash")}
                >
                  <Trash2 size={18} />
                </Button>
              </div>
              <div className="flex items-center gap-2 text-slate-400">
                <Button variant="ghost" className="!p-2" onClick={() => actions.toggleStar(selectedMail.id)}>
                  <Star size={18} className={selectedMail.isStarred ? "text-[#009BF5] fill-current" : ""} />
                </Button>
                <Button variant="ghost" className="!p-2">
                  <MoreVertical size={18} />
                </Button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto">
              <div className="max-w-4xl mx-auto p-8">
                <h1 className="text-2xl font-bold text-slate-900 mb-6">{selectedMail.subject}</h1>

                <div className="flex items-start justify-between mb-8 pb-6 border-b border-slate-100 gap-6">
                  <div className="flex items-center gap-4 min-w-0">
                    <div className="w-12 h-12 rounded-full bg-slate-200 flex items-center justify-center text-slate-600 font-bold text-lg shrink-0">
                      {selectedMail.avatar}
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="font-medium text-slate-900">{selectedMail.sender}</span>
                        <span className="text-sm text-slate-500 break-all">&lt;{selectedMail.senderEmail}&gt;</span>
                        <Badge color={selectedMail.role === "平台官方" ? "orange" : "slate"}>{selectedMail.role}</Badge>
                        {selectedMail.tags.map((tag) => (
                          <Badge key={tag} color="slate">{tag}</Badge>
                        ))}
                      </div>
                      <div className="text-sm text-slate-500 mt-1">{recipientLine}</div>
                    </div>
                  </div>
                  <div className="text-sm text-slate-500 whitespace-nowrap">{selectedMail.dateTimeLabel}</div>
                </div>

                <div className="mail-html max-w-none text-slate-800" dangerouslySetInnerHTML={{ __html: sanitizeMailHtml(selectedMail.content) }} />

                {selectedMail.attachments?.length ? (
                  <div className="mt-12 pt-6 border-t border-slate-100">
                    <h4 className="text-sm font-medium text-slate-700 mb-4 flex items-center gap-2">
                      <Paperclip size={16} />
                      {selectedMail.attachments.length} 个附件
                    </h4>
                    <div className="flex gap-4 flex-wrap">
                      {selectedMail.attachments.map((attachment) => (
                        <div key={attachment.id} className="border border-slate-200 rounded-lg p-3 w-64 flex items-center gap-3 hover:bg-slate-50 cursor-pointer transition-colors">
                          <div className="w-10 h-10 bg-red-100 text-red-600 rounded flex items-center justify-center">
                            <FileText size={20} />
                          </div>
                          <div className="flex-1 min-w-0">
                            <div className="text-sm font-medium text-slate-800 truncate">{attachment.name}</div>
                            <div className="text-xs text-slate-500">{attachment.sizeLabel}</div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ) : null}
              </div>
            </div>
          </>
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center text-slate-400 bg-slate-50/50">
            <Mail size={48} className="mb-4 opacity-20" strokeWidth={1} />
            <p className="text-lg">选择一封邮件进行阅读</p>
          </div>
        )}
      </div>
    </div>
  );
}
