import React, { useEffect, useState } from "react";
import { ArrowUpRight, FileText, Mail, Search, Users } from "lucide-react";
import { usePostOffice } from "../state/PostOfficeContext";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { cn } from "../lib/utils/cn";

const folderLabels = {
  inbox: "收件箱",
  sent: "已发送",
  drafts: "草稿箱",
  starred: "星标邮件",
  trash: "垃圾箱",
};

function getThreadDirectionLabel(item) {
  if (item?.folderId === "drafts") {
    return "草稿";
  }
  return item?.isOutgoing ? "我发出的" : "对方向我发来";
}

export function ContactsView() {
  const { contacts, selectedContact, contactThread, actions } = usePostOffice();
  const [searchTerm, setSearchTerm] = useState("");

  useEffect(() => {
    const timer = window.setTimeout(() => {
      actions.loadContacts(searchTerm);
    }, 120);
    return () => window.clearTimeout(timer);
  }, [searchTerm]);

  useEffect(() => {
    if (selectedContact?.id) {
      actions.loadContactThread(selectedContact);
    }
  }, [selectedContact?.id, selectedContact?.email]);

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
              placeholder="搜索联系人、角色、组织..."
              className="w-full pl-9 pr-3 py-2 bg-slate-100 border-transparent rounded-md text-sm focus:border-[#009BF5] focus:bg-white focus:ring-0 transition-colors"
            />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          <div className="px-4 py-2 text-xs font-medium text-slate-500">生态联系人</div>
          {contacts.items.map((contact) => (
            <div
              key={contact.id}
              onClick={() => actions.selectContact(contact.id)}
              className={cn(
                "px-4 py-3 border-b border-slate-100 cursor-pointer transition-colors bg-white",
                selectedContact?.id === contact.id ? "bg-[#E5F5FF]" : "hover:bg-slate-50",
              )}
            >
              <div className="flex items-start gap-3">
                <div className="w-10 h-10 rounded-full bg-slate-200 flex items-center justify-center text-slate-600 font-semibold">
                  {contact.avatar}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm text-slate-900">{contact.name}</span>
                    <Badge color={contact.role === "平台官方" ? "orange" : "slate"}>{contact.role}</Badge>
                  </div>
                  <div className="text-sm text-slate-600 truncate mt-1">{contact.email}</div>
                  <div className="text-xs text-slate-500 mt-1">{contact.organization}</div>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="flex-1 flex flex-col min-w-0 bg-white">
        {selectedContact ? (
          <div className="flex-1 overflow-y-auto">
            <div className="max-w-4xl mx-auto p-8">
              <div className="border border-slate-200 rounded-xl p-8">
                <div className="flex items-start justify-between gap-6">
                  <div className="flex items-center gap-4">
                    <div className="w-16 h-16 rounded-full bg-[#E5F5FF] text-[#009BF5] flex items-center justify-center font-bold text-2xl">
                      {selectedContact.avatar}
                    </div>
                    <div>
                      <div className="flex items-center gap-2 flex-wrap">
                        <h2 className="text-2xl font-bold text-slate-900">{selectedContact.name}</h2>
                        <Badge color={selectedContact.role === "平台官方" ? "orange" : "slate"}>{selectedContact.role}</Badge>
                      </div>
                      <p className="text-slate-500 mt-2">{selectedContact.organization}</p>
                      <p className="text-sm text-slate-500 mt-1">{selectedContact.email}</p>
                    </div>
                  </div>
                  <Button onClick={() => actions.openComposeForContact(selectedContact)}>
                    <Mail size={16} /> 发邮件
                  </Button>
                </div>

                <div className="grid grid-cols-2 gap-6 mt-8">
                  <div className="rounded-lg border border-slate-200 p-5 bg-slate-50/60">
                    <div className="text-sm font-medium text-slate-900">最近联系</div>
                    <div className="text-sm text-slate-500 mt-2">{selectedContact.lastContactedAt}</div>
                    <div className="text-xs text-slate-400 mt-2">累计 {selectedContact.stats?.totalMessages || 0} 条往来记录</div>
                  </div>
                  <div className="rounded-lg border border-slate-200 p-5 bg-slate-50/60">
                    <div className="text-sm font-medium text-slate-900">协作关系</div>
                    <div className="text-sm text-slate-500 mt-2">{selectedContact.note}</div>
                    <div className="text-xs text-slate-400 mt-2">邀请沉淀 {selectedContact.stats?.inviteCount || 0} 次</div>
                  </div>
                </div>

                {selectedContact.tags?.length ? (
                  <div className="mt-6">
                    <div className="text-sm font-medium text-slate-900 mb-3">联系标签</div>
                    <div className="flex flex-wrap gap-2">
                      {selectedContact.tags.map((tag) => (
                        <Badge key={tag} color={tag.includes("邀请") || tag.includes("已邀请") ? "blue" : "slate"}>
                          {tag}
                        </Badge>
                      ))}
                    </div>
                  </div>
                ) : null}

                <div className="mt-8 pt-6 border-t border-slate-100">
                  <div className="text-sm font-medium text-slate-900 mb-3">快速操作</div>
                  <div className="flex gap-3">
                    <Button variant="secondary" onClick={() => actions.openComposeForContact(selectedContact)}>发送邮件</Button>
                    <Button variant="ghost" onClick={() => actions.openContactHistory(selectedContact)}>查看往来邮件</Button>
                  </div>
                  <div className="text-xs text-slate-400 mt-3">联系人会基于跨角色往来邮件与生态会话自动聚合。</div>
                </div>

                <div className="mt-8 pt-6 border-t border-slate-100">
                  <div className="flex items-center justify-between gap-4 mb-4">
                    <div>
                      <div className="text-sm font-medium text-slate-900">往来记录</div>
                      <div className="text-xs text-slate-500 mt-1">按最近互动倒序展示，可直接跳转到对应邮件。</div>
                    </div>
                    <div className="text-sm text-slate-500 whitespace-nowrap">{contactThread.total} 条记录</div>
                  </div>

                  {contactThread.isLoading ? (
                    <div className="rounded-lg border border-slate-200 bg-slate-50/60 px-4 py-10 text-center text-sm text-slate-400">
                      正在加载往来记录...
                    </div>
                  ) : contactThread.items.length ? (
                    <div className="space-y-3">
                      {contactThread.items.map((item) => (
                        <div key={item.id} className="rounded-lg border border-slate-200 bg-slate-50/60 p-4">
                          <div className="flex items-start justify-between gap-4">
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-2 flex-wrap">
                                <Badge color={item.isOutgoing ? "blue" : "green"}>{getThreadDirectionLabel(item)}</Badge>
                                <Badge color="slate">{folderLabels[item.folderId] || item.folderId}</Badge>
                                {(item.tags || []).slice(0, 3).map((tag) => (
                                  <Badge key={`${item.id}-${tag}`} color="slate">{tag}</Badge>
                                ))}
                              </div>
                              <div className="text-sm font-medium text-slate-900 mt-3">{item.subject}</div>
                              <div className="text-xs text-slate-500 mt-1">
                                {item.isOutgoing ? `发送至 ${item.recipients?.join(", ") || selectedContact.email}` : `来自 ${item.senderEmail || selectedContact.email}`}
                              </div>
                              <div className="text-sm text-slate-600 mt-2 leading-relaxed line-clamp-2">{item.snippet}</div>
                            </div>
                            <div className="shrink-0 text-right">
                              <div className="text-xs text-slate-500">{item.dateTimeLabel}</div>
                              <Button
                                variant="ghost"
                                className="mt-2 !px-2"
                                onClick={() => actions.openContactThreadMessage(item)}
                              >
                                <ArrowUpRight size={14} /> 查看
                              </Button>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="rounded-lg border border-slate-200 bg-slate-50/60 px-4 py-10 text-center text-sm text-slate-400">
                      暂无与该联系人的往来记录
                    </div>
                  )}

                  {contactThread.hasMore ? (
                    <div className="mt-4 flex justify-center">
                      <Button variant="secondary" onClick={actions.loadMoreContactThread}>
                        <FileText size={16} /> 加载更多记录
                      </Button>
                    </div>
                  ) : null}
                </div>
              </div>
            </div>
          </div>
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center text-slate-400 bg-slate-50/50">
            <Users size={48} className="mb-4 opacity-20" strokeWidth={1} />
            <p className="text-lg">选择联系人查看详情</p>
          </div>
        )}
      </div>
    </div>
  );
}
