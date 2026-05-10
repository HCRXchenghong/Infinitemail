import { runtimeConfig } from "../lib/config/runtime";

const DEV_SSO_ENABLED_STORAGE_KEY = "yuexiang-post-office.dev-sso.enabled";
const DEV_SSO_ROLE_STORAGE_KEY = "yuexiang-post-office.dev-sso.role";
const UNIFIED_SESSION_STORAGE_KEYS = [
  "token",
  "access_token",
  "refreshToken",
  "tokenExpiresAt",
  "authMode",
  "userProfile",
];

function normalizeString(value) {
  return String(value == null ? "" : value).trim();
}

function normalizeRole(value, fallback = "user") {
  const normalized = normalizeString(value).toLowerCase();
  if (normalized === "merchant" || normalized === "rider" || normalized === "user") {
    return normalized;
  }
  return fallback;
}

function readStorage() {
  if (typeof window === "undefined") {
    return null;
  }
  return window.localStorage || null;
}

function readStorageString(key) {
  const storage = readStorage();
  if (!storage) {
    return "";
  }

  try {
    return normalizeString(storage.getItem(key));
  } catch (_error) {
    return "";
  }
}

function writeStorageString(key, value) {
  const storage = readStorage();
  if (!storage) {
    return;
  }

  try {
    const normalized = normalizeString(value);
    if (!normalized) {
      storage.removeItem(key);
      return;
    }
    storage.setItem(key, normalized);
  } catch (_error) {
    // ignore local storage failures in local dev shell
  }
}

function readBooleanFlag(key) {
  return readStorageString(key) === "1";
}

function writeBooleanFlag(key, value) {
  writeStorageString(key, value ? "1" : "");
}

function clearStoredUnifiedSession() {
  const storage = readStorage();
  if (!storage) {
    return;
  }

  try {
    for (const key of UNIFIED_SESSION_STORAGE_KEYS) {
      storage.removeItem(key);
    }
  } catch (_error) {
    // ignore local storage failures in local dev shell
  }
}

function resolveApiUrl(pathname) {
  const normalizedBase = normalizeString(runtimeConfig.platformApiBaseUrl || "/api").replace(/\/+$/, "");
  const normalizedPath = normalizeString(pathname).startsWith("/")
    ? normalizeString(pathname)
    : `/${normalizeString(pathname)}`;
  return `${normalizedBase}${normalizedPath}`;
}

function readQueryRole() {
  if (typeof window === "undefined") {
    return "";
  }

  try {
    const params = new URLSearchParams(window.location.search);
    return normalizeString(params.get("devRole"));
  } catch (_error) {
    return "";
  }
}

function readQueryAutoLogin() {
  if (typeof window === "undefined") {
    return false;
  }

  try {
    const params = new URLSearchParams(window.location.search);
    const value = normalizeString(params.get("devAutoLogin")).toLowerCase();
    return value === "1" || value === "true" || value === "yes";
  } catch (_error) {
    return false;
  }
}

function resolveSelectedRole() {
  const queryRole = normalizeRole(readQueryRole(), "");
  if (queryRole) {
    writeStorageString(DEV_SSO_ROLE_STORAGE_KEY, queryRole);
    return queryRole;
  }

  const storedRole = normalizeRole(
    readStorageString(DEV_SSO_ROLE_STORAGE_KEY),
    "",
  );
  if (storedRole) {
    return storedRole;
  }

  return normalizeRole(runtimeConfig.devSsoAuthMode, "user");
}

function setSelectedRole(role) {
  const nextRole = normalizeRole(role, "user");
  writeStorageString(DEV_SSO_ROLE_STORAGE_KEY, nextRole);
  clearStoredUnifiedSession();
  return nextRole;
}

function isEnabled() {
  return readBooleanFlag(DEV_SSO_ENABLED_STORAGE_KEY);
}

function setEnabled(nextValue) {
  writeBooleanFlag(DEV_SSO_ENABLED_STORAGE_KEY, nextValue);
}

function buildFallbackSessionPayload(authMode) {
  const user = {
    id: runtimeConfig.devSsoUserId,
    uid: runtimeConfig.devSsoUserId,
    phone: runtimeConfig.devSsoPhone,
    nickname: runtimeConfig.devSsoName,
    name: runtimeConfig.devSsoName,
    avatarUrl: "",
    principalType: authMode,
    role: authMode,
    authMode,
  };

  return {
    authenticated: true,
    authMode,
    session: {
      token: runtimeConfig.devSsoAccessToken,
      refreshToken: runtimeConfig.devSsoRefreshToken,
      expiresIn: runtimeConfig.devSsoExpiresIn,
    },
    user,
    userProfile: user,
  };
}

async function requestDevSession(role) {
  const response = await fetch(resolveApiUrl(runtimeConfig.devSsoEndpointPath), {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      authMode: role,
    }),
  });

  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(payload?.message || "开发联调会话创建失败");
  }

  return payload?.data || payload || null;
}

async function buildSessionPayload(role) {
  const normalizedRole = normalizeRole(role, "user");

  try {
    return await requestDevSession(normalizedRole);
  } catch (error) {
    if (normalizeString(runtimeConfig.devSsoAccessToken)) {
      return buildFallbackSessionPayload(normalizedRole);
    }
    throw error;
  }
}

export function installDevSsoBridge() {
  if (typeof window === "undefined" || !runtimeConfig.devSsoEnabled) {
    return;
  }

  const bridgeName =
    normalizeString(runtimeConfig.ssoBridgeName) || "__YUEXIANG_POST_OFFICE_SSO__";

  const queryRole = normalizeRole(readQueryRole(), "");
  if (queryRole) {
    setSelectedRole(queryRole);
  }

  const selectedRole = resolveSelectedRole();
  const currentSessionRole = normalizeRole(readStorageString("authMode"), "");
  if (currentSessionRole && currentSessionRole !== selectedRole) {
    clearStoredUnifiedSession();
  }

  if (runtimeConfig.devSsoAutoLogin || readQueryAutoLogin()) {
    setEnabled(true);
  }

  const readSession = async () => {
    if (!isEnabled()) {
      return null;
    }
    return buildSessionPayload(resolveSelectedRole());
  };

  const beginLogin = async (options = {}) => {
    const nextRole = setSelectedRole(options?.authMode || options?.role || resolveSelectedRole());
    setEnabled(true);
    return buildSessionPayload(nextRole);
  };

  const logout = async () => {
    setEnabled(false);
    clearStoredUnifiedSession();
    return { success: true };
  };

  const switchRole = async (role) => {
    const nextRole = setSelectedRole(role);
    setEnabled(true);
    return buildSessionPayload(nextRole);
  };

  window[bridgeName] = {
    getSession: readSession,
    readSession,
    beginLogin,
    login: beginLogin,
    startLogin: beginLogin,
    logout,
  };

  window.__YUEXIANG_POST_OFFICE_DEV_SSO__ = {
    enable() {
      setEnabled(true);
      return buildSessionPayload(resolveSelectedRole());
    },
    disable() {
      setEnabled(false);
      return { success: true };
    },
    getRole() {
      return resolveSelectedRole();
    },
    setRole(role) {
      return setSelectedRole(role);
    },
    switchRole,
    getSession: readSession,
    bridgeName,
  };
}
