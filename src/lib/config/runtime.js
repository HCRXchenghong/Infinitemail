import { POST_OFFICE_API_BASE_PATH } from "@infinitech/contracts/post-office";

export const runtimeConfig = {
  apiMode: import.meta.env.VITE_MAIL_API_MODE || "mock",
  platformApiBaseUrl: import.meta.env.VITE_PLATFORM_API_BASE_URL || "/api",
  mailApiBaseUrl: import.meta.env.VITE_MAIL_API_BASE_URL || POST_OFFICE_API_BASE_PATH,
  apiBaseUrl: import.meta.env.VITE_MAIL_API_BASE_URL || POST_OFFICE_API_BASE_PATH,
  mailboxDomain: import.meta.env.VITE_MAILBOX_DOMAIN || "yuexiang.com",
  ssoEntryUrl: import.meta.env.VITE_PLATFORM_SSO_ENTRY_URL || "",
  ssoBridgeName: import.meta.env.VITE_PLATFORM_SSO_BRIDGE_NAME || "__YUEXIANG_POST_OFFICE_SSO__",
  requestTimeoutMs: Number(import.meta.env.VITE_MAIL_API_TIMEOUT || 10000),
  devSsoEnabled: String(import.meta.env.VITE_DEV_SSO_ENABLED || "").trim() === "true",
  devSsoAutoLogin: String(import.meta.env.VITE_DEV_SSO_AUTO_LOGIN || "").trim() === "true",
  devSsoEndpointPath: import.meta.env.VITE_DEV_SSO_ENDPOINT_PATH || "/auth/dev/post-office-session",
  devSsoAccessToken: import.meta.env.VITE_DEV_SSO_ACCESS_TOKEN || "",
  devSsoRefreshToken: import.meta.env.VITE_DEV_SSO_REFRESH_TOKEN || "dev-refresh-token",
  devSsoExpiresIn: Number(import.meta.env.VITE_DEV_SSO_EXPIRES_IN || 86400),
  devSsoAuthMode: import.meta.env.VITE_DEV_SSO_AUTH_MODE || "user",
  devSsoUserId: import.meta.env.VITE_DEV_SSO_USER_ID || "user-10001",
  devSsoPhone: import.meta.env.VITE_DEV_SSO_PHONE || "13800138000",
  devSsoName: import.meta.env.VITE_DEV_SSO_NAME || "MyName",
};

export const isMockMode = runtimeConfig.apiMode !== "live";
