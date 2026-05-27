import React from "react";
import { cn } from "../../lib/utils/cn";

export function Button({ children, variant = "primary", className = "", type = "button", ...props }) {
  const baseStyle =
    "inline-flex items-center justify-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed";

  const variants = {
    primary: "bg-[#009BF5] text-white hover:bg-[#008AE6] focus:ring-[#009BF5]",
    secondary: "bg-white text-slate-700 border border-slate-300 hover:bg-slate-50 focus:ring-[#009BF5]",
    ghost: "text-slate-600 hover:bg-slate-100 hover:text-slate-900 focus:ring-slate-500",
    danger: "bg-red-50 text-red-600 hover:bg-red-100 focus:ring-red-500",
  };

  return (
    <button type={type} className={cn(baseStyle, variants[variant], className)} {...props}>
      {children}
    </button>
  );
}
