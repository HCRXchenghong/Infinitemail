import React from "react";
import { FileText, Inbox, LogOut, Mail, Plus, Send, Settings, Star, Trash2, Users } from "lucide-react";
import { Button } from "../ui/Button";
import { cn } from "../../lib/utils/cn";

function NavItem({ id, icon: Icon, label, badge, currentView, onNavigate }) {
  const isActive = currentView === id;

  return (
    <button
      onClick={() => onNavigate(id)}
      className={cn(
        "w-full flex items-center justify-between px-3 py-2 rounded-md text-sm font-medium transition-colors",
        isActive ? "bg-[#E5F5FF] text-[#009BF5]" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900",
      )}
    >
      <div className="flex items-center gap-3">
        <Icon size={18} className={isActive ? "text-[#009BF5]" : "text-slate-400"} />
        {label}
      </div>
      {badge ? (
        <span className={cn("px-2 py-0.5 rounded-full text-xs", isActive ? "bg-[#009BF5] text-white" : "bg-slate-100 text-slate-500")}>
          {badge}
        </span>
      ) : null}
    </button>
  );
}

export function AppSidebar({ currentView, folderCounts, profile, onNavigate, onLogout }) {
  return (
    <div className="w-64 bg-white border-r border-slate-200 flex flex-col z-10 shrink-0">
      <div className="h-14 flex items-center px-6 border-b border-slate-200">
        <div className="flex items-center gap-2 text-[#009BF5] font-bold text-lg tracking-wide">
          <Mail size={22} strokeWidth={2.5} />
          <span className="text-slate-800">悦享邮局</span>
        </div>
      </div>

      <div className="p-4">
        <Button className="w-full shadow-sm" onClick={() => onNavigate("compose")}>
          <Plus size={18} /> 写邮件
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto px-3 py-2 space-y-1">
        <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2 px-3 mt-2">邮件</div>
        <NavItem id="inbox" icon={Inbox} label="收件箱" badge={folderCounts.inbox ? String(folderCounts.inbox) : null} currentView={currentView} onNavigate={onNavigate} />
        <NavItem id="starred" icon={Star} label="星标邮件" currentView={currentView} onNavigate={onNavigate} />
        <NavItem id="sent" icon={Send} label="已发送" currentView={currentView} onNavigate={onNavigate} />
        <NavItem id="drafts" icon={FileText} label="草稿箱" currentView={currentView} onNavigate={onNavigate} />
        <NavItem id="trash" icon={Trash2} label="垃圾箱" currentView={currentView} onNavigate={onNavigate} />

        <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2 px-3 mt-6">联系人</div>
        <NavItem id="contacts" icon={Users} label="通讯录" currentView={currentView} onNavigate={onNavigate} />

        <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2 px-3 mt-6">系统</div>
        <NavItem id="settings" icon={Settings} label="设置" currentView={currentView} onNavigate={onNavigate} />
      </div>

      <div className="p-4 border-t border-slate-200">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#009BF5] text-white flex items-center justify-center font-bold text-sm">
            {profile?.avatarInitial || "M"}
          </div>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium text-slate-900 truncate">{profile?.displayName || "MyName"}</div>
            <div className="text-xs text-slate-500 truncate">{profile?.email || "myname@yuexiang..."}</div>
          </div>
          <button className="text-slate-400 hover:text-slate-600" onClick={onLogout}>
            <LogOut size={16} />
          </button>
        </div>
      </div>
    </div>
  );
}
