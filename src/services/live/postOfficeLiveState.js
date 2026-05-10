const LIVE_STATE_STORAGE_KEY = "yuexiang-post-office.live.v1";

const defaultLiveState = Object.freeze({
  mailboxByAccountId: {},
  settingsByAccountId: {},
  localMessagesByAccountId: {},
  conversationMetaByAccountId: {},
});

function clonePlainObject(value, fallback = {}) {
  return value && typeof value === "object" && !Array.isArray(value) ? { ...value } : { ...fallback };
}

function readStorageArea() {
  if (typeof window === "undefined") {
    return null;
  }
  return window.localStorage || null;
}

function readString(key) {
  const storage = readStorageArea();
  if (!storage) {
    return "";
  }
  try {
    return String(storage.getItem(key) || "").trim();
  } catch (_error) {
    return "";
  }
}

function writeString(key, value) {
  const storage = readStorageArea();
  if (!storage) {
    return;
  }
  try {
    if (value === undefined || value === null || value === "") {
      storage.removeItem(key);
      return;
    }
    storage.setItem(key, String(value));
  } catch (_error) {
    // ignore storage write failure in prototype shell
  }
}

function readJson(key, fallback = null) {
  const storage = readStorageArea();
  if (!storage) {
    return fallback;
  }
  try {
    const raw = storage.getItem(key);
    if (!raw) {
      return fallback;
    }
    return JSON.parse(raw);
  } catch (_error) {
    return fallback;
  }
}

function writeJson(key, value) {
  const storage = readStorageArea();
  if (!storage) {
    return;
  }
  try {
    storage.setItem(key, JSON.stringify(value));
  } catch (_error) {
    // ignore storage write failure in prototype shell
  }
}

function normalizeLiveState(state) {
  const source = state && typeof state === "object" && !Array.isArray(state) ? state : {};
  return {
    mailboxByAccountId: clonePlainObject(source.mailboxByAccountId),
    settingsByAccountId: clonePlainObject(source.settingsByAccountId),
    localMessagesByAccountId: clonePlainObject(source.localMessagesByAccountId),
    conversationMetaByAccountId: clonePlainObject(source.conversationMetaByAccountId),
  };
}

export function readLivePostOfficeState() {
  return normalizeLiveState(readJson(LIVE_STATE_STORAGE_KEY, defaultLiveState));
}

export function updateLivePostOfficeState(updater) {
  const current = readLivePostOfficeState();
  const draft = normalizeLiveState(current);
  const next = typeof updater === "function" ? updater(draft) || draft : draft;
  const normalized = normalizeLiveState(next);
  writeJson(LIVE_STATE_STORAGE_KEY, normalized);
  return normalized;
}

export function readStoredUnifiedSession() {
  return {
    token: readString("token") || readString("access_token"),
    refreshToken: readString("refreshToken"),
    tokenExpiresAt: Number(readString("tokenExpiresAt") || 0),
    authMode: readString("authMode"),
    userProfile: readJson("userProfile", null),
  };
}

export function persistUnifiedSession({ token, refreshToken, expiresIn, authMode, userProfile }) {
  const normalizedToken = String(token || "").trim();
  const normalizedRefreshToken = String(refreshToken || "").trim();
  const normalizedAuthMode = String(authMode || "").trim();
  const expiresInMs = Number(expiresIn || 0) > 0 ? Date.now() + Number(expiresIn) * 1000 : 0;

  writeString("token", normalizedToken);
  writeString("refreshToken", normalizedRefreshToken);
  writeString("authMode", normalizedAuthMode || "user");
  writeString("tokenExpiresAt", expiresInMs > 0 ? String(expiresInMs) : "");

  if (userProfile && typeof userProfile === "object" && !Array.isArray(userProfile)) {
    writeJson("userProfile", userProfile);
  }
}

export function clearUnifiedSession() {
  writeString("token", "");
  writeString("access_token", "");
  writeString("refreshToken", "");
  writeString("tokenExpiresAt", "");
  writeString("authMode", "");
  const storage = readStorageArea();
  if (!storage) {
    return;
  }
  try {
    storage.removeItem("userProfile");
  } catch (_error) {
    // ignore storage write failure in prototype shell
  }
}
