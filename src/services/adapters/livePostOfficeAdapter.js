import { extractEnvelopeData } from "@infinitech/contracts/http";
import { runtimeConfig } from "../../lib/config/runtime";
import { createHttpClient } from "../httpClient";
import {
  POST_OFFICE_DEFAULT_FILTER,
  POST_OFFICE_DEFAULT_FOLDER,
  buildPostOfficeContractHeaders,
  getPostOfficeOperationSchemaNames,
  getPostOfficeContractOperation,
  normalizePostOfficeFolder,
  normalizePostOfficeMessageFilter,
  pickPostOfficeSettingsPatch,
  resolvePostOfficeOperationPath,
} from "../postOfficeContract";
import {
  clearUnifiedSession,
  persistUnifiedSession,
  readStoredUnifiedSession,
} from "../live/postOfficeLiveState";

const mailHttp = createHttpClient({
  baseUrl: runtimeConfig.mailApiBaseUrl,
  timeoutMs: runtimeConfig.requestTimeoutMs,
});

const MAIL_API_UNAVAILABLE_STATUSES = new Set([404, 405, 501, 502, 503, 504]);
const ADMIN_SESSION_STORAGE_KEY = "infinitemail.admin.session.v1";

function normalizePlainObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function normalizeString(value) {
  return String(value == null ? "" : value).trim();
}

function normalizeArray(value) {
  return Array.isArray(value) ? value : [];
}

