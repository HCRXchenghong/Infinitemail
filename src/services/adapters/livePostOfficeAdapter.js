import { extractAuthSessionResult, extractEnvelopeData, extractPaginatedItems } from "@infinitech/contracts/http";
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
  readLivePostOfficeState,
  readStoredUnifiedSession,
  updateLivePostOfficeState,
} from "../live/postOfficeLiveState";

const platformHttp = createHttpClient({
  baseUrl: runtimeConfig.platformApiBaseUrl,
  timeoutMs: runtimeConfig.requestTimeoutMs,
});

const mailHttp = createHttpClient({
  baseUrl: runtimeConfig.mailApiBaseUrl,
  timeoutMs: runtimeConfig.requestTimeoutMs,
});

const MAIL_API_UNAVAILABLE_STATUSES = new Set([404, 405, 501, 502, 503, 504]);

const inviteCardCopy = Object.freeze({
  merchant: {
    title: "诚邀入驻悦享e食",
    subtitle: "开启您的线上餐厅新增长",
    salutation: "尊敬的商户伙伴：",
    body: "我们诚挚地邀请您加入悦享e食生态。在这里，您将获得全城活跃用户的流量支持，以及专业的数字化经营工具。",
    bullets: ["首月免佣金福利", "一对一运营指导", "专属骑手运力保障"],
    actionLabel: "点击立即入驻",
  },
  rider: {
    title: "诚邀加入悦享骑手团队",
    subtitle: "更稳定的收入，更灵活的配送协作",
    salutation: "尊敬的骑手伙伴：",
    body: "我们期待您加入悦享运力生态，获得更透明的结算体验、稳定的配送单量和完善的培训支持。",
    bullets: ["新人扶持奖励", "站点培训与保险保障", "灵活排班与就近接单"],
    actionLabel: "点击立即加入",
  },
  user: {
    title: "悦享e食核心用户内测邀请",
    subtitle: "抢先体验新功能与生态权益",
    salutation: "尊敬的体验官：",
    body: "诚邀您加入悦享e食核心用户内测计划，提前体验平台新能力，并获得专属反馈奖励和邀请权益。",
    bullets: ["新功能抢先体验", "专属福利与邀请码", "产品团队直连反馈"],
    actionLabel: "点击立即参与",
  },
});

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

function maskPhone(phone) {
  const normalized = normalizeString(phone);
  if (!/^1\d{10}$/.test(normalized)) {
    return normalized;
  }
  return `${normalized.slice(0, 3)}****${normalized.slice(-4)}`;
}

function resolveAuthMode(candidate, fallback = "user") {
  const normalized = normalizeString(candidate).toLowerCase();
  if (normalized === "merchant" || normalized === "rider" || normalized === "user") {
    return normalized;
  }
  return fallback;
}

function resolveRolePrefix(authMode) {
  switch (resolveAuthMode(authMode)) {
    case "merchant":
      return "merchant-";
    case "rider":
      return "rider-";
    default:
      return "user-";
  }
}

function resolveSelfRoleTitle(authMode) {
  switch (resolveAuthMode(authMode)) {
    case "merchant":
      return "悦享商户";
    case "rider":
      return "悦享骑手";
    default:
      return "悦享用户";
  }
}

function resolveRoleLabel(role) {
  switch (normalizeString(role).toLowerCase()) {
    case "merchant":
      return "外部商户";
    case "rider":
      return "骑手伙伴";
    case "user":
      return "生态用户";
    default:
      return "平台官方";
  }
}

function resolveOrganization(role) {
  switch (normalizeString(role).toLowerCase()) {
    case "merchant":
      return "悦享生态合作商户";
    case "rider":
      return "悦享运力协作网络";
    case "user":
      return "悦享生态用户网络";
    default:
      return "悦享e食平台";
  }
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
    signature: "--\n此致\n悦享e食 合作伙伴",
    autoReplyEnabled: false,
    autoReplyMessage: "您好，我暂时无法及时回复，稍后会第一时间处理您的来信。",
    updatedAt: null,
  };
}

function resolveAccountId(profile) {
  return normalizeString(profile?.id || profile?.uid || profile?.phone || profile?.role_id);
}

function resolveCurrentUserLookupId(snapshot = {}) {
  const storedProfile = normalizePlainObject(snapshot.userProfile);
  return normalizeString(storedProfile.id || storedProfile.uid || storedProfile.phone || storedProfile.userId);
}

function resolveRemainingExpiresIn(tokenExpiresAt) {
  const expiresAt = Number(tokenExpiresAt || 0);
  if (!Number.isFinite(expiresAt) || expiresAt <= 0) {
    return 0;
  }
  return Math.max(0, Math.floor((expiresAt - Date.now()) / 1000));
}

function resolveSsoBridge() {
  if (typeof window === "undefined") {
    return null;
  }

  const candidates = [
    runtimeConfig.ssoBridgeName,
    "__YUEXIANG_POST_OFFICE_SSO__",
    "__YUEXIANG_EFOOD_SSO__",
    "__YUEXIANG_SSO__",
  ]
    .map((name) => normalizeString(name))
    .filter(Boolean);

  for (const name of candidates) {
    const bridge = window[name];
    if (bridge && typeof bridge === "object") {
      return bridge;
    }
  }

  return null;
}

function buildSsoEntryUrl() {
  const configured = normalizeString(runtimeConfig.ssoEntryUrl);
  if (!configured || typeof window === "undefined") {
    return "";
  }

  try {
    const target = new URL(configured, window.location.origin);
    if (!target.searchParams.has("redirect")) {
      target.searchParams.set("redirect", window.location.href);
    }
    return target.toString();
  } catch (_error) {
    return configured;
  }
}

function normalizeBridgeAuthMode(source, fallback = "user") {
  return resolveAuthMode(
    source?.authMode ||
      source?.principalType ||
      source?.role ||
      source?.type ||
      source?.user?.principalType ||
      source?.user?.role,
    fallback,
  );
}

function buildConversationMailId(rawId) {
  return `conv:${normalizeString(rawId)}`;
}

function getConversationMeta(state, accountId) {
  const current = normalizePlainObject(state.conversationMetaByAccountId?.[accountId]);
  return {
    starredIds: normalizeArray(current.starredIds),
    folderByMessageId: normalizePlainObject(current.folderByMessageId),
  };
}

function getLocalMessages(state, accountId) {
  return normalizeArray(state.localMessagesByAccountId?.[accountId]).map((item) => ({
    ...item,
    tags: normalizeArray(item.tags),
    recipients: normalizeArray(item.recipients),
    attachments: normalizeArray(item.attachments),
  }));
}

function getMailboxState(state, accountId) {
  return normalizePlainObject(state.mailboxByAccountId?.[accountId]);
}

