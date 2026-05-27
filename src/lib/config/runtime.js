import { POST_OFFICE_API_BASE_PATH } from "@infinitech/contracts/post-office";

export const runtimeConfig = {
  appSurface: import.meta.env.VITE_APP_SURFACE || "user",
  userAppOrigin: import.meta.env.VITE_USER_APP_ORIGIN || "http://127.0.0.1:1788",
  adminAppOrigin: import.meta.env.VITE_ADMIN_APP_ORIGIN || "http://127.0.0.1:1888",
  apiProxyTarget: import.meta.env.VITE_API_PROXY_TARGET || "http://127.0.0.1:1666",
  mailApiBaseUrl: import.meta.env.VITE_MAIL_API_BASE_URL || POST_OFFICE_API_BASE_PATH,
  adminApiToken: import.meta.env.VITE_ADMIN_API_TOKEN || "",
  mailboxDomain: import.meta.env.VITE_MAILBOX_DOMAIN || "yuexiang.com",
  requestTimeoutMs: Number(import.meta.env.VITE_MAIL_API_TIMEOUT || 10000),
};
