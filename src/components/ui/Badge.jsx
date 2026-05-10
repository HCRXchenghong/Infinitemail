import React from "react";
import { cn } from "../../lib/utils/cn";

export function Badge({ children, color = "blue", className = "" }) {
  const colors = {
    blue: "bg-[#E5F5FF] text-[#009BF5] border-[#B3E0FF]",
    orange: "bg-[#E5F5FF] text-[#009BF5] border-[#B3E0FF]",
    slate: "bg-slate-100 text-slate-700 border-slate-200",
    green: "bg-green-50 text-green-700 border-green-200",
  };

  return (
    <span className={cn("inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border", colors[color], className)}>
      {children}
    </span>
  );
}