function getSettingsState(state, accountId, profile) {
  return {
    ...buildDefaultSettings(profile),
    ...normalizePlainObject(state.settingsByAccountId?.[accountId]),
  };
}

function buildPostOfficeProfile(userPayload, snapshot, liveState) {
  const user = normalizePlainObject(userPayload);
  const storedProfile = normalizePlainObject(snapshot.userProfile);
  const authMode = resolveAuthMode(snapshot.authMode || storedProfile.authMode || normalizeBridgeAuthMode(user));
  const accountId = normalizeString(user.id || storedProfile.id || user.phone || storedProfile.phone);
  const mailboxState = getMailboxState(liveState, accountId);
  const displayName = normalizeString(user.nickname || user.name || storedProfile.nickname || storedProfile.name || "悦享用户");
  const rolePrefix = resolveRolePrefix(authMode);
  const mailboxDomain = resolveMailboxDomain();

  return {
    id: accountId,
    displayName,
    avatarInitial: deriveAvatarText(user.avatarText || displayName, "Y"),
    unifiedAccountPhone: maskPhone(user.phone || storedProfile.phone),
    rolePrefix,
    mailboxDomain,
    mailboxProvisioned: Boolean(mailboxState.email),
    emailPrefix: normalizeString(mailboxState.emailPrefix),
    email: normalizeString(mailboxState.email),
    authMode,
    sourceUserId: accountId,
    provisioningStatus: mailboxState.email ? "active" : "pending",
    createdAt: null,
    updatedAt: null,
    provisionedAt: normalizeString(mailboxState.provisionedAt) || null,
    sourceUser: user,
  };
}

function persistLatestProfile(snapshot, userPayload) {
  const user = normalizePlainObject(userPayload);
  const storedProfile = normalizePlainObject(snapshot.userProfile);
  const mergedProfile = {
    ...storedProfile,
    ...user,
    id: normalizeString(user.id || storedProfile.id),
    phone: normalizeString(user.phone || storedProfile.phone),
    nickname: normalizeString(user.nickname || user.name || storedProfile.nickname),
    name: normalizeString(user.name || storedProfile.name || user.nickname),
    avatarUrl: normalizeString(user.avatarUrl || storedProfile.avatarUrl),
    authMode: resolveAuthMode(snapshot.authMode || storedProfile.authMode || "user"),
  };

  persistUnifiedSession({
    token: snapshot.token,
    refreshToken: snapshot.refreshToken,
    expiresIn: resolveRemainingExpiresIn(snapshot.tokenExpiresAt),
    authMode: mergedProfile.authMode,
    userProfile: mergedProfile,
  });

  return mergedProfile;
}

function buildConversationSubject(conversation) {
  const preview = normalizeString(conversation.lastMessage || conversation.msg || "");
  if (!preview || preview === "[暂无消息]") {
    return `与 ${normalizeString(conversation.name || "生态联系人")} 的生态会话`;
  }
  return preview.length > 26 ? `${preview.slice(0, 26)}...` : preview;
}

function resolveConversationAddress(conversation) {
  return normalizeString(conversation.phone || conversation.targetId || conversation.chatId || conversation.id);
}

function buildConversationHtml(conversation) {
  const roleLabel = resolveRoleLabel(conversation.role);
  const organization = resolveOrganization(conversation.role);
  const contactAddress = resolveConversationAddress(conversation);
  const preview = normalizeString(conversation.lastMessage || conversation.msg || "[暂无消息]");

  return `
    <div class="border border-slate-200 rounded-xl overflow-hidden bg-white">
      <div class="h-2 bg-[#009BF5] w-full"></div>
      <div class="p-8">
        <div class="flex items-center gap-3 mb-6">
          <div class="w-12 h-12 bg-[#E5F5FF] text-[#009BF5] rounded-full flex items-center justify-center font-bold text-lg">
            ${escapeHtml(deriveAvatarText(conversation.avatar || conversation.name, "Y"))}
          </div>
          <div>
            <h2 class="text-xl font-bold text-slate-900">${escapeHtml(normalizeString(conversation.name || "生态联系人"))}</h2>
            <p class="text-sm text-slate-500">${escapeHtml(roleLabel)} · ${escapeHtml(organization)}</p>
          </div>
        </div>
        <div class="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600 mb-6">
          联系标识：${escapeHtml(contactAddress || "未提供")}
        </div>
        <div class="text-slate-700 leading-relaxed whitespace-pre-wrap">${escapeHtml(preview)}</div>
        <p class="text-xs text-slate-400 mt-8 pt-4 border-t border-slate-100">该内容来自悦享e食现有消息会话数据源，当前作为邮局联调期收件箱映射展示。</p>
      </div>
    </div>
  `;
}

function buildConversationMail(conversation, profile, state) {
  const accountId = resolveAccountId(profile);
  const meta = getConversationMeta(state, accountId);
  const mailId = buildConversationMailId(conversation.chatId || conversation.roomId || conversation.id);
  const folder = normalizeString(meta.folderByMessageId[mailId] || "inbox");
  const timestamp = Number(conversation.updatedAt || 0) || Date.now();

  return {
    id: mailId,
    threadId: normalizeString(conversation.chatId || conversation.roomId || conversation.id) || null,
    folder,
    previousFolder: "inbox",
    sender: normalizeString(conversation.name || "生态联系人"),
    senderEmail: resolveConversationAddress(conversation),
    recipients: profile.email ? [profile.email] : [],
    cc: [],
    bcc: [],
    avatar: deriveAvatarText(conversation.avatar || conversation.name, "Y"),
    role: resolveRoleLabel(conversation.role),
    subject: buildConversationSubject(conversation),
    snippet: normalizeString(conversation.lastMessage || conversation.msg || "[暂无消息]"),
    time: normalizeString(conversation.time) || formatListTime(timestamp),
    dateTimeLabel: formatDateTimeLabel(timestamp),
    sortAt: new Date(timestamp).toISOString(),
    sentAt: null,
    receivedAt: new Date(timestamp).toISOString(),
    isUnread: Number(conversation.unread || 0) > 0,
    isStarred: meta.starredIds.includes(mailId),
    hasAttachment: false,
    tags: ["生态会话"],
    isOutgoing: false,
    content: buildConversationHtml(conversation),
    attachments: [],
    source: "imported",
    deliveryStatus: "received",
    replyToMessageId: null,
    meta: {
      conversationId: normalizeString(conversation.chatId || conversation.roomId || conversation.id),
    },
    conversation,
  };
}

