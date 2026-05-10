function trimText(value) {
  return String(value == null ? "" : value).trim();
}

function resolveAssetPreviewUrl(source) {
  if (!source || typeof source !== "object") {
    return "";
  }

  return trimText(source.previewUrl || source.preview_url);
}

function resolveAssetCanonicalUrl(source) {
  if (!source || typeof source !== "object") {
    return "";
  }

  return trimText(source.asset_url || source.assetUrl);
}

function resolveAssetDirectUrl(source) {
  if (!source || typeof source !== "object") {
    return "";
  }

  return trimText(source.url || source.imageUrl || source.image_url);
}

export const HTTP_STATUS_ERROR_CODE_OVERRIDES = Object.freeze({
  400: "INVALID_ARGUMENT",
  401: "UNAUTHORIZED",
  403: "FORBIDDEN",
  404: "NOT_FOUND",
  405: "METHOD_NOT_ALLOWED",
  409: "CONFLICT",
  410: "GONE",
  413: "PAYLOAD_TOO_LARGE",
  429: "TOO_MANY_REQUESTS",
  502: "UPSTREAM_UNAVAILABLE",
  503: "UPSTREAM_UNAVAILABLE",
  504: "UPSTREAM_TIMEOUT",
});

export const HTTP_ENVELOPE_CODES = Object.freeze([
  "OK",
  ...Array.from(new Set(Object.values(HTTP_STATUS_ERROR_CODE_OVERRIDES))).sort(),
  "INTERNAL_ERROR",
  "REQUEST_FAILED",
]);

const PROTECTED_UPLOAD_PATH_PREFIXES = [
  "/uploads/certs/",
  "/uploads/merchant_document/",
  "/uploads/medical_document/",
  "/uploads/onboarding-invite/",
];

export function isProtectedUploadUrl(url) {
  const raw = trimText(url);
  if (!raw) {
    return false;
  }

  let pathname = raw;
  if (/^https?:\/\//i.test(raw)) {
    try {
      pathname = trimText(new URL(raw).pathname);
    } catch {
      pathname = raw;
    }
  }

  return PROTECTED_UPLOAD_PATH_PREFIXES.some((prefix) =>
    pathname.startsWith(prefix),
  );
}

export function normalizeErrorCode(status, explicitCode = "") {
  const code = trimText(explicitCode);
  if (code) {
    return code;
  }

  const normalizedStatus = Number(status);
  if (HTTP_STATUS_ERROR_CODE_OVERRIDES[normalizedStatus]) {
    return HTTP_STATUS_ERROR_CODE_OVERRIDES[normalizedStatus];
  }

  return normalizedStatus >= 500 ? "INTERNAL_ERROR" : "REQUEST_FAILED";
}

function resolveEnvelopeRequestIdSource(source, upstreamPayload = {}) {
  if (!source || typeof source !== "object") {
    return trimText(upstreamPayload?.request_id || upstreamPayload?.requestId);
  }

  return trimText(
    upstreamPayload?.request_id ||
      upstreamPayload?.requestId ||
      source?.requestId ||
      source?.headers?.["x-request-id"] ||
      source?.headers?.["X-Request-ID"],
  );
}

function normalizeDataPayload(data) {
  if (data && typeof data === "object" && !Array.isArray(data)) {
    return data;
  }
  return {};
}

function normalizeSuccessData(data) {
  if (data === undefined || data === null) {
    return {};
  }
  return data;
}

function normalizeBooleanFlag(value) {
  if (typeof value === "boolean") {
    return value;
  }

  const normalized = trimText(value).toLowerCase();
  if (!normalized) {
    return undefined;
  }

  if (normalized === "true" || normalized === "1" || normalized === "yes") {
    return true;
  }
  if (normalized === "false" || normalized === "0" || normalized === "no") {
    return false;
  }

  return Boolean(value);
}

