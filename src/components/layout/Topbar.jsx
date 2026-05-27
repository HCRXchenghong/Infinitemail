import React from "react";
import { AlertCircle, Bell, CheckCircle2, Shield, X } from "lucide-react";
import { cn } from "../../lib/utils/cn";

const noticeStyleMap = {
  success: {
    container: "bg-[#E5F5FF] text-[#007ACC] border-[#B3E0FF]",
    icon: CheckCircle2,
  },
  warning: {
    container: "bg-amber-50 text-amber-700 border-amber-200",
    icon: AlertCircle,
  },
  info: {
    container: "bg-slate-100 text-slate-600 border-slate-200",
    icon: AlertCircle,
  },
};

export function Topbar({ health, notice, onDismissNotice }) {
  const noticeTone = noticeStyleMap[notice?.tone] ? notice.tone : "info";
  const NoticeIcon = noticeStyleMap[noticeTone].icon;

  return (
    <header className="h-14 bg-white border-b border-slate-200 flex items-center justify-between px-6 shrink-0">
      <div className="flex-1 max-w-2xl flex items-center gap-3 min-w-0">
        {notice ? (
          <div className={cn("min-w-0 flex-1 flex items-center gap-2 rounded-md border px-3 py-2 text-sm", noticeStyleMap[noticeTone].container)}>
            <NoticeIcon size={16} className="shrink-0" />
            <span className="truncate">{notice.message}</span>
            <button
              type="button"
              className="ml-auto shrink-0 opacity-70 hover:opacity-100 transition-opacity"
              onClick={onDismissNotice}
            >
              <X size={14} />
            </button>
          </div>
        ) : null}
      </div>
      <div className="flex items-center gap-4 text-slate-500">
        <button className="hover:text-slate-800 transition-colors relative">
          <Bell size={20} />
          <span className="absolute top-0 right-0 w-2 h-2 bg-red-500 rounded-full border-2 border-white"></span>
        </button>
        <div className="w-px h-5 bg-slate-200"></div>
        <div className="flex items-center gap-2 text-sm cursor-pointer hover:text-slate-800">
          <Shield size={16} className="text-green-500" />
          <span>{health?.label || "服务连接正常"}</span>
        </div>
      </div>
    </header>
  );
}