function buildDefaultInviteHtml(role, inviteCode, profile) {
  const copy = inviteCardCopy[role] || inviteCardCopy.merchant;
  const inviteCodeText = normalizeString(inviteCode) || "待生成";
  const senderName = escapeHtml(profile.displayName);

  return `
    <div class="border border-slate-200 rounded-xl overflow-hidden bg-white">
      <div class="h-2 bg-[#009BF5] w-full"></div>
      <div class="p-8">
        <div class="text-2xl font-bold text-slate-900 mb-2">${escapeHtml(copy.title)}</div>
        <div class="text-slate-500 mb-8">${escapeHtml(copy.subtitle)}</div>
        <div class="space-y-4 text-slate-700 text-sm leading-relaxed mb-8">
          <p>${escapeHtml(copy.salutation)}</p>
          <p>${escapeHtml(copy.body)}</p>
          <ul class="list-disc pl-5 space-y-1 text-slate-600">
            ${copy.bullets.map((item) => `<li>${escapeHtml(item)}</li>`).join("")}
          </ul>
        </div>
        <div class="rounded-lg border border-[#B3E0FF] bg-[#E5F5FF] text-[#007ACC] px-4 py-3 text-sm mb-8">
          邀请码：<span class="font-semibold">${escapeHtml(inviteCodeText)}</span>
        </div>
        <div class="text-center">
          <a href="#" class="inline-block bg-[#009BF5] text-white px-8 py-3 rounded-full font-medium">${escapeHtml(copy.actionLabel)}</a>
        </div>
      </div>
      <div class="bg-slate-50 p-4 text-center text-xs text-slate-400 border-t border-slate-100">
        此邮件由悦享邮局自动生成 · 发起人 ${senderName}
      </div>
    </div>
  `;
}

function buildInviteTemplates(inviteCode, profile) {
  return Object.keys(inviteCardCopy).map((role) => ({
    id: `invite-${role}`,
    role,
    subject: inviteCardCopy[role].title,
    html: buildDefaultInviteHtml(role, inviteCode, profile),
  }));
}

function buildInviteSentMessage({ role, recipients, inviteCode, profile, template }) {
  const timestamp = Date.now();
  const inviteTargetTag = resolveInviteContactTag(role);
  return {
    id: nextLocalMessageId("invite"),
    threadId: null,
    folder: "sent",
    previousFolder: "sent",
    sender: profile.displayName,
    senderEmail: profile.email,
    recipients,
    cc: [],
    bcc: [],
    avatar: profile.avatarInitial,
    role: resolveSelfRoleTitle(profile.authMode),
    subject: normalizeString(template?.subject || inviteCardCopy[role]?.title || "业务邀请函"),
    snippet: `邀请已发出，共 ${recipients.length} 个目标地址。邀请码 ${inviteCode || "待生成"}`,
    time: formatListTime(timestamp),
    dateTimeLabel: formatDateTimeLabel(timestamp),
    sortAt: new Date(timestamp).toISOString(),
    sentAt: new Date(timestamp).toISOString(),
    receivedAt: null,
    isUnread: false,
    isStarred: false,
    hasAttachment: false,
    tags: dedupeStrings(["业务邀请", "已邀请", inviteTargetTag]),
    isOutgoing: true,
    content: normalizeString(template?.html || buildDefaultInviteHtml(role, inviteCode, profile)),
    attachments: [],
    source: "invite",
    deliveryStatus: "sent",
    replyToMessageId: null,
    meta: {
      inviteRole: role,
    },
  };
}

function buildLocalComposeMessage({ folder, recipients, subject, body, profile }) {
  const timestamp = Date.now();
  return {
    id: nextLocalMessageId(folder === "drafts" ? "draft" : "mail"),
    threadId: null,
    folder,
    previousFolder: folder,
    sender: profile.displayName,
    senderEmail: profile.email,
    recipients,
    cc: [],
    bcc: [],
    avatar: profile.avatarInitial,
    role: resolveSelfRoleTitle(profile.authMode),
    subject: normalizeString(subject) || (folder === "drafts" ? "未命名草稿" : "未命名邮件"),
    snippet: normalizeString(body || subject).slice(0, 48),
    time: formatListTime(timestamp),
    dateTimeLabel: formatDateTimeLabel(timestamp),
    sortAt: new Date(timestamp).toISOString(),
    sentAt: folder === "drafts" ? null : new Date(timestamp).toISOString(),
    receivedAt: null,
    isUnread: false,
    isStarred: false,
    hasAttachment: false,
    tags: [folder === "drafts" ? "草稿" : "已发送"],
    isOutgoing: true,
    content: `<p class="text-slate-700 leading-relaxed whitespace-pre-wrap">${escapeHtml(body || subject)}</p>`,
    attachments: [],
    source: folder === "drafts" ? "draft" : "mailbox",
    deliveryStatus: folder === "drafts" ? "draft" : "sent",
    replyToMessageId: null,
    meta: {},
  };
}

function matchesFilter(message, filter) {
  if (filter === "unread") {
    return message.isUnread;
  }
  if (filter === "important") {
    return message.isStarred;
  }
  if (filter === "attachment") {
    return message.hasAttachment;
  }
  return true;
}

function matchesFolder(message, folderId) {
  if (folderId === "starred") {
    return message.isStarred && message.folder !== "trash" && message.folder !== "archive";
  }
  return message.folder === folderId;
}

function matchesSearch(message, search) {
  const keyword = normalizeString(search).toLowerCase();
  if (!keyword) {
    return true;
  }

  return [
    message.sender,
    message.senderEmail,
    message.subject,
    message.snippet,
    ...(message.tags || []),
  ]
    .join(" ")
    .toLowerCase()
    .includes(keyword);
}