function normalizeIdentityScope(value) {
  if (Array.isArray(value)) {
    return value.map((item) => trimText(item)).filter(Boolean);
  }

  const normalized = trimText(value);
  if (!normalized) {
    return [];
  }

  return normalized
    .split(/[,\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizeNumberValue(value, fallback = 0) {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : fallback;
}

function normalizePlainObject(value) {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value
    : null;
}

function normalizeAuthSessionUser(source) {
  const user =
    normalizePlainObject(source?.user) || normalizePlainObject(source?.profile);
  return user ? { ...user } : null;
}

export function buildSuccessEnvelopePayload(source, message, data, options = {}) {
  const normalizedMessage = trimText(message) || "ok";
  const payload = {
    request_id: resolveEnvelopeRequestIdSource(source, options.upstreamPayload),
    code: trimText(options.code) || "OK",
    message: normalizedMessage,
    data: normalizeSuccessData(data),
    success: true,
  };

  const legacy = options.legacy;
  if (legacy && typeof legacy === "object") {
    for (const [key, value] of Object.entries(legacy)) {
      if (payload[key] === undefined) {
        payload[key] = value;
      }
    }
  }

  return payload;
}

export function buildErrorEnvelopePayload(source, status, message, options = {}) {
  const normalizedMessage = trimText(message) || "Request failed";
  const payload = {
    request_id: resolveEnvelopeRequestIdSource(source, options.upstreamPayload),
    code: normalizeErrorCode(status, options.code),
    message: normalizedMessage,
    data: normalizeDataPayload(options.data),
    success: false,
    error: normalizedMessage,
  };

  const legacy = options.legacy;
  if (legacy && typeof legacy === "object") {
    for (const [key, value] of Object.entries(legacy)) {
      if (payload[key] === undefined) {
        payload[key] = value;
      }
    }
  }

  return payload;
}

export function extractEnvelopeData(payload) {
  if (
    payload &&
    typeof payload === "object" &&
    payload.data !== undefined
  ) {
    return payload.data;
  }
  return payload;
}

export function extractEnvelopeRequestId(payload) {
  if (!payload || typeof payload !== "object") {
    return "";
  }
  return trimText(payload.request_id || payload.requestId);
}

export function extractEnvelopeCode(payload, fallback = "OK") {
  if (!payload || typeof payload !== "object") {
    return fallback;
  }
  return trimText(payload.code) || fallback;
}

export function extractEnvelopeMessage(payload, fallback = "") {
  if (!payload || typeof payload !== "object") {
    return fallback;
  }
  return trimText(payload.message || payload.error) || fallback;
}

export function extractPaginatedItems(payload, options = {}) {
  const data = extractEnvelopeData(payload);
  const listKeys = Array.isArray(options.listKeys) && options.listKeys.length > 0
    ? options.listKeys
    : ["items", "records", "list"];
  const pagination = data && typeof data === "object" && data.pagination && typeof data.pagination === "object"
    ? data.pagination
    : {};
  const normalized = {
    items: [],
    total: 0,
    page: 0,
    limit: 0,
  };

  if (Array.isArray(data)) {
    normalized.items = data;
    normalized.total = data.length;
    return normalized;
  }

  if (!data || typeof data !== "object") {
    return normalized;
  }

  for (const key of listKeys) {
    if (Array.isArray(data[key])) {
      normalized.items = data[key];
      break;
    }
  }

  normalized.total = Number(
    data.total ?? pagination.total ?? payload?.total ?? normalized.items.length ?? 0,
  );
  normalized.page = Number(data.page ?? pagination.page ?? payload?.page ?? 0);
  normalized.limit = Number(
    data.limit ??
      data.pageSize ??
      data.page_size ??
      pagination.limit ??
      pagination.pageSize ??
      pagination.page_size ??
      payload?.limit ??
      payload?.pageSize ??
      payload?.page_size ??
      0,
  );
  return normalized;
}

function resolveCredentialSource(payload = {}) {
  if (payload?.temporaryCredential && typeof payload.temporaryCredential === "object") {
    return payload.temporaryCredential;
  }
  if (payload?.data?.temporaryCredential && typeof payload.data.temporaryCredential === "object") {
    return payload.data.temporaryCredential;
  }
  return null;
}

export function extractTemporaryCredential(payload = {}) {
  const source = resolveCredentialSource(payload);
  const temporaryPassword = trimText(source?.temporaryPassword);

  if (!temporaryPassword) {
    return null;
  }

  return {
    temporaryPassword,
    deliveryMode: trimText(source?.deliveryMode || "operator_receipt") || "operator_receipt",
    subjectHint: trimText(source?.subjectHint),
  };
}

export function extractUploadAsset(payload) {
  const data = extractEnvelopeData(payload);
  if (!data || typeof data !== "object") {
    return null;
  }
  const assetId = String(data.asset_id || data.assetId || data.assetRef || "").trim();
  const url =
    resolveAssetCanonicalUrl(data) ||
    resolveAssetPreviewUrl(data) ||
    resolveAssetDirectUrl(data);
  if (!url) {
    if (!assetId) {
      return null;
    }
  }
  return {
    ...data,
    asset_id: assetId,
    url,
    filename: String(data.filename || "").trim(),
  };
}

export function resolveUploadAssetUrl(payload) {
  if (typeof payload === "string") {
    return trimText(payload);
  }

  const asset = extractUploadAsset(payload) || payload;
  if (!asset || typeof asset !== "object") {
    return "";
  }

  const accessPolicy = trimText(asset.access_policy || asset.accessPolicy).toLowerCase();
  const previewUrl = resolveAssetPreviewUrl(asset);
  const assetUrl = resolveAssetCanonicalUrl(asset);
  const directUrl = resolveAssetDirectUrl(asset);

  if (accessPolicy === "private") {
    return previewUrl || assetUrl || directUrl;
  }

  if (previewUrl && (isProtectedUploadUrl(assetUrl) || isProtectedUploadUrl(directUrl))) {
    return previewUrl;
  }

  return assetUrl || directUrl || previewUrl;
}

export function extractSMSResult(payload) {
  if (!payload || typeof payload !== "object") {
    return {};
  }

  const data = extractEnvelopeData(payload);
  const source = data && typeof data === "object" ? data : payload;
  const successValue =
    source.success !== undefined ? source.success : payload.success;
  const success =
    successValue === undefined ? undefined : Boolean(successValue);
  const message = trimText(
    source.message || payload.message || payload.error,
  );
  const explicitError = trimText(payload.error || source.error);
  const smsCode = trimText(
    source.code ||
      source.sms_code ||
      source.smsCode ||
      source.verification_code ||
      source.verificationCode ||
      payload.sms_code ||
      payload.smsCode ||
      payload.verification_code ||
      payload.verificationCode,
  );

  return {
    ...payload,
    success,
    message,
    error: explicitError || (success === false ? message : ""),
    needCaptcha: Boolean(
      source.needCaptcha ??
        source.need_captcha ??
        payload.needCaptcha ??
        payload.need_captcha,
    ),
    sessionId: trimText(
      source.sessionId ||
        source.session_id ||
        payload.sessionId ||
        payload.session_id,
    ),
    code: smsCode,
    smsCode,
  };
}

export function extractAuthSessionResult(payload) {
  if (!payload || typeof payload !== "object") {
    return {
      request_id: "",
      code: "",
      message: "",
      success: false,
      authenticated: false,
      token: "",
      refreshToken: "",
      expiresIn: 0,
      user: null,
      error: "",
      needRegister: false,
      type: "",
      bindToken: "",
      nickname: "",
      avatarUrl: "",
    };
  }

  const data = extractEnvelopeData(payload);
  const source = normalizePlainObject(data) || normalizePlainObject(payload) || {};
  const sessionSource =
    normalizePlainObject(source.session) ||
    normalizePlainObject(source.authSession) ||
    null;
  const bindingSource =
    normalizePlainObject(source.binding) ||
    normalizePlainObject(source.bind) ||
    null;
  const token = trimText(
    sessionSource?.token ||
      sessionSource?.accessToken ||
      sessionSource?.access_token ||
      source.token ||
      source.accessToken ||
      source.access_token,
  );
  const refreshToken = trimText(
    sessionSource?.refreshToken ||
      sessionSource?.refresh_token ||
      source.refreshToken ||
      source.refresh_token,
  );
  const type = trimText(source.type);
  const bindToken = trimText(
    bindingSource?.bindToken ||
      bindingSource?.bind_token ||
      source.bindToken ||
      source.bind_token,
  );
  const needRegister =
    normalizeBooleanFlag(source.needRegister ?? source.need_register) === true;
  const explicitSuccess = normalizeBooleanFlag(source.success ?? payload.success);
  const explicitAuthenticated = normalizeBooleanFlag(
    source.authenticated ?? source.isAuthenticated,
  );
  const authenticated =
    explicitAuthenticated !== undefined
      ? explicitAuthenticated
      : Boolean(token);
  const success =
    explicitSuccess !== undefined
      ? explicitSuccess
      : type === "error"
        ? false
        : authenticated ||
          (type === "login" && normalizeAuthSessionUser(source)) ||
          (type === "bind_required" && bindToken);
  const message =
    trimText(source.message || source.error) ||
    extractEnvelopeMessage(payload, "");

  return {
    request_id: extractEnvelopeRequestId(payload),
    code: extractEnvelopeCode(payload, success ? "OK" : ""),
    message,
    success,
    authenticated,
    token,
    refreshToken,
    expiresIn: normalizeNumberValue(
      sessionSource?.expiresIn ??
        sessionSource?.expires_in ??
        source.expiresIn ??
        source.expires_in,
      0,
    ),
    user: normalizeAuthSessionUser(source) || normalizeAuthSessionUser(sessionSource),
    error: trimText(source.error || (success ? "" : message)),
    needRegister,
    type,
    bindToken,
    nickname: trimText(bindingSource?.nickname || source.nickname),
    avatarUrl: trimText(
      bindingSource?.avatarUrl ||
        bindingSource?.avatar_url ||
        source.avatarUrl ||
        source.avatar_url,
    ),
  };
}

export function extractAuthVerifyResult(payload) {
  if (!payload || typeof payload !== "object") {
    return {
      request_id: "",
      code: "",
      message: "",
      valid: false,
      identity: null,
    };
  }

  const data = extractEnvelopeData(payload);
  const source = data && typeof data === "object" ? data : payload;
  const identitySource =
    source.identity && typeof source.identity === "object"
      ? source.identity
      : source;

  const principalId = trimText(
    identitySource.principalId ||
      identitySource.principal_id ||
      source.principalId ||
      source.principal_id ||
      source.id,
  );
  const legacyId = trimText(
    identitySource.legacyId ||
      identitySource.legacy_id ||
      identitySource.userId ||
      identitySource.user_id ||
      source.legacyId ||
      source.legacy_id ||
      source.userId ||
      source.user_id,
  );
  const phone = trimText(identitySource.phone || source.phone);
  const principalType = trimText(
    identitySource.principalType ||
      identitySource.principal_type ||
      source.principalType ||
      source.principal_type,
  );
  const role = trimText(identitySource.role || source.role);
  const sessionId = trimText(
    identitySource.sessionId ||
      identitySource.session_id ||
      source.sessionId ||
      source.session_id,
  );
  const name = trimText(identitySource.name || source.name);
  const explicitValid = normalizeBooleanFlag(source.valid ?? payload.valid);
  const success = normalizeBooleanFlag(payload.success);
  const hasIdentity = Boolean(
    principalId ||
      legacyId ||
      phone ||
      principalType ||
      role ||
      sessionId ||
      name,
  );
  const valid =
    explicitValid !== undefined
      ? explicitValid
      : success === false
        ? false
        : hasIdentity;

  return {
    request_id: extractEnvelopeRequestId(payload),
    code: extractEnvelopeCode(payload, valid ? "OK" : ""),
    message: extractEnvelopeMessage(payload, ""),
    valid,
    identity: hasIdentity
      ? {
          id: trimText(principalId || legacyId || phone),
          principalId,
          principalType,
          legacyId,
          userId: legacyId,
          phone,
          role,
          sessionId,
          scope: normalizeIdentityScope(identitySource.scope ?? source.scope),
          name,
        }
      : null,
  };
}

export function extractErrorMessage(payload, fallback = "请求失败") {
  if (!payload) {
    return fallback;
  }

  if (typeof payload === "string") {
    return payload.trim() || fallback;
  }

  if (payload.error && typeof payload.error === "string") {
    return payload.error;
  }

  if (payload.message && typeof payload.message === "string") {
    return payload.message;
  }

  if (payload.response?.data) {
    return extractErrorMessage(payload.response.data, fallback);
  }

  return fallback;
}