function escapeHtml(value) {
  return normalizeString(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function ensureBearerToken(token) {
  const normalized = normalizeString(token);
  if (!normalized) {
    return "";
  }
  return /^bearer\s+/i.test(normalized) ? normalized : `Bearer ${normalized}`;
}

function buildAuthHeaders(token) {
  const authorization = ensureBearerToken(token);
  return authorization ? { Authorization: authorization } : {};
}

function buildContractRequestHeaders(snapshot, operationId) {
  return {
    ...buildAuthHeaders(snapshot?.token),
    ...buildPostOfficeContractHeaders(operationId),
  };
}

function isMailApiUnavailableError(error) {
  const status = Number(error?.status || 0);
  return !status || MAIL_API_UNAVAILABLE_STATUSES.has(status);
}

function isBffSnapshot(snapshot) {
  return normalizeString(snapshot?.token).startsWith("bff_") ||
    normalizePlainObject(snapshot?.userProfile).source === "infinitemail-bff";
}

function buildBffAuthHeaders() {
  return buildAuthHeaders(readStoredUnifiedSession().token);
}

function buildBffAdminHeaders() {
  const adminToken = normalizeString(runtimeConfig.adminApiToken);
  const adminSession = readStoredAdminSession();
  return {
    ...buildBffAuthHeaders(),
    ...(adminToken ? { "X-Admin-Token": adminToken } : {}),
    ...(!adminToken && adminSession.token ? { Authorization: ensureBearerToken(adminSession.token) } : {}),
  };
}

function readStoredAdminSession() {
  if (typeof window === "undefined") {
    return {};
  }
  try {
    const raw = window.localStorage?.getItem(ADMIN_SESSION_STORAGE_KEY);
    return raw ? normalizePlainObject(JSON.parse(raw)) : {};
  } catch (_error) {
    return {};
  }
}

function persistAdminSession(session) {
  if (typeof window === "undefined") {
    return;
  }
  const token = normalizeString(session?.token);
  try {
    if (!token) {
      window.localStorage?.removeItem(ADMIN_SESSION_STORAGE_KEY);
      return;
    }
    window.localStorage?.setItem(ADMIN_SESSION_STORAGE_KEY, JSON.stringify({
      token,
      username: normalizeString(session?.username || "admin"),
      expiresAt: normalizeString(session?.expiresAt),
    }));
  } catch (_error) {
    // ignore storage failures in restricted browser shells
  }
}

function clearAdminSession() {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage?.removeItem(ADMIN_SESSION_STORAGE_KEY);
  } catch (_error) {
    // ignore storage failures in restricted browser shells
  }
}

async function requestBffSession() {
  return mailHttp.get("/auth/session", {
    headers: buildBffAuthHeaders(),
  });
}

function persistBffSession(payload) {
  if (!payload?.token) {
    return payload;
  }

  const profile = normalizePlainObject(payload.profile);
  const user = normalizePlainObject(payload.user);
  persistUnifiedSession({
    token: payload.token,
    refreshToken: payload.refreshToken || "",
    expiresIn: payload.expiresIn || 30 * 24 * 60 * 60,
    authMode: user.authMode || profile.authMode || "user",
    userProfile: {
      ...user,
      ...profile,
      id: normalizeString(user.id || profile.id),
      phone: normalizeString(user.phone || profile.phone),
      name: normalizeString(user.name || user.nickname || profile.displayName),
      nickname: normalizeString(user.nickname || user.name || profile.displayName),
      authMode: user.authMode || profile.authMode || "user",
      source: "infinitemail-bff",
    },
  });
  return payload;
}

function normalizeBffProfile(sessionPayload) {
  const payload = normalizePlainObject(sessionPayload);
  return normalizeContractProfile(payload.profile, payload.user || payload.profile);
}

function maskPhone(phone) {
  const normalized = normalizeString(phone);
  if (!/^1\d{10}$/.test(normalized)) {
    return normalized;
  }
  return `${normalized.slice(0, 3)}****${normalized.slice(-4)}`;
}

function resolveAuthMode(candidate, fallback = "user") {
  const normalized = normalizeString(candidate).toLowerCase();
  if (normalized === "user") {
    return normalized;
  }
  return fallback;
}

function resolveRolePrefix(authMode) {
  return resolveAuthMode(authMode) === "user" ? "user-" : "user-";
}

function resolveSelfRoleTitle(authMode) {
  return resolveAuthMode(authMode) === "user" ? "悦享用户" : "悦享用户";
}

function deriveAvatarText(value, fallback = "Y") {
  const source = normalizeString(value);
  if (!source) {
    return fallback;
  }
  return source.slice(0, 1).toUpperCase();
}

function formatListTime(timestamp) {
  const date = new Date(timestamp || Date.now());
  if (Number.isNaN(date.getTime())) {
    return "--:--";
  }

  const now = new Date();
  const sameYear = now.getFullYear() === date.getFullYear();
  const sameMonth = now.getMonth() === date.getMonth();
  const sameDate = now.getDate() === date.getDate();

  if (sameYear && sameMonth && sameDate) {
    return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
  }

  return `${date.getMonth() + 1}月${date.getDate()}日`;
}

function formatDateTimeLabel(timestamp) {
  const date = new Date(timestamp || Date.now());
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日 ${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

function nextLocalMessageId(prefix = "mail") {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function resolveMailboxDomain() {
  return normalizeString(runtimeConfig.mailboxDomain) || "yuexiang.com";
}

function buildDefaultSettings(profile) {
  return {
    defaultSenderName: `${resolveSelfRoleTitle(profile.authMode)}_${profile.displayName}`,
    signature: "--\n此致\n悦享邮局",
    autoReplyEnabled: false,
    autoReplyMessage: "您好，我暂时无法及时回复，稍后会第一时间处理您的来信。",
    updatedAt: null,
  };
}

function normalizeTemplateItems(payload, profile) {
  const data = extractEnvelopeData(payload) || payload || {};
  const items = normalizeArray(data.items).map((item) => ({
    id: normalizeString(item?.id || `template-${item?.role || "user"}`),
    role: normalizeString(item?.role || "user"),
    subject: normalizeString(item?.subject || "公司邮箱使用邀请"),
    html: normalizeString(item?.html || item?.content || ""),
  })).filter((item) => item.id && item.html);
  return items;
}

function normalizeContactItems(payload) {
  const data = extractEnvelopeData(payload) || payload || {};
  return normalizeArray(data.items).map((contact) => ({
    ...contact,
    id: normalizeString(contact?.id || contact?.email),
    name: normalizeString(contact?.name || contact?.email || "邮件联系人"),
    email: normalizeString(contact?.email),
    avatar: normalizeString(contact?.avatar || deriveAvatarText(contact?.name || contact?.email, "I")),
    role: normalizeString(contact?.role || "邮件联系人"),
    organization: normalizeString(contact?.organization || "外部邮箱"),
    lastContactedAt: normalizeString(contact?.lastContactedAt || "-"),
    note: normalizeString(contact?.note || "基于真实邮件往来自动沉淀"),
    tags: normalizeArray(contact?.tags),
    stats: {
      totalMessages: Number(contact?.stats?.totalMessages || 0),
      inviteCount: Number(contact?.stats?.inviteCount || 0),
    },
  })).filter((contact) => contact.id && contact.email);
}

function normalizeContactThreadPayload(payload) {
  const data = extractEnvelopeData(payload) || payload || {};
  return {
    contact: normalizePlainObject(data.contact),
    items: normalizeArray(data.items).map((item) => ({
      ...normalizeContractMessage(item, {
        folder: normalizeString(item?.folderId || item?.folder || "inbox"),
      }),
      folderId: normalizeString(item?.folderId || item?.folder || "inbox"),
    })),
    total: Number(data.total || normalizeArray(data.items).length || 0),
  };
}

function normalizeSecuritySessionsPayload(payload) {
  const data = extractEnvelopeData(payload) || payload || {};
  return {
    items: normalizeArray(data.items).map((session) => ({
      id: normalizeString(session?.id),
      device: normalizeString(session?.device || "未知设备"),
      ip: normalizeString(session?.ip || "-"),
      userAgent: normalizeString(session?.userAgent),
      createdAt: normalizeString(session?.createdAt),
      lastSeenAt: normalizeString(session?.lastSeenAt || session?.createdAt),
      expiresAt: normalizeString(session?.expiresAt),
      current: Boolean(session?.current),
    })).filter((session) => session.id),
    removed: Number(data.removed || 0),
  };
}
function deriveFolderCounts(messages) {
  return {
    inbox: messages.filter((message) => message.folder === "inbox" && message.isUnread).length,
    starred: messages.filter((message) => message.isStarred && message.folder !== "trash" && message.folder !== "archive").length,
    sent: messages.filter((message) => message.folder === "sent").length,
    drafts: messages.filter((message) => message.folder === "drafts").length,
    trash: messages.filter((message) => message.folder === "trash").length,
  };
}

function normalizeFolderCounts(source, fallbackMessages = []) {
  const fallback = deriveFolderCounts(fallbackMessages);
  const counts = normalizePlainObject(source);
  return {
    inbox: Number.isFinite(Number(counts.inbox)) ? Number(counts.inbox) : fallback.inbox,
    starred: Number.isFinite(Number(counts.starred)) ? Number(counts.starred) : fallback.starred,
    sent: Number.isFinite(Number(counts.sent)) ? Number(counts.sent) : fallback.sent,
    drafts: Number.isFinite(Number(counts.drafts)) ? Number(counts.drafts) : fallback.drafts,
    trash: Number.isFinite(Number(counts.trash)) ? Number(counts.trash) : fallback.trash,
  };
}

function shouldResetSession(error) {
  return Number(error?.status || 0) === 401 || Number(error?.status || 0) === 403;
}

function normalizeAttachmentPayload(attachment) {
  const source = normalizePlainObject(attachment);
  return {
    id: normalizeString(source.id || source.assetId || source.asset_id || nextLocalMessageId("asset")),
    assetId: normalizeString(source.assetId || source.asset_id) || null,
    name: normalizeString(source.name || source.filename || "未命名附件"),
    type: normalizeString(source.type || source.contentType || source.content_type || "file"),
    contentType: normalizeString(source.contentType || source.content_type) || null,
    sizeBytes: Number.isFinite(Number(source.sizeBytes)) ? Number(source.sizeBytes) : null,
    sizeLabel: normalizeString(source.sizeLabel || source.size || "--"),
    downloadUrl: normalizeString(source.downloadUrl || source.url) || null,
    previewUrl: normalizeString(source.previewUrl) || null,
    contentEncoding: normalizeString(source.contentEncoding || source.content_encoding) || null,
    contentBase64: normalizeString(source.contentBase64 || source.content_base64) || null,
  };
}

function normalizeContractMessage(message, fallback = {}) {
  const source = normalizePlainObject(message);
  const fallbackSource = normalizePlainObject(fallback);
  const timestamp = normalizeString(source.sortAt || source.sentAt || source.receivedAt || fallbackSource.sortAt);
  const resolvedTimestamp = timestamp || new Date().toISOString();
  const folder = normalizePostOfficeFolder(source.folder || fallbackSource.folder, POST_OFFICE_DEFAULT_FOLDER);
  const normalizedSubject = normalizeString(source.subject || fallbackSource.subject || "未命名邮件");
  const normalizedSnippet = normalizeString(source.snippet || fallbackSource.snippet || "");
  const sentAt = normalizeString(source.sentAt);
  const receivedAt = normalizeString(source.receivedAt);

  return {
    id: normalizeString(source.id || fallbackSource.id || nextLocalMessageId("mail")),
    threadId: normalizeString(source.threadId || source.thread_id) || null,
    folder,
    previousFolder: normalizeString(source.previousFolder || fallbackSource.previousFolder) || folder,
    sender: normalizeString(source.sender || fallbackSource.sender || "邮件联系人"),
    senderEmail: normalizeString(source.senderEmail || fallbackSource.senderEmail || ""),
    recipients: normalizeArray(source.recipients || fallbackSource.recipients).map((item) => normalizeString(item)).filter(Boolean),
    cc: normalizeArray(source.cc || fallbackSource.cc).map((item) => normalizeString(item)).filter(Boolean),
    bcc: normalizeArray(source.bcc || fallbackSource.bcc).map((item) => normalizeString(item)).filter(Boolean),
    avatar: deriveAvatarText(source.avatar || fallbackSource.avatar || source.sender || "Y", "Y"),
    role: normalizeString(source.role || fallbackSource.role || "邮件联系人"),
    subject: normalizedSubject,
    snippet: normalizedSnippet,
    time: normalizeString(source.time || fallbackSource.time) || formatListTime(resolvedTimestamp),
    dateTimeLabel: normalizeString(source.dateTimeLabel || fallbackSource.dateTimeLabel) || formatDateTimeLabel(resolvedTimestamp),
    sortAt: resolvedTimestamp,
    sentAt: sentAt || null,
    receivedAt: receivedAt || null,
    isUnread: Boolean(source.isUnread ?? fallbackSource.isUnread),
    isStarred: Boolean(source.isStarred ?? fallbackSource.isStarred),
    hasAttachment: Boolean(source.hasAttachment ?? normalizeArray(source.attachments).length > 0),
    tags: normalizeArray(source.tags || fallbackSource.tags).map((item) => normalizeString(item)).filter(Boolean),
    isOutgoing: Boolean(source.isOutgoing ?? fallbackSource.isOutgoing),
    content: normalizeString(source.content || fallbackSource.content),
    attachments: normalizeArray(source.attachments || fallbackSource.attachments).map(normalizeAttachmentPayload),
    source: normalizeString(source.source || fallbackSource.source || "mailbox"),
    deliveryStatus: normalizeString(source.deliveryStatus || fallbackSource.deliveryStatus || "received"),
    acceptedAt: normalizeString(source.acceptedAt || source.accepted_at || fallbackSource.acceptedAt) || null,
    providerMessageId: normalizeString(source.providerMessageId || source.provider_message_id || fallbackSource.providerMessageId) || null,
    deliveryError: normalizeString(source.deliveryError || source.delivery_error || fallbackSource.deliveryError) || null,
    replyToMessageId: normalizeString(source.replyToMessageId || source.reply_to_message_id) || null,
    meta: normalizePlainObject(source.meta),
  };
}

function normalizeContractSettings(settings, profile) {
  return {
    ...buildDefaultSettings(profile),
    ...pickPostOfficeSettingsPatch(normalizePlainObject(settings)),
    updatedAt: normalizeString(settings?.updatedAt || settings?.updated_at) || null,
  };
}

function normalizeProvisioningStatus(value, hasEmail = false) {
  switch (normalizeString(value).toLowerCase()) {
    case "provisioned":
    case "active":
      return "provisioned";
    case "queued":
    case "pending":
      return "queued";
    case "failed":
      return "failed";
    case "pending_config":
      return "pending_config";
    default:
      return hasEmail ? "queued" : "pending_config";
  }
}

function normalizeContractProfile(profilePayload, fallbackUser = {}) {
  const profile = normalizePlainObject(profilePayload);
  const user = normalizePlainObject(fallbackUser);
  const authMode = resolveAuthMode(
    profile.authMode || user.authMode || user.principalType || user.role || user.type,
  );
  const displayName = normalizeString(profile.displayName || user.nickname || user.name || "悦享用户");
  const email = normalizeString(profile.email);
  const provisioningStatus = normalizeProvisioningStatus(profile.provisioningStatus, Boolean(email));
  const mailboxProvisioned = Boolean(profile.mailboxProvisioned) || provisioningStatus === "provisioned";

  return {
    id: normalizeString(profile.id || profile.sourceUserId || user.id || user.phone),
    displayName,
    avatarInitial: deriveAvatarText(profile.avatarInitial || displayName, "Y"),
    unifiedAccountPhone: normalizeString(profile.unifiedAccountPhone || maskPhone(user.phone)),
    rolePrefix: normalizeString(profile.rolePrefix || resolveRolePrefix(authMode)),
    emailPrefix: normalizeString(profile.emailPrefix),
    email,
    mailboxDomain: normalizeString(profile.mailboxDomain || resolveMailboxDomain()),
    mailboxProvisioned,
    provisioningStatus,
    authMode,
    sourceUserId: normalizeString(profile.sourceUserId || user.id || user.phone),
    createdAt: normalizeString(profile.createdAt || profile.created_at) || null,
    updatedAt: normalizeString(profile.updatedAt || profile.updated_at) || null,
    provisionedAt: normalizeString(profile.provisionedAt || profile.provisioned_at) || null,
    sourceUser: user,
  };
}

async function requestPostOfficeOperation(snapshot, operationId, { pathParams, query, body } = {}) {
  const operation = getPostOfficeContractOperation(operationId);
  if (!operation) {
    throw new Error(`未找到邮局契约操作: ${operationId}`);
  }

  const path = resolvePostOfficeOperationPath(operationId, pathParams);
  const headers = buildContractRequestHeaders(snapshot, operationId);
  const method = normalizeString(operation.method).toUpperCase();

  switch (method) {
    case "GET":
      return extractEnvelopeData(
        await mailHttp.get(path, {
          query,
          headers,
        }),
      );
    case "POST":
      return extractEnvelopeData(
        await mailHttp.post(path, {
          body,
          headers,
        }),
      );
    case "PUT":
      return extractEnvelopeData(
        await mailHttp.put(path, {
          body,
          headers,
        }),
      );
    case "PATCH":
      return extractEnvelopeData(
        await mailHttp.patch(path, {
          body,
          headers,
        }),
      );
    default:
      throw new Error(`不支持的邮局契约方法: ${operation.method}`);
  }
}


async function requirePostOfficeOperation(snapshot, operationId, options = {}) {
  try {
    return await requestPostOfficeOperation(snapshot, operationId, options);
  } catch (error) {
    if (shouldResetSession(error)) {
      clearUnifiedSession();
    }
    const schemaMeta = getPostOfficeOperationSchemaNames(operationId);
    const message = normalizeString(error?.message || "邮局服务请求失败");
    error.message = [
      message,
      `(operation=${operationId}`,
      `request=${schemaMeta.request || "none"}`,
      `response=${schemaMeta.responseData || "none"})`,
    ].join(" ");
    throw error;
  }
}


async function resolveAuthenticatedProfile() {
  try {
    const bffSession = await requestBffSession();
    if (bffSession?.isAuthenticated && bffSession?.profile) {
      persistBffSession(bffSession);
      const nextSnapshot = {
        ...readStoredUnifiedSession(),
        authMode: "user",
      };
      const profile = normalizeBffProfile(bffSession);
      return {
        snapshot: nextSnapshot,
        userPayload: normalizePlainObject(bffSession.user || bffSession.profile),
        profile,
      };
    }
    throw new Error("请先登录越想邮局账号");
  } catch (error) {
    if (shouldResetSession(error)) {
      clearUnifiedSession();
    }
    if (isMailApiUnavailableError(error)) {
      throw new Error("BFF 未连接，邮箱数据源已停止");
    }
    throw error;
  }
}

export const livePostOfficeAdapter = {
  async getSession() {
    const baseRolePrefix = resolveRolePrefix(readStoredUnifiedSession().authMode || "user");
    try {
      const bffSession = await requestBffSession();
      if (bffSession) {
        persistBffSession(bffSession);
        return {
          ...bffSession,
          rolePrefix: bffSession.rolePrefix || baseRolePrefix,
          adminConfig: bffSession.adminConfig,
        };
      }
    } catch (error) {
      if (shouldResetSession(error)) {
        clearUnifiedSession();
      }
    }
    return {
      isAuthenticated: false,
      requiresActivation: true,
      rolePrefix: baseRolePrefix,
    };
  },


  async beginOAuthLogin() {
    try {
      const bffSession = await mailHttp.post("/auth/oauth/start", {
        headers: buildBffAuthHeaders(),
      });
      if (bffSession?.redirectUrl && typeof window !== "undefined") {
        window.location.assign(bffSession.redirectUrl);
        return {
          redirected: true,
          requiresActivation: false,
          rolePrefix: bffSession.rolePrefix || resolveRolePrefix(readStoredUnifiedSession().authMode || "user"),
          adminConfig: bffSession.adminConfig,
        };
      }
      if (bffSession?.isAuthenticated) {
        persistBffSession(bffSession);
        return bffSession;
      }
      return {
        isAuthenticated: false,
        requiresActivation: false,
        rolePrefix: resolveRolePrefix(readStoredUnifiedSession().authMode || "user"),
        errorMessage: "OAuth 登录未返回有效会话",
      };
    } catch (error) {
      if (shouldResetSession(error)) {
        clearUnifiedSession();
      }
      return {
        isAuthenticated: false,
        requiresActivation: false,
        rolePrefix: resolveRolePrefix(readStoredUnifiedSession().authMode || "user"),
        errorMessage: normalizeString(error?.message || "BFF 登录接口不可用"),
      };
    }
  },


  async activateMailbox({ emailPrefix }) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const normalizedPrefix = normalizeString(emailPrefix).toLowerCase().replace(/[^a-z0-9-]/g, "");
    const payload = await requirePostOfficeOperation(snapshot, "activateMailbox", {
      body: {
        emailPrefix: normalizedPrefix,
      },
    });
    const nextProfile = normalizeContractProfile(payload?.mailbox, profile.sourceUser || profile);
    return {
      success: true,
      mailbox: nextProfile,
    };
  },


  async logout() {
    await mailHttp.post("/auth/logout", {
      headers: buildBffAuthHeaders(),
    }).catch(() => null);
    clearUnifiedSession();
    return { success: true };
  },

  async getBootstrap() {
    const { snapshot, userPayload, profile } = await resolveAuthenticatedProfile();
    const remoteMailbox = await requirePostOfficeOperation(snapshot, "getMailboxProfile");
    const nextProfile = normalizeContractProfile(remoteMailbox?.profile || profile, userPayload);
    const [contactsPayload, templatesPayload, remoteSettings] = await Promise.all([
      mailHttp.get("/contacts", {
        headers: buildBffAuthHeaders(),
      }),
      mailHttp.get("/templates", {
        headers: buildBffAuthHeaders(),
      }),
      requirePostOfficeOperation(snapshot, "getSettings"),
    ]);
    const nextSettings = normalizeContractSettings(remoteSettings?.settings, nextProfile);
    return {
      profile: nextProfile,
      health: {
        status: "healthy",
        label: "服务连接正常",
        source: "infinitemail-bff",
      },
      settings: nextSettings,
      contacts: normalizeContactItems(contactsPayload),
      templates: normalizeTemplateItems(templatesPayload, nextProfile),
      folderCounts: normalizeFolderCounts(remoteMailbox?.folderCounts),
    };
  },


  async getHealth() {
    try {
      await mailHttp.get("/auth/session", {
        headers: buildBffAuthHeaders(),
      });
      return {
        status: "healthy",
        label: "服务连接正常",
        source: "infinitemail-bff",
      };
    } catch (_error) {
      return {
        status: "unhealthy",
        label: "BFF 未连接",
        source: "infinitemail-bff",
      };
    }
  },


  async listMessages({ folderId, filter, search }) {
    const { snapshot } = await resolveAuthenticatedProfile();
    const normalizedFolderId = normalizePostOfficeFolder(folderId, POST_OFFICE_DEFAULT_FOLDER);
    const normalizedFilter = normalizePostOfficeMessageFilter(filter, POST_OFFICE_DEFAULT_FILTER);
    const remoteMessages = await requirePostOfficeOperation(snapshot, "listMessages", {
      query: {
        folderId: normalizedFolderId,
        filter: normalizedFilter,
        search: normalizeString(search),
      },
    });
    const items = normalizeArray(remoteMessages.items).map((message) =>
      normalizeContractMessage(message, {
        folder: normalizedFolderId,
      }),
    );
    return {
      items,
      folderCounts: normalizeFolderCounts(remoteMessages.folderCounts, items),
      nextCursor: normalizeString(remoteMessages.nextCursor) || null,
      hasMore: Boolean(remoteMessages.hasMore),
    };
  },


  async toggleStar(messageId, options = {}) {
    const { snapshot } = await resolveAuthenticatedProfile();
    const normalizedMessageId = normalizeString(messageId);
    let nextStarred = typeof options.nextStarred === "boolean" ? options.nextStarred : undefined;
    if (typeof nextStarred !== "boolean") {
      const detail = await requirePostOfficeOperation(snapshot, "getMessageDetail", {
        pathParams: { messageId: normalizedMessageId },
      });
      nextStarred = !Boolean(detail?.message?.isStarred);
    }
    await requirePostOfficeOperation(snapshot, "updateMessageStar", {
      pathParams: { messageId: normalizedMessageId },
      body: {
        starred: Boolean(nextStarred),
      },
    });
    return { success: true };
  },


  async moveMessage(messageId, targetFolder) {
    const { snapshot } = await resolveAuthenticatedProfile();
    await requirePostOfficeOperation(snapshot, "moveMessage", {
      pathParams: { messageId: normalizeString(messageId) },
      body: {
        targetFolder: normalizePostOfficeFolder(targetFolder, POST_OFFICE_DEFAULT_FOLDER),
      },
    });
    return { success: true };
  },


  async updateMessageReadState(messageId, options = {}) {
    const { snapshot } = await resolveAuthenticatedProfile();
    await requirePostOfficeOperation(snapshot, "updateMessageReadState", {
      pathParams: { messageId: normalizeString(messageId) },
      body: {
        isUnread: typeof options.isUnread === "boolean" ? options.isUnread : false,
      },
    });
    return { success: true };
  },


  async listContacts({ search }) {
    await resolveAuthenticatedProfile();
    const contactsPayload = await mailHttp.get("/contacts", {
      query: { search: normalizeString(search) },
      headers: buildBffAuthHeaders(),
    });
    return {
      items: normalizeContactItems(contactsPayload),
    };
  },


  async getContactThread(contactId) {
    const { snapshot } = await resolveAuthenticatedProfile();
    if (!isBffSnapshot(snapshot)) {
      return null;
    }
    const payload = await mailHttp.get(`/contacts/${encodeURIComponent(normalizeString(contactId))}/thread`, {
      headers: buildBffAuthHeaders(),
    });
    return normalizeContactThreadPayload(payload);
  },

  async getSettings() {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const remoteSettings = await requirePostOfficeOperation(snapshot, "getSettings");
    return normalizeContractSettings(remoteSettings?.settings, profile);
  },


  async updateSettings(patch) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const nextSettingsPayload = await requirePostOfficeOperation(snapshot, "updateSettings", {
      body: pickPostOfficeSettingsPatch(patch),
    });
    return normalizeContractSettings(nextSettingsPayload?.settings, profile);
  },


  async listSecuritySessions() {
    const payload = await mailHttp.get("/security/sessions", {
      headers: buildBffAuthHeaders(),
    });
    return normalizeSecuritySessionsPayload(payload);
  },

  async logoutOtherSecuritySessions() {
    const payload = await mailHttp.post("/security/sessions/logout-others", {
      headers: buildBffAuthHeaders(),
    });
    return normalizeSecuritySessionsPayload(payload);
  },

  async revokeSecuritySession(sessionId) {
    const normalizedSessionId = encodeURIComponent(normalizeString(sessionId));
    if (!normalizedSessionId) {
      throw new Error("登录设备不存在或已失效");
    }
    const payload = await mailHttp.post(`/security/sessions/${normalizedSessionId}/revoke`, {
      headers: buildBffAuthHeaders(),
    });
    return normalizeSecuritySessionsPayload(payload);
  },

  async sendMessage(payload) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const recipients = normalizeArray(payload?.recipients).map((item) => normalizeString(item)).filter(Boolean);
    const subject = normalizeString(payload?.subject) || "未命名邮件";
    const textBody = normalizeString(payload?.body);
    const htmlBody = normalizeString(payload?.html);
    const attachments = normalizeArray(payload?.attachments).map(normalizeAttachmentPayload);

    const remoteMessage = await requirePostOfficeOperation(snapshot, "sendMessage", {
      body: {
        recipients,
        cc: [],
        bcc: [],
        subject,
        body: {
          format: htmlBody ? (textBody ? "both" : "html") : "text",
          text: textBody || null,
          html: htmlBody || null,
        },
        attachments,
        templateId: null,
        replyToMessageId: null,
        source: "manual",
      },
    });

    if (!remoteMessage?.message) {
      throw new Error("BFF 发信接口未返回邮件结果，已阻止前端补写成功");
    }

    return normalizeContractMessage(remoteMessage.message, {
      sender: profile.displayName,
      senderEmail: profile.email,
      recipients,
      subject,
      content: htmlBody || `<p class="text-slate-700 leading-relaxed whitespace-pre-wrap">${escapeHtml(textBody || subject)}</p>`,
      attachments,
      isOutgoing: true,
      source: "mailbox",
      deliveryStatus: "accepted",
      folder: "sent",
      role: resolveSelfRoleTitle(profile.authMode),
    });
  },


  async uploadAttachments(attachments) {
    const normalizedAttachments = normalizeArray(attachments).map(normalizeAttachmentPayload);
    if (!normalizedAttachments.length) {
      return [];
    }
    await resolveAuthenticatedProfile();
    const result = await mailHttp.post("/attachments", {
      body: { attachments: normalizedAttachments },
      headers: buildBffAuthHeaders(),
    });
    const data = extractEnvelopeData(result) || result || {};
    return normalizeArray(data.items).map(normalizeAttachmentPayload);
  },


  async saveDraft(payload) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const recipients = normalizeArray(payload?.recipients).map((item) => normalizeString(item)).filter(Boolean);
    const subject = normalizeString(payload?.subject) || "未命名草稿";
    const textBody = normalizeString(payload?.body);
    const attachments = normalizeArray(payload?.attachments).map(normalizeAttachmentPayload);

    const remoteDraft = await requirePostOfficeOperation(snapshot, "createDraft", {
      body: {
        draftId: null,
        recipients,
        cc: [],
        bcc: [],
        subject,
        body: {
          format: "text",
          text: textBody || null,
          html: null,
        },
        attachments,
        autosave: false,
      },
    });

    if (!remoteDraft?.draft) {
      throw new Error("BFF 草稿接口未返回草稿结果，已阻止前端补写成功");
    }

    return normalizeContractMessage(remoteDraft.draft, {
      sender: profile.displayName,
      senderEmail: profile.email,
      recipients,
      subject,
      attachments,
      isOutgoing: true,
      source: "draft",
      deliveryStatus: "draft",
      folder: "drafts",
      role: resolveSelfRoleTitle(profile.authMode),
    });
  },


  async sendInvite(payload) {
    await resolveAuthenticatedProfile();
    const result = await mailHttp.post("/templates/send", {
      body: {
        role: normalizeString(payload?.role || "user"),
        recipients: normalizeArray(payload?.recipients),
      },
      headers: buildBffAuthHeaders(),
    });
    const data = extractEnvelopeData(result) || result || {};
    return {
      recorded: true,
      message: data.message,
      recipientCount: Number(data.recipientCount || normalizeArray(payload?.recipients).length),
      role: normalizeString(data.role || payload?.role || "user"),
    };
  },

	  async getAdminAuthSession() {
	    try {
	      const session = await mailHttp.get("/admin/auth/session", {
	        headers: buildBffAdminHeaders(),
	      });
      if (session?.isAuthenticated === false) {
        clearAdminSession();
	      }
	      return session;
	    } catch (error) {
	      if (error?.status === 401) {
	        clearAdminSession();
	        return { authRequired: true, isAuthenticated: false, username: "admin" };
	      }
      throw error;
    }
  },

  async loginAdmin(payload) {
    const session = await mailHttp.post("/admin/auth/login", {
      body: payload || {},
      headers: buildBffAuthHeaders(),
    });
    persistAdminSession(session);
    return session;
  },

  async logoutAdmin() {
    try {
      await mailHttp.post("/admin/auth/logout", {
        body: {},
        headers: buildBffAdminHeaders(),
      });
    } finally {
      clearAdminSession();
    }
    return { success: true };
  },

	  async getAdminConfig() {
	    try {
	      return await mailHttp.get("/admin/mail/config", {
	        headers: buildBffAdminHeaders(),
	      });
    } catch (error) {
	      if (error?.status === 401) {
	        clearAdminSession();
	      }
	      throw error;
	    }
	  },

  async updateAdminConfig(patch) {
    return mailHttp.patch("/admin/mail/config", {
      body: patch || {},
      headers: buildBffAdminHeaders(),
    });
  },

  async createMailboxInvite(payload) {
    return mailHttp.post("/admin/mail/invites", {
      body: payload || {},
      headers: buildBffAdminHeaders(),
    });
  },

  async runOperationalTasks() {
    return mailHttp.post("/admin/mail/ops/run", {
      body: {},
      headers: buildBffAdminHeaders(),
    });
  },

  async verifyMailboxDomain() {
    return mailHttp.post("/admin/mail/domains/verify", {
      body: {},
      headers: buildBffAdminHeaders(),
    });
  },

  async testMailServer() {
    return mailHttp.post("/admin/mail/server/test", {
      body: {},
      headers: buildBffAdminHeaders(),
    });
  },

  async disableMailboxAccount(accountId) {
    return mailHttp.post(`/admin/mail/accounts/${encodeURIComponent(accountId)}/disable`, {
      body: {},
      headers: buildBffAdminHeaders(),
    });
  },

  async enableMailboxAccount(accountId) {
    return mailHttp.post(`/admin/mail/accounts/${encodeURIComponent(accountId)}/enable`, {
      body: {},
      headers: buildBffAdminHeaders(),
    });
  },

  async resetMailboxAccountPassword(accountId) {
    return mailHttp.post(`/admin/mail/accounts/${encodeURIComponent(accountId)}/reset-password`, {
      body: {},
      headers: buildBffAdminHeaders(),
    });
  },

  async retryMailboxProvision(accountId) {
    return mailHttp.post(`/admin/mail/accounts/${encodeURIComponent(accountId)}/provision`, {
      body: {},
      headers: buildBffAdminHeaders(),
    });
  },

  async sendAuthSmsCode(payload) {
    return mailHttp.post("/auth/sms/send", {
      body: payload || {},
      headers: buildBffAuthHeaders(),
    });
  },

  async registerWithPassword(payload) {
    const session = await mailHttp.post("/auth/register", {
      body: payload || {},
      headers: buildBffAuthHeaders(),
    });
    persistBffSession(session);
    return session;
  },

  async loginWithPassword(payload) {
    const session = await mailHttp.post("/auth/login", {
      body: payload || {},
      headers: buildBffAuthHeaders(),
    });
    persistBffSession(session);
    return session;
  },
};