function sortMessages(messages) {
  return [...messages].sort((left, right) => new Date(right.sortAt).getTime() - new Date(left.sortAt).getTime());
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

function buildListPayload(messages, folderId, filter = "all", search = "") {
  const items = sortMessages(
    messages.filter((message) => matchesFolder(message, folderId) && matchesFilter(message, filter) && matchesSearch(message, search)),
  );

  return {
    items,
    folderCounts: deriveFolderCounts(messages),
    nextCursor: null,
    hasMore: false,
  };
}

function buildContactsFromConversations(conversations) {
  return conversations.map((conversation) => {
    const timestamp = Number(conversation.updatedAt || 0) || Date.now();
    return {
      id: normalizeString(conversation.chatId || conversation.roomId || conversation.id),
      name: normalizeString(conversation.name || "生态联系人"),
      email: resolveConversationAddress(conversation),
      avatar: deriveAvatarText(conversation.avatar || conversation.name, "Y"),
      role: resolveRoleLabel(conversation.role),
      organization: resolveOrganization(conversation.role),
      lastContactedAt: formatDateTimeLabel(timestamp),
      note: normalizeString(conversation.lastMessage || conversation.msg || "暂无最近会话摘要"),
    };
  });
}

function deriveContactDisplayName(email) {
  const normalized = normalizeString(email);
  if (!normalized.includes("@")) {
    return normalized || "生态联系人";
  }

  const [localPart] = normalized.split("@");
  const words = localPart
    .split(/[._-]+/)
    .map((item) => item.trim())
    .filter(Boolean);

  if (words.length === 0) {
    return normalized;
  }

  return words
    .map((item) => item.slice(0, 1).toUpperCase() + item.slice(1))
    .join(" ");
}

function inferContactRoleFromOutgoingMessage(message) {
  const inviteRole = normalizeString(message?.meta?.inviteRole).toLowerCase();
  return resolveRoleLabel(inviteRole || "user");
}

function dedupeStrings(items) {
  return Array.from(
    new Set(
      normalizeArray(items)
        .map((item) => normalizeString(item))
        .filter(Boolean),
    ),
  );
}

function createEmptyContactStats() {
  return {
    totalMessages: 0,
    incomingCount: 0,
    outgoingCount: 0,
    inviteCount: 0,
    draftCount: 0,
  };
}

function normalizeContactStats(stats) {
  const next = normalizePlainObject(stats);
  return {
    totalMessages: Number(next.totalMessages || 0),
    incomingCount: Number(next.incomingCount || 0),
    outgoingCount: Number(next.outgoingCount || 0),
    inviteCount: Number(next.inviteCount || 0),
    draftCount: Number(next.draftCount || 0),
  };
}

function mergeContactStats(current, patch) {
  const base = normalizeContactStats(current);
  const delta = normalizeContactStats(patch);
  return {
    totalMessages: base.totalMessages + delta.totalMessages,
    incomingCount: base.incomingCount + delta.incomingCount,
    outgoingCount: base.outgoingCount + delta.outgoingCount,
    inviteCount: base.inviteCount + delta.inviteCount,
    draftCount: base.draftCount + delta.draftCount,
  };
}

function resolveInviteContactTag(role) {
  switch (normalizeString(role).toLowerCase()) {
    case "merchant":
      return "商户邀请";
    case "rider":
      return "骑手招募";
    case "user":
      return "用户内测";
    default:
      return "";
  }
}

function buildContactTagsFromMessage(message) {
  const tags = normalizeArray(message?.tags).filter((tag) => normalizeString(tag) && normalizeString(tag) !== "已发送");
  const inviteTag = resolveInviteContactTag(message?.meta?.inviteRole);

  if (message?.source === "invite" || tags.includes("业务邀请")) {
    tags.push("已邀请");
  }
  if (inviteTag) {
    tags.push(inviteTag);
  }
  if (message?.folder === "drafts") {
    tags.push("待发送");
  }
  if (message?.hasAttachment) {
    tags.push("含附件");
  }

  return dedupeStrings(tags);
}

function resolveContactOrganizationFromMessage(message) {
  if (message?.isOutgoing && message?.source === "invite") {
    return "来自业务邀请";
  }
  if (message?.isOutgoing && message?.folder === "drafts") {
    return "来自待发送草稿";
  }
  if (message?.isOutgoing) {
    return "来自已发送邮件";
  }
  return "来自收件箱往来";
}

function sortContacts(contacts) {
  return [...normalizeArray(contacts)].sort(
    (left, right) =>
      new Date(right?.lastSortAt || 0).getTime() - new Date(left?.lastSortAt || 0).getTime(),
  );
}

function buildContactsFromMessages(messages, profile) {
  const contactsByEmail = new Map();
  const selfEmail = normalizeString(profile?.email).toLowerCase();

  const upsert = (contact, statsPatch = createEmptyContactStats()) => {
    const email = normalizeString(contact?.email).toLowerCase();
    if (!email || email === selfEmail) {
      return;
    }

    const current = contactsByEmail.get(email);
    const tags = dedupeStrings([...(current?.tags || []), ...normalizeArray(contact?.tags)]);
    const stats = mergeContactStats(current?.stats, statsPatch);
    contactsByEmail.set(email, {
      id: normalizeString(contact?.id || current?.id || email),
      name: normalizeString(contact?.name || current?.name || deriveContactDisplayName(email) || "生态联系人"),
      email,
      avatar: deriveAvatarText(contact?.avatar || current?.avatar || current?.name || email, "Y"),
      role: normalizeString(contact?.role || current?.role || "生态联系人"),
      organization: normalizeString(contact?.organization || current?.organization || "悦享生态邮件往来"),
      lastContactedAt: normalizeString(contact?.lastContactedAt || current?.lastContactedAt || ""),
      note: normalizeString(contact?.note || current?.note || "来自邮件往来记录"),
      tags,
      stats,
      lastSortAt: normalizeString(contact?.lastSortAt || current?.lastSortAt || ""),
    });
  };

  for (const message of sortMessages(messages)) {
    const messageTags = buildContactTagsFromMessage(message);
    const statsPatch = {
      totalMessages: 1,
      incomingCount: message.isOutgoing ? 0 : 1,
      outgoingCount: message.isOutgoing ? 1 : 0,
      inviteCount: message.source === "invite" ? 1 : 0,
      draftCount: message.folder === "drafts" ? 1 : 0,
    };

    if (message.isOutgoing) {
      const outgoingRole = inferContactRoleFromOutgoingMessage(message);
      for (const recipient of normalizeArray(message.recipients)) {
        upsert({
          id: `mail:${recipient}`,
          name: deriveContactDisplayName(recipient),
          email: recipient,
          avatar: deriveContactDisplayName(recipient),
          role: outgoingRole,
          organization: resolveContactOrganizationFromMessage(message),
          lastContactedAt: message.dateTimeLabel,
          note: normalizeString(message.subject || message.snippet || "最近一次主动联系"),
          tags: messageTags,
          lastSortAt: message.sortAt,
        }, statsPatch);
      }
      continue;
    }

    upsert({
      id: `mail:${message.senderEmail}`,
      name: message.sender,
      email: message.senderEmail,
      avatar: message.avatar || message.sender,
      role: message.role,
      organization: resolveContactOrganizationFromMessage(message),
      lastContactedAt: message.dateTimeLabel,
      note: normalizeString(message.subject || message.snippet || "最近一次来信"),
      tags: messageTags,
      lastSortAt: message.sortAt,
    }, statsPatch);
  }

  return sortContacts(Array.from(contactsByEmail.values()));
}

function buildRoleSeedContacts(profile) {
  const mailboxDomain = resolveMailboxDomain();

  switch (resolveAuthMode(profile?.authMode)) {
    case "merchant":
      return [
        {
          id: "seed-merchant-ops",
          name: "商户成长顾问",
          email: `merchant.success@${mailboxDomain}`,
          avatar: "商",
          role: "平台官方",
          organization: "悦享商户成长中心",
          lastContactedAt: "今天",
          note: "提供入驻、经营、活动和流量策略支持",
        },
        {
          id: "seed-rider-ops",
          name: "骑手运营组",
          email: `rider.ops@${mailboxDomain}`,
          avatar: "骑",
          role: "平台官方",
          organization: "悦享运力运营部",
          lastContactedAt: "今天",
          note: "配送运力协同、时效和履约异常处理",
        },
      ];
    case "rider":
      return [
        {
          id: "seed-rider-station",
          name: "站点调度",
          email: `dispatch@${mailboxDomain}`,
          avatar: "站",
          role: "平台官方",
          organization: "悦享站点运营中心",
          lastContactedAt: "今天",
          note: "负责排班、补贴通知和异常订单协助",
        },
        {
          id: "seed-rider-support",
          name: "保险与关怀",
          email: `rider.care@${mailboxDomain}`,
          avatar: "保",
          role: "平台官方",
          organization: "悦享骑手保障中心",
          lastContactedAt: "今天",
          note: "负责保障、培训和骑手权益支持",
        },
      ];
    default:
      return [
        {
          id: "seed-user-design",
          name: "李设计",
          email: `li.design@${mailboxDomain}`,
          avatar: "L",
          role: "内部员工",
          organization: "品牌设计中心",
          lastContactedAt: "昨天",
          note: "负责品牌主视觉与活动物料",
        },
        {
          id: "seed-user-rider-ops",
          name: "骑手运营组",
          email: `rider.ops@${mailboxDomain}`,
          avatar: "R",
          role: "平台官方",
          organization: "运力运营部",
          lastContactedAt: "今天",
          note: "骑手招募、培训和激励政策",
        },
      ];
  }
}

function buildUnifiedContacts({ conversations = [], messages = [], profile }) {
  const contactsByEmail = new Map();

  const pushContacts = (items) => {
    for (const contact of normalizeArray(items)) {
      const email = normalizeString(contact?.email).toLowerCase();
      if (!email || contactsByEmail.has(email)) {
        continue;
      }
      contactsByEmail.set(email, {
        ...contact,
        email,
        tags: dedupeStrings(contact?.tags),
        stats: normalizeContactStats(contact?.stats),
      });
    }
  };

  pushContacts(buildContactsFromMessages(messages, profile));
  pushContacts(buildContactsFromConversations(conversations));
  pushContacts(buildRoleSeedContacts(profile));

  return sortContacts(Array.from(contactsByEmail.values()));
}

function filterContacts(contacts, search) {
  const keyword = normalizeString(search).toLowerCase();
  if (!keyword) {
    return contacts;
  }

  return contacts.filter((contact) =>
    [contact.name, contact.email, contact.role, contact.organization, contact.note, ...normalizeArray(contact.tags)]
      .join(" ")
      .toLowerCase()
      .includes(keyword),
  );
}

async function fetchContactSourceMessages(snapshot, profile) {
  const folderIds = ["inbox", "sent", "drafts"];
  const results = await Promise.all(
    folderIds.map((folderId) =>
      tryPostOfficeOperation(
        snapshot,
        "listMessages",
        {
          query: {
            folderId,
            filter: "all",
            search: "",
          },
        },
        () => null,
      ),
    ),
  );

  const messagesById = new Map();
  results.forEach((payload, index) => {
    const folderId = folderIds[index];
    normalizeArray(payload?.items).forEach((message) => {
      const normalized = normalizeContractMessage(message, {
        folder: folderId,
      });
      messagesById.set(normalized.id, normalized);
    });
  });

  return Array.from(messagesById.values());
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
    sender: normalizeString(source.sender || fallbackSource.sender || "悦享生态联系人"),
    senderEmail: normalizeString(source.senderEmail || fallbackSource.senderEmail || ""),
    recipients: normalizeArray(source.recipients || fallbackSource.recipients).map((item) => normalizeString(item)).filter(Boolean),
    cc: normalizeArray(source.cc || fallbackSource.cc).map((item) => normalizeString(item)).filter(Boolean),
    bcc: normalizeArray(source.bcc || fallbackSource.bcc).map((item) => normalizeString(item)).filter(Boolean),
    avatar: deriveAvatarText(source.avatar || fallbackSource.avatar || source.sender || "Y", "Y"),
    role: normalizeString(source.role || fallbackSource.role || "平台官方"),
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

function normalizeContractProfile(profilePayload, fallbackUser = {}) {
  const profile = normalizePlainObject(profilePayload);
  const user = normalizePlainObject(fallbackUser);
  const authMode = resolveAuthMode(profile.authMode || user.authMode || normalizeBridgeAuthMode(user));
  const displayName = normalizeString(profile.displayName || user.nickname || user.name || "悦享用户");

  return {
    id: normalizeString(profile.id || profile.sourceUserId || user.id || user.phone),
    displayName,
    avatarInitial: deriveAvatarText(profile.avatarInitial || displayName, "Y"),
    unifiedAccountPhone: normalizeString(profile.unifiedAccountPhone || maskPhone(user.phone)),
    rolePrefix: normalizeString(profile.rolePrefix || resolveRolePrefix(authMode)),
    emailPrefix: normalizeString(profile.emailPrefix),
    email: normalizeString(profile.email),
    mailboxDomain: normalizeString(profile.mailboxDomain || resolveMailboxDomain()),
    mailboxProvisioned: Boolean(profile.mailboxProvisioned ?? profile.email),
    provisioningStatus: normalizeString(profile.provisioningStatus || (profile.email ? "active" : "pending")),
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

async function tryPostOfficeOperation(snapshot, operationId, options = {}, fallback) {
  try {
    return await requestPostOfficeOperation(snapshot, operationId, options);
  } catch (error) {
    if (shouldResetSession(error)) {
      clearUnifiedSession();
      throw error;
    }

    if (typeof fallback === "function" && isMailApiUnavailableError(error)) {
      return fallback(error);
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

async function fetchCurrentUser(snapshot) {
  const lookupId = resolveCurrentUserLookupId(snapshot);
  if (!lookupId) {
    throw new Error("未找到悦享e食用户标识，无法加载用户中心资料");
  }

  const payload = await platformHttp.get(`/user/${encodeURIComponent(lookupId)}`, {
    headers: buildAuthHeaders(snapshot.token),
  });

  return extractEnvelopeData(payload) || payload || {};
}

async function fetchInviteCode(snapshot, userPayload) {
  const user = normalizePlainObject(userPayload);
  const payload = await platformHttp.get("/invite/code", {
    query: {
      userId: normalizeString(user.id),
      phone: normalizeString(user.phone),
    },
    headers: buildAuthHeaders(snapshot.token),
  });

  const data = extractEnvelopeData(payload) || payload || {};
  return normalizeString(data.code || data.inviteCode);
}

async function fetchConversations(snapshot) {
  const payload = await platformHttp.get("/messages/conversations", {
    headers: buildAuthHeaders(snapshot.token),
  });

  return extractPaginatedItems(payload, {
    listKeys: ["conversations", "items", "records", "list"],
  }).items;
}

function composeAllMessages({ profile, conversations, state }) {
  const conversationMessages = conversations.map((item) => buildConversationMail(item, profile, state));
  const localMessages = getLocalMessages(state, resolveAccountId(profile));
  return [...conversationMessages, ...localMessages];
}

async function resolveAuthenticatedProfile() {
  const snapshot = readStoredUnifiedSession();
  if (!snapshot.token) {
    throw new Error("未检测到悦享e食登录态");
  }

  const liveState = readLivePostOfficeState();
  let userPayload;
  try {
    userPayload = await fetchCurrentUser(snapshot);
  } catch (error) {
    if (shouldResetSession(error)) {
      clearUnifiedSession();
    }
    throw error;
  }
  const mergedProfile = persistLatestProfile(snapshot, userPayload);
  const nextSnapshot = {
    ...snapshot,
    userProfile: mergedProfile,
    authMode: resolveAuthMode(snapshot.authMode || mergedProfile.authMode || "user"),
  };

  return {
    snapshot: nextSnapshot,
    userPayload,
    profile: buildPostOfficeProfile(userPayload, nextSnapshot, liveState),
    liveState,
  };
}

async function hydrateBridgeSession(mode = "getSession", options = {}) {
  const bridge = resolveSsoBridge();
  if (!bridge) {
    return null;
  }

  const bridgeMethod =
    mode === "beginLogin"
      ? bridge.beginLogin || bridge.login || bridge.startLogin
      : bridge.getSession || bridge.readSession;

  if (typeof bridgeMethod !== "function") {
    return null;
  }

  const rawResult = await bridgeMethod({
    app: "yuexiang-post-office",
    returnUrl: typeof window !== "undefined" ? window.location.href : "",
    ...options,
  });

  if (typeof rawResult === "string" && /^https?:\/\//i.test(rawResult) && typeof window !== "undefined") {
    window.location.assign(rawResult);
    return { redirected: true };
  }

  if (rawResult && typeof rawResult === "object" && normalizeString(rawResult.redirectUrl) && typeof window !== "undefined") {
    window.location.assign(rawResult.redirectUrl);
    return { redirected: true };
  }

  const session = extractAuthSessionResult(rawResult);
  if (!session.authenticated) {
    return null;
  }

  const authMode = normalizeBridgeAuthMode(rawResult);
  const userProfile = {
    ...normalizePlainObject(rawResult?.userProfile),
    ...normalizePlainObject(session.user),
    authMode,
  };

  persistUnifiedSession({
    token: session.token,
    refreshToken: session.refreshToken,
    expiresIn: session.expiresIn || 7200,
    authMode,
    userProfile,
  });

  return {
    redirected: false,
    authenticated: true,
    authMode,
  };
}

function upsertMailboxState(profile, emailPrefix) {
  const accountId = resolveAccountId(profile);
  const normalizedPrefix = normalizeString(emailPrefix).toLowerCase().replace(/[^a-z0-9-]/g, "");
  const email = `${profile.rolePrefix}${normalizedPrefix}@${resolveMailboxDomain()}`;

  updateLivePostOfficeState((state) => {
    state.mailboxByAccountId[accountId] = {
      emailPrefix: normalizedPrefix,
      email,
      provisionedAt: new Date().toISOString(),
    };

    const currentSettings = normalizePlainObject(state.settingsByAccountId[accountId]);
    state.settingsByAccountId[accountId] = {
      ...buildDefaultSettings(profile),
      ...currentSettings,
      defaultSenderName:
        currentSettings.defaultSenderName ||
        `${resolveSelfRoleTitle(profile.authMode)}_${profile.displayName}`,
    };

    return state;
  });
}

function persistSettingsMirror(profile, settings) {
  const accountId = resolveAccountId(profile);
  updateLivePostOfficeState((state) => {
    state.settingsByAccountId[accountId] = {
      ...getSettingsState(state, accountId, profile),
      ...pickPostOfficeSettingsPatch(settings),
      updatedAt: normalizeString(settings?.updatedAt) || state.settingsByAccountId?.[accountId]?.updatedAt || null,
    };
    return state;
  });
}

function appendLocalMessage(profile, message) {
  const accountId = resolveAccountId(profile);

  updateLivePostOfficeState((state) => {
    const currentMessages = getLocalMessages(state, accountId);
    state.localMessagesByAccountId[accountId] = [message, ...currentMessages];
    return state;
  });

  return message;
}

function mutateConversationMeta(profile, updater) {
  const accountId = resolveAccountId(profile);

  updateLivePostOfficeState((state) => {
    const current = getConversationMeta(state, accountId);
    const draft = {
      starredIds: [...current.starredIds],
      folderByMessageId: { ...current.folderByMessageId },
    };
    const next = updater(draft) || draft;
    state.conversationMetaByAccountId[accountId] = next;
    return state;
  });
}

export const livePostOfficeAdapter = {
  async getSession() {
    const snapshot = readStoredUnifiedSession();
    const baseRolePrefix = resolveRolePrefix(snapshot.authMode || "user");

    try {
      if (!snapshot.token) {
        const bridgeResult = await hydrateBridgeSession("getSession");
        if (!bridgeResult?.authenticated) {
          return {
            isAuthenticated: false,
            requiresActivation: true,
            rolePrefix: baseRolePrefix,
          };
        }
      }

      const { profile } = await resolveAuthenticatedProfile();
      return {
        isAuthenticated: true,
        requiresActivation: !profile.mailboxProvisioned,
        rolePrefix: profile.rolePrefix,
      };
    } catch (error) {
      if (shouldResetSession(error)) {
        clearUnifiedSession();
      }
      return {
        isAuthenticated: false,
        requiresActivation: true,
        rolePrefix: baseRolePrefix,
      };
    }
  },

  async beginOAuthLogin() {
    try {
      const session = await this.getSession();
      if (session.isAuthenticated) {
        return session;
      }

      const bridgeResult = await hydrateBridgeSession("beginLogin");
      if (bridgeResult?.redirected) {
        return {
          redirected: true,
          requiresActivation: false,
          rolePrefix: resolveRolePrefix(readStoredUnifiedSession().authMode || "user"),
        };
      }

      if (bridgeResult?.authenticated) {
        const nextSession = await this.getSession();
        return nextSession;
      }

      const entryUrl = buildSsoEntryUrl();
      if (entryUrl && typeof window !== "undefined") {
        window.location.assign(entryUrl);
        return {
          redirected: true,
          requiresActivation: false,
          rolePrefix: resolveRolePrefix(readStoredUnifiedSession().authMode || "user"),
        };
      }

      return {
        isAuthenticated: false,
        requiresActivation: false,
        rolePrefix: resolveRolePrefix(readStoredUnifiedSession().authMode || "user"),
        errorMessage: "当前未注入宿主 SSO 能力，请在悦享e食宿主应用中打开，或配置平台登录入口。",
      };
    } catch (error) {
      if (shouldResetSession(error)) {
        clearUnifiedSession();
      }
      return {
        isAuthenticated: false,
        requiresActivation: false,
        rolePrefix: resolveRolePrefix(readStoredUnifiedSession().authMode || "user"),
        errorMessage: normalizeString(error?.message || "登录态接入失败，请稍后重试"),
      };
    }
  },

  async activateMailbox({ emailPrefix }) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const normalizedPrefix = normalizeString(emailPrefix).toLowerCase().replace(/[^a-z0-9-]/g, "");

    const payload = await tryPostOfficeOperation(
      snapshot,
      "activateMailbox",
      {
        body: {
          emailPrefix: normalizedPrefix,
        },
      },
      async () => {
        upsertMailboxState(profile, normalizedPrefix);
        return {
          mailbox: {
            ...profile,
            emailPrefix: normalizedPrefix,
            email: `${profile.rolePrefix}${normalizedPrefix}@${resolveMailboxDomain()}`,
            mailboxProvisioned: true,
            provisioningStatus: "active",
            provisionedAt: new Date().toISOString(),
          },
        };
      },
    );

    const nextProfile = normalizeContractProfile(payload?.mailbox, profile.sourceUser || profile);
    if (nextProfile.emailPrefix) {
      upsertMailboxState(nextProfile, nextProfile.emailPrefix);
    }

    return {
      success: true,
      mailbox: nextProfile,
    };
  },

  async logout() {
    const bridge = resolveSsoBridge();
    if (bridge && typeof bridge.logout === "function") {
      await bridge.logout({ app: "yuexiang-post-office" });
    }
    clearUnifiedSession();
    return { success: true };
  },

  async getBootstrap() {
    const { snapshot, userPayload, profile, liveState } = await resolveAuthenticatedProfile();
    const [inviteCode, conversations] = await Promise.all([
      fetchInviteCode(snapshot, userPayload).catch(() => ""),
      fetchConversations(snapshot),
    ]);

    const remoteMailbox = await tryPostOfficeOperation(snapshot, "getMailboxProfile", {}, () => null);
    if (remoteMailbox) {
      const nextProfile = normalizeContractProfile(remoteMailbox.profile, userPayload);
      const remoteContactMessages = await fetchContactSourceMessages(snapshot, nextProfile);
      const remoteSettings = await tryPostOfficeOperation(
        snapshot,
        "getSettings",
        {},
        () => ({
          settings: getSettingsState(liveState, resolveAccountId(nextProfile), nextProfile),
        }),
      );
      const nextSettings = normalizeContractSettings(remoteSettings?.settings, nextProfile);

      if (nextProfile.emailPrefix) {
        upsertMailboxState(nextProfile, nextProfile.emailPrefix);
      }
      persistSettingsMirror(nextProfile, nextSettings);

      return {
        profile: nextProfile,
        health: {
          status: "healthy",
          label: "服务连接正常",
          source: "post-office-contract",
        },
        settings: nextSettings,
        contacts: buildUnifiedContacts({
          conversations,
          messages: remoteContactMessages,
          profile: nextProfile,
        }),
        templates: buildInviteTemplates(inviteCode, nextProfile),
        folderCounts: normalizeFolderCounts(remoteMailbox.folderCounts),
      };
    }

    const templates = buildInviteTemplates(inviteCode, profile);
    const allMessages = composeAllMessages({
      profile,
      conversations,
      state: liveState,
    });

    return {
      profile,
      health: {
        status: "healthy",
        label: "服务连接正常",
        source: "platform-live",
      },
      settings: getSettingsState(liveState, resolveAccountId(profile), profile),
      contacts: buildUnifiedContacts({
        conversations,
        messages: allMessages,
        profile,
      }),
      templates,
      folderCounts: deriveFolderCounts(allMessages),
    };
  },

  async getHealth() {
    return {
      status: "healthy",
      label: "服务连接正常",
      source: "platform-live",
    };
  },

  async listMessages({ folderId, filter, search }) {
    const { snapshot, profile, liveState } = await resolveAuthenticatedProfile();
    const normalizedFolderId = normalizePostOfficeFolder(folderId, POST_OFFICE_DEFAULT_FOLDER);
    const normalizedFilter = normalizePostOfficeMessageFilter(filter, POST_OFFICE_DEFAULT_FILTER);
    const normalizedSearch = normalizeString(search);

    const remoteMessages = await tryPostOfficeOperation(
      snapshot,
      "listMessages",
      {
        query: {
          folderId: normalizedFolderId,
          filter: normalizedFilter,
          search: normalizedSearch,
        },
      },
      () => null,
    );

    if (remoteMessages) {
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
    }

    const conversations = await fetchConversations(snapshot);
    const allMessages = composeAllMessages({
      profile,
      conversations,
      state: liveState,
    });

    return buildListPayload(allMessages, normalizedFolderId, normalizedFilter, normalizedSearch);
  },

  async toggleStar(messageId, options = {}) {
    const { snapshot, profile, liveState } = await resolveAuthenticatedProfile();
    const accountId = resolveAccountId(profile);
    const normalizedMessageId = normalizeString(messageId);
    const localMessages = getLocalMessages(liveState, accountId);
    const localMessage = localMessages.find((message) => message.id === normalizedMessageId);

    if (normalizedMessageId.startsWith("conv:")) {
      mutateConversationMeta(profile, (meta) => {
        meta.starredIds = meta.starredIds.includes(normalizedMessageId)
          ? meta.starredIds.filter((item) => item !== normalizedMessageId)
          : [...meta.starredIds, normalizedMessageId];
        return meta;
      });
      return { success: true };
    }

    if (localMessage) {
      updateLivePostOfficeState((state) => {
        state.localMessagesByAccountId[accountId] = localMessages.map((message) =>
          message.id === normalizedMessageId
            ? {
                ...message,
                isStarred: typeof options.nextStarred === "boolean" ? options.nextStarred : !message.isStarred,
              }
            : message,
        );
        return state;
      });

      return { success: true };
    }

    let nextStarred = typeof options.nextStarred === "boolean" ? options.nextStarred : undefined;
    if (typeof nextStarred !== "boolean") {
      const detail = await tryPostOfficeOperation(
        snapshot,
        "getMessageDetail",
        {
          pathParams: { messageId: normalizedMessageId },
        },
        () => null,
      );
      nextStarred = !Boolean(detail?.message?.isStarred);
    }

    await tryPostOfficeOperation(snapshot, "updateMessageStar", {
      pathParams: { messageId: normalizedMessageId },
      body: {
        starred: Boolean(nextStarred),
      },
    });

    return { success: true };
  },

  async moveMessage(messageId, targetFolder) {
    const { snapshot, profile, liveState } = await resolveAuthenticatedProfile();
    const accountId = resolveAccountId(profile);
    const normalizedMessageId = normalizeString(messageId);
    const normalizedTargetFolder = normalizePostOfficeFolder(targetFolder, POST_OFFICE_DEFAULT_FOLDER);
    const localMessages = getLocalMessages(liveState, accountId);
    const localMessage = localMessages.find((message) => message.id === normalizedMessageId);

    if (normalizedMessageId.startsWith("conv:")) {
      mutateConversationMeta(profile, (meta) => {
        if (normalizedTargetFolder === "inbox") {
          delete meta.folderByMessageId[normalizedMessageId];
        } else {
          meta.folderByMessageId[normalizedMessageId] = normalizedTargetFolder;
        }
        return meta;
      });
      return { success: true };
    }

    if (localMessage) {
      updateLivePostOfficeState((state) => {
        state.localMessagesByAccountId[accountId] = localMessages.map((message) =>
          message.id === normalizedMessageId
            ? {
                ...message,
                previousFolder: message.folder,
                folder: normalizedTargetFolder,
              }
            : message,
        );
        return state;
      });

      return { success: true };
    }

    await tryPostOfficeOperation(snapshot, "moveMessage", {
      pathParams: { messageId: normalizedMessageId },
      body: {
        targetFolder: normalizedTargetFolder,
      },
    });

    return { success: true };
  },

  async listContacts({ search }) {
    const { snapshot, profile, liveState } = await resolveAuthenticatedProfile();
    const conversations = await fetchConversations(snapshot);
    const remoteMessages = await fetchContactSourceMessages(snapshot, profile);
    const localMessages = getLocalMessages(liveState, resolveAccountId(profile));
    const contacts = buildUnifiedContacts({
      conversations,
      messages: [...remoteMessages, ...localMessages],
      profile,
    });
    return {
      items: filterContacts(contacts, search),
    };
  },

  async getSettings() {
    const { snapshot, profile, liveState } = await resolveAuthenticatedProfile();
    const remoteSettings = await tryPostOfficeOperation(
      snapshot,
      "getSettings",
      {},
      () => ({
        settings: getSettingsState(liveState, resolveAccountId(profile), profile),
      }),
    );

    const nextSettings = normalizeContractSettings(remoteSettings?.settings, profile);
    persistSettingsMirror(profile, nextSettings);
    return nextSettings;
  },

  async updateSettings(patch) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const sanitizedPatch = pickPostOfficeSettingsPatch(patch);
    const nextSettingsPayload = await tryPostOfficeOperation(
      snapshot,
      "updateSettings",
      {
        body: sanitizedPatch,
      },
      () => ({
        settings: {
          ...getSettingsState(readLivePostOfficeState(), resolveAccountId(profile), profile),
          ...sanitizedPatch,
          updatedAt: new Date().toISOString(),
        },
      }),
    );

    const nextSettings = normalizeContractSettings(nextSettingsPayload?.settings, profile);
    persistSettingsMirror(profile, nextSettings);
    return nextSettings;
  },

  async sendMessage(payload) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const recipients = normalizeArray(payload?.recipients).map((item) => normalizeString(item)).filter(Boolean);
    const subject = normalizeString(payload?.subject) || "未命名邮件";
    const textBody = normalizeString(payload?.body);
    const htmlBody = normalizeString(payload?.html);

    const remoteMessage = await tryPostOfficeOperation(
      snapshot,
      "sendMessage",
      {
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
          attachments: [],
          templateId: null,
          replyToMessageId: null,
          source: "manual",
        },
      },
      () => null,
    );

    if (remoteMessage?.message) {
      return normalizeContractMessage(remoteMessage.message, {
        sender: profile.displayName,
        senderEmail: profile.email,
        recipients,
        subject,
        content: htmlBody || `<p class="text-slate-700 leading-relaxed whitespace-pre-wrap">${escapeHtml(textBody || subject)}</p>`,
        isOutgoing: true,
        source: "mailbox",
        deliveryStatus: "accepted",
        folder: "sent",
        role: resolveSelfRoleTitle(profile.authMode),
      });
    }

    return appendLocalMessage(
      profile,
      buildLocalComposeMessage({
        folder: "sent",
        recipients,
        subject,
        body: textBody,
        profile,
      }),
    );
  },

  async saveDraft(payload) {
    const { snapshot, profile } = await resolveAuthenticatedProfile();
    const recipients = normalizeArray(payload?.recipients).map((item) => normalizeString(item)).filter(Boolean);
    const subject = normalizeString(payload?.subject) || "未命名草稿";
    const textBody = normalizeString(payload?.body);

    const remoteDraft = await tryPostOfficeOperation(
      snapshot,
      "createDraft",
      {
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
          attachments: [],
          autosave: false,
        },
      },
      () => null,
    );

    if (remoteDraft?.draft) {
      return normalizeContractMessage(remoteDraft.draft, {
        sender: profile.displayName,
        senderEmail: profile.email,
        recipients,
        subject,
        isOutgoing: true,
        source: "draft",
        deliveryStatus: "draft",
        folder: "drafts",
        role: resolveSelfRoleTitle(profile.authMode),
      });
    }

    return appendLocalMessage(
      profile,
      buildLocalComposeMessage({
        folder: "drafts",
        recipients,
        subject,
        body: textBody,
        profile,
      }),
    );
  },

  async sendInvite(payload) {
    const { snapshot, userPayload, profile } = await resolveAuthenticatedProfile();
    const inviteCode = await fetchInviteCode(snapshot, userPayload);
    const templates = buildInviteTemplates(inviteCode, profile);
    const activeTemplate = templates.find((item) => item.role === payload?.role) || templates[0];

    await platformHttp.post("/invite/share", {
      body: {
        userId: normalizeString(userPayload.id),
        phone: normalizeString(userPayload.phone),
        code: inviteCode,
      },
      headers: buildAuthHeaders(snapshot.token),
    });

    appendLocalMessage(
      profile,
      buildInviteSentMessage({
        role: normalizeString(payload?.role || "merchant"),
        recipients: normalizeArray(payload?.recipients),
        inviteCode,
        profile,
        template: activeTemplate,
      }),
    );

    return {
      recorded: true,
      inviteCode,
      recipientCount: normalizeArray(payload?.recipients).length,
      role: normalizeString(payload?.role || "merchant"),
    };
  },

  async switchDevRole(role) {
    const devBridge = typeof window !== "undefined" ? window.__YUEXIANG_POST_OFFICE_DEV_SSO__ : null;
    if (!devBridge || typeof devBridge.switchRole !== "function") {
      throw new Error("当前环境未开启开发角色切换");
    }

    clearUnifiedSession();
    await hydrateBridgeSession("beginLogin", { authMode: role });
    return this.getSession();
  },
};
