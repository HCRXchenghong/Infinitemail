import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { runtimeConfig } from "../lib/config/runtime";
import {
  POST_OFFICE_DEFAULT_FILTER,
  POST_OFFICE_DEFAULT_FOLDER,
  POST_OFFICE_VISIBLE_MAIL_FOLDERS,
} from "../services/postOfficeContract";
import { postOfficeApi } from "../services/postOfficeApi";

const PostOfficeContext = createContext(null);

const MAILBOX_VIEWS = POST_OFFICE_VISIBLE_MAIL_FOLDERS;
const CONTACT_THREAD_PAGE_SIZE = 5;

const initialCompose = {
  mode: "normal",
  recipients: "",
  subject: "",
  body: "",
  attachments: [],
  inviteRole: "account",
  inviteEmails: "",
};

function createInitialHealth() {
  return {
    label: "服务连接正常",
    status: "healthy",
  };
}

function createInitialFolderCounts() {
  return {
    inbox: 0,
    starred: 0,
    sent: 0,
    drafts: 0,
    trash: 0,
  };
}

function createInitialMailboxState() {
  return {
    activeFolder: POST_OFFICE_DEFAULT_FOLDER,
    query: { search: "", filter: POST_OFFICE_DEFAULT_FILTER },
    items: [],
    selectedMailId: null,
    isLoading: false,
  };
}

function createInitialContactsState() {
  return {
    items: [],
    selectedContactId: null,
    isLoading: false,
  };
}

function createInitialSettingsState() {
  return {
    data: null,
    isLoading: false,
  };
}

function createInitialSecuritySessionsState() {
  return {
    items: [],
    isLoading: false,
  };
}

function createInitialContactThreadState() {
  return {
    contactId: null,
    items: [],
    allItems: [],
    page: 1,
    pageSize: CONTACT_THREAD_PAGE_SIZE,
    total: 0,
    hasMore: false,
    isLoading: false,
  };
}

function createInitialAuthFlow(rolePrefix = "user-") {
  return {
    rolePrefix,
    requiresActivation: true,
    requiresProvisioning: false,
    provisioningStatus: "pending_config",
    mailboxProvisioned: false,
    profile: null,
    hasAuthenticatedSession: false,
    isLoading: false,
    errorMessage: "",
  };
}

function createInitialAdminConfig() {
  return {
    mailbox: {
      domain: runtimeConfig.mailboxDomain || "yuexiang.com",
      prefixPolicyEnabled: true,
      allowedPrefixes: ["user", "admin", "support"],
      defaultPrefix: "user",
      dns: {
        status: "pending",
        domain: runtimeConfig.mailboxDomain || "yuexiang.com",
        selector: "infinitemail",
        records: [],
        recommended: [],
        verifiedRecords: 0,
        totalRecords: 4,
      },
      server: {
        provider: "stalwart",
        enabled: false,
        strictDataPlane: false,
        baseUrl: "",
        provisionPath: "",
        lifecyclePath: "",
        messageListPath: "",
        messageDetailPath: "",
        messageSendPath: "",
        draftPath: "",
        messageStarPath: "",
        messageMovePath: "",
        messageReadPath: "",
        adminTokenSet: false,
        smtpEnabled: false,
        smtpHost: "",
        smtpPort: 25,
        smtpUsername: "",
        smtpPasswordSet: false,
        smtpTlsMode: "auto",
        imapEnabled: false,
        imapHost: "",
        imapPort: 993,
        imapUsername: "{email}",
        imapPasswordSet: false,
        imapTlsMode: "tls",
        status: "not_configured",
      },
    },
    auth: {
      oauthEnabled: true,
      oauthProviderName: "悦享账号",
      oauthClientId: "",
      oauthClientSecretSet: false,
      oauthAuthorizeUrl: "",
      oauthTokenUrl: "",
      oauthUserInfoUrl: "",
      oauthRedirectUrl: "",
      oauthScopes: ["openid", "profile", "email", "phone"],
      oauthSubjectField: "sub",
      oauthPhoneField: "phone",
      oauthEmailField: "email",
      oauthNameField: "name",
      passwordLoginEnabled: true,
      phoneLoginEnabled: true,
      emailLoginEnabled: false,
      registrationEnabled: true,
      inviteRequired: true,
      loginLandingMode: "oauth",
    },
    sms: {
      provider: "aliyun",
      aliyunEnabled: false,
      accessKeyId: "",
      accessKeySecretSet: false,
      signName: "",
      templateCode: "",
      codeTtlMinutes: 5,
    },
    ops: {
      autoRunEnabled: false,
      intervalMinutes: 5,
      lastRunStatus: "idle",
      lastRunMessage: "自动巡检未开启",
    },
    security: {
      username: "admin",
      passwordSet: false,
      apiTokenSet: false,
      apiTokenMasked: "",
    },
    deployment: {
      strict: false,
      ready: false,
      status: "development",
      store: "json",
      blockingCount: 0,
      checks: [],
    },
    usage: {
      activeSeats: 0,
      reservedSeats: 0,
      usedSeats: 0,
      seatLimit: 0,
      storageUsedBytes: 0,
      storageUsedMb: 0,
      storageLimitGb: 0,
      storagePercent: 0,
    },
    provisionJobs: [],
    invites: [],
    smsLogs: [],
    auditLogs: [],
    registeredUsers: [],
    updatedAt: null,
  };
}

function createInitialAdminAuth() {
  return {
    checked: false,
    authRequired: false,
    isAuthenticated: false,
    username: "admin",
    isLoading: false,
    errorMessage: "",
  };
}

function parseRecipients(input) {
  return String(input || "")
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizeString(value) {
  return String(value == null ? "" : value).trim();
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

function buildAuthFlowFromSession(session = {}, current = {}) {
  const hasAuthenticatedSession = Boolean(session?.isAuthenticated);
  const nextProfile = hasAuthenticatedSession ? (session?.profile || current.profile || null) : null;
  const hasEmail = Boolean(normalizeString(nextProfile?.email));
  const provisioningStatus = normalizeProvisioningStatus(
    session?.provisioningStatus || nextProfile?.provisioningStatus,
    hasEmail,
  );
  const mailboxProvisioned = Boolean(session?.mailboxProvisioned ?? nextProfile?.mailboxProvisioned) || provisioningStatus === "provisioned";
  const requiresActivation = hasAuthenticatedSession && (Boolean(session?.requiresActivation) || !hasEmail);
  const requiresProvisioning = hasAuthenticatedSession && !requiresActivation && (Boolean(session?.requiresProvisioning) || !mailboxProvisioned);

  return {
    rolePrefix: session?.rolePrefix || nextProfile?.rolePrefix || current.rolePrefix || "user-",
    requiresActivation,
    requiresProvisioning,
    provisioningStatus,
    mailboxProvisioned,
    profile: nextProfile,
    hasAuthenticatedSession,
    isLoading: false,
    errorMessage: session?.errorMessage || current.errorMessage || "",
  };
}

function provisioningNotice(status, action = "登录") {
  switch (normalizeProvisioningStatus(status, true)) {
    case "provisioned":
      return `${action}成功，已进入邮箱`;
    case "queued":
      return "邮箱开通任务处理中，开通成功后即可进入";
    case "failed":
      return "邮箱开通失败，请联系管理员处理";
    default:
      return "邮件服务尚未配置，邮箱已保留，等待后台开通";
  }
}

function normalizeArray(value) {
  return Array.isArray(value) ? value : [];
}

function resolveRoleName(role) {
  switch (normalizeString(role).toLowerCase()) {
    case "account":
      return "账号开通";
    case "collaboration":
      return "协作";
    case "notice":
      return "通知";
    default:
      return "用户";
  }
}

function paginateThreadItems(items, page = 1, pageSize = CONTACT_THREAD_PAGE_SIZE) {
  const normalizedItems = normalizeArray(items);
  const normalizedPage = Math.max(1, Number(page || 1));
  const normalizedPageSize = Math.max(1, Number(pageSize || CONTACT_THREAD_PAGE_SIZE));
  const visibleItems = normalizedItems.slice(0, normalizedPage * normalizedPageSize);

  return {
    items: visibleItems,
    allItems: normalizedItems,
    page: normalizedPage,
    pageSize: normalizedPageSize,
    total: normalizedItems.length,
    hasMore: visibleItems.length < normalizedItems.length,
  };
}

export function PostOfficeProvider({ children }) {
  const [isBootstrapping, setIsBootstrapping] = useState(true);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [authFlow, setAuthFlow] = useState(createInitialAuthFlow());
  const [profile, setProfile] = useState(null);
  const [health, setHealth] = useState(createInitialHealth);
  const [folderCounts, setFolderCounts] = useState(createInitialFolderCounts);
  const [currentView, setCurrentViewState] = useState(POST_OFFICE_DEFAULT_FOLDER);
  const [mailbox, setMailbox] = useState(createInitialMailboxState);
  const [contacts, setContacts] = useState(createInitialContactsState);
  const [contactThread, setContactThread] = useState(createInitialContactThreadState);
  const [settings, setSettings] = useState(createInitialSettingsState);
  const [securitySessions, setSecuritySessions] = useState(createInitialSecuritySessionsState);
  const [templates, setTemplates] = useState([]);
  const [compose, setCompose] = useState(initialCompose);
  const [notice, setNotice] = useState(null);
  const [adminConfig, setAdminConfig] = useState(createInitialAdminConfig);
  const [adminAuth, setAdminAuth] = useState(createInitialAdminAuth);
  const [adminConfigLoading, setAdminConfigLoading] = useState(false);

  const clearNotice = useCallback(() => {
    setNotice(null);
  }, []);

  const showNotice = useCallback((message, tone = "success") => {
    const nextMessage = normalizeString(message);
    if (!nextMessage) {
      return;
    }

    setNotice({
      id: Date.now(),
      tone,
      message: nextMessage,
    });
  }, []);

  const loadAdminConfig = useCallback(async () => {
    setAdminConfigLoading(true);
    try {
      const nextConfig = await postOfficeApi.getAdminConfig();
      setAdminConfig(nextConfig || createInitialAdminConfig());
      return nextConfig;
    } catch (error) {
      if (error?.status === 401) {
        setAdminAuth((current) => ({
          ...current,
          checked: true,
          authRequired: true,
          isAuthenticated: false,
          isLoading: false,
          errorMessage: "请先登录管理后台",
        }));
      }
      showNotice(error?.message || "后台配置加载失败", "warning");
      return adminConfig;
    } finally {
      setAdminConfigLoading(false);
    }
  }, [adminConfig, showNotice]);

  const loadAdminAuthSession = useCallback(async () => {
    setAdminAuth((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const session = await postOfficeApi.getAdminAuthSession();
      const nextAuth = {
        checked: true,
        authRequired: Boolean(session?.authRequired),
        isAuthenticated: Boolean(session?.isAuthenticated),
        username: normalizeString(session?.username) || "admin",
        isLoading: false,
        errorMessage: "",
      };
      setAdminAuth(nextAuth);
      return nextAuth;
    } catch (error) {
      const nextAuth = {
        ...createInitialAdminAuth(),
        checked: true,
        authRequired: true,
        isAuthenticated: false,
        errorMessage: error?.message || "管理后台认证状态检查失败",
      };
      setAdminAuth(nextAuth);
      return nextAuth;
    }
  }, []);

  const loginAdmin = useCallback(async (payload) => {
    setAdminAuth((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const session = await postOfficeApi.loginAdmin(payload);
      const nextAuth = {
        checked: true,
        authRequired: Boolean(session?.authRequired),
        isAuthenticated: Boolean(session?.isAuthenticated),
        username: normalizeString(session?.username || payload?.username) || "admin",
        isLoading: false,
        errorMessage: "",
      };
      setAdminAuth(nextAuth);
      showNotice("管理后台已登录", "success");
      await loadAdminConfig();
      return session;
    } catch (error) {
      setAdminAuth((current) => ({
        ...current,
        checked: true,
        authRequired: true,
        isAuthenticated: false,
        isLoading: false,
        errorMessage: error?.message || "管理员登录失败",
      }));
      showNotice(error?.message || "管理员登录失败", "warning");
      return null;
    }
  }, [loadAdminConfig, showNotice]);

  const logoutAdmin = useCallback(async () => {
    await postOfficeApi.logoutAdmin();
    setAdminConfig(createInitialAdminConfig());
    setAdminAuth((current) => ({
      ...current,
      checked: true,
      isAuthenticated: !current.authRequired,
      errorMessage: "",
    }));
    showNotice("已退出管理后台", "success");
  }, [showNotice]);

  useEffect(() => {
    if (!notice?.id) {
      return undefined;
    }

    const timer = window.setTimeout(() => {
      setNotice(null);
    }, 3200);

    return () => window.clearTimeout(timer);
  }, [notice?.id]);

  const resetWorkspaceState = useCallback(() => {
    setIsAuthenticated(false);
    setProfile(null);
    setHealth(createInitialHealth());
    setFolderCounts(createInitialFolderCounts());
    setCurrentViewState(POST_OFFICE_DEFAULT_FOLDER);
    setMailbox(createInitialMailboxState());
    setContacts(createInitialContactsState());
    setContactThread(createInitialContactThreadState());
    setSettings(createInitialSettingsState());
    setSecuritySessions(createInitialSecuritySessionsState());
    setTemplates([]);
    setCompose(initialCompose);
  }, []);

  const hydrateAuthenticatedState = useCallback(async (folderId = POST_OFFICE_DEFAULT_FOLDER) => {
    const [bootstrap, folderPayload] = await Promise.all([
      postOfficeApi.getBootstrap(),
      postOfficeApi.listMessages({
        folderId,
        filter: POST_OFFICE_DEFAULT_FILTER,
        search: "",
      }),
    ]);

    setIsAuthenticated(true);
    setProfile(bootstrap.profile);
    setHealth(bootstrap.health || createInitialHealth());
    setSettings({ data: bootstrap.settings, isLoading: false });
    setSecuritySessions(createInitialSecuritySessionsState());
    setContacts({
      items: bootstrap.contacts,
      selectedContactId: bootstrap.contacts[0]?.id || null,
      isLoading: false,
    });
    setContactThread(createInitialContactThreadState());
    setTemplates(bootstrap.templates);
    setFolderCounts(folderPayload.folderCounts || bootstrap.folderCounts || createInitialFolderCounts());
    setMailbox({
      activeFolder: folderId,
      query: { search: "", filter: POST_OFFICE_DEFAULT_FILTER },
      items: folderPayload.items,
      selectedMailId: folderPayload.items[0]?.id || null,
      isLoading: false,
    });
    setCurrentViewState(folderId);
  }, []);

	  const bootstrap = useCallback(async () => {
	    setIsBootstrapping(true);
	    try {
	      const session = await postOfficeApi.getSession();
	      let nextAdminConfig = session.adminConfig || null;
      if (!nextAdminConfig) {
        nextAdminConfig = await postOfficeApi.getAdminConfig().catch(() => null);
      }
	      setAdminConfig(session.adminConfig || nextAdminConfig || createInitialAdminConfig());
      const nextAuthFlow = buildAuthFlowFromSession(session);
      setAuthFlow(nextAuthFlow);

      if (session.isAuthenticated && !nextAuthFlow.requiresActivation && !nextAuthFlow.requiresProvisioning) {
        await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
      } else {
        resetWorkspaceState();
      }
    } finally {
      setIsBootstrapping(false);
    }
  }, [hydrateAuthenticatedState, resetWorkspaceState]);

  useEffect(() => {
    bootstrap();
  }, [bootstrap]);

  const beginUnifiedLogin = useCallback(async () => {
    setAuthFlow((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const result = await postOfficeApi.beginOAuthLogin();
      const nextAuthFlow = buildAuthFlowFromSession(result, { errorMessage: result?.errorMessage || "" });
      setAuthFlow(nextAuthFlow);

      if (result?.isAuthenticated && !nextAuthFlow.requiresActivation && !nextAuthFlow.requiresProvisioning) {
        await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
      } else {
        resetWorkspaceState();
      }

      return result;
    } catch (error) {
      const errorMessage = error?.message || "登录态接入失败，请稍后重试";
      setAuthFlow((current) => ({
        ...current,
        hasAuthenticatedSession: false,
        errorMessage,
      }));
      return {
        isAuthenticated: false,
        requiresActivation: false,
        rolePrefix: "user-",
        errorMessage,
      };
    } finally {
      setAuthFlow((current) => ({ ...current, isLoading: false }));
    }
  }, [hydrateAuthenticatedState, resetWorkspaceState]);

  const activateMailbox = useCallback(
    async (payload) => {
      setAuthFlow((current) => ({ ...current, isLoading: true }));
      try {
        const result = await postOfficeApi.activateMailbox(payload);
        const nextAuthFlow = buildAuthFlowFromSession({
          isAuthenticated: true,
          requiresActivation: false,
          requiresProvisioning: !result?.mailbox?.mailboxProvisioned,
          mailboxProvisioned: Boolean(result?.mailbox?.mailboxProvisioned),
          provisioningStatus: result?.mailbox?.provisioningStatus,
          profile: result?.mailbox,
          rolePrefix: result?.mailbox?.rolePrefix,
        });
        setAuthFlow(nextAuthFlow);
        if (!nextAuthFlow.requiresProvisioning) {
          await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
          showNotice("邮箱已真实开通，已进入悦享邮局", "success");
        } else {
          resetWorkspaceState();
          showNotice(provisioningNotice(nextAuthFlow.provisioningStatus, "开通"), "warning");
        }
      } catch (error) {
        setAuthFlow((current) => ({
          ...current,
          errorMessage: error?.message || "邮箱开通失败，请稍后重试",
        }));
        throw error;
      } finally {
        setAuthFlow((current) => ({ ...current, isLoading: false }));
      }
    },
    [hydrateAuthenticatedState, resetWorkspaceState, showNotice],
  );

  const logout = useCallback(async () => {
    await postOfficeApi.logout();
    clearNotice();
    resetWorkspaceState();

    const session = await postOfficeApi.getSession();
    setAuthFlow(buildAuthFlowFromSession(session));
  }, [clearNotice, resetWorkspaceState]);

  const loadFolder = useCallback(async (folderId, queryOverrides = {}) => {
    setMailbox((current) => ({
      ...current,
      activeFolder: folderId,
      query: {
        ...current.query,
        ...queryOverrides,
      },
      isLoading: true,
    }));

    const query = {
      search: queryOverrides.search ?? mailbox.query.search,
      filter: queryOverrides.filter ?? mailbox.query.filter,
    };

    const payload = await postOfficeApi.listMessages({
      folderId,
      ...query,
    });

    setFolderCounts(payload.folderCounts || createInitialFolderCounts());
    setMailbox((current) => {
      const nextSelectedId = payload.items.some((item) => item.id === current.selectedMailId)
        ? current.selectedMailId
        : payload.items[0]?.id || null;

      return {
        ...current,
        activeFolder: folderId,
        query,
        items: payload.items,
        selectedMailId: nextSelectedId,
        isLoading: false,
      };
    });
  }, [mailbox.query.filter, mailbox.query.search]);

  const setCurrentView = useCallback((view) => {
    setCurrentViewState(view);
    if (view === "compose") {
      setCompose((current) => ({ ...initialCompose, mode: current.mode }));
    }
  }, []);

  const selectMail = useCallback((messageId) => {
    const currentMessage = mailbox.items.find((item) => item.id === messageId);
    setMailbox((current) => ({
      ...current,
      selectedMailId: messageId,
      items: current.items.map((item) =>
        item.id === messageId && item.isUnread ? { ...item, isUnread: false } : item,
      ),
    }));
    if (!currentMessage?.isUnread || currentMessage.isOutgoing) {
      return;
    }
    if (mailbox.activeFolder === "inbox") {
      setFolderCounts((current) => ({
        ...current,
        inbox: Math.max(0, Number(current.inbox || 0) - 1),
      }));
    }
    postOfficeApi.updateMessageReadState(messageId, { isUnread: false }).catch((error) => {
      setNotice({ type: "error", message: error.message || "邮件已读状态同步失败" });
      loadFolder(mailbox.activeFolder, mailbox.query);
    });
  }, [loadFolder, mailbox.activeFolder, mailbox.items, mailbox.query]);

  const toggleStar = useCallback(async (messageId) => {
    const currentMessage = mailbox.items.find((item) => item.id === messageId);
    await postOfficeApi.toggleStar(messageId, {
      nextStarred: currentMessage ? !currentMessage.isStarred : undefined,
    });
    await loadFolder(mailbox.activeFolder, mailbox.query);
  }, [loadFolder, mailbox.activeFolder, mailbox.items, mailbox.query]);

  const moveMessage = useCallback(async (messageId, targetFolder) => {
    await postOfficeApi.moveMessage(messageId, targetFolder);
    await loadFolder(mailbox.activeFolder, mailbox.query);
  }, [loadFolder, mailbox.activeFolder, mailbox.query]);

  const loadContacts = useCallback(async (search = "") => {
    setContacts((current) => ({ ...current, isLoading: true }));
    try {
      const payload = await postOfficeApi.listContacts({ search });
      setContacts((current) => ({
        ...current,
        items: payload.items,
        selectedContactId:
          payload.items.some((item) => item.id === current.selectedContactId)
            ? current.selectedContactId
            : payload.items[0]?.id || null,
        isLoading: false,
      }));
      return payload;
    } catch (error) {
      setContacts((current) => ({ ...current, isLoading: false }));
      showNotice(error?.message || "通讯录加载失败，请稍后重试", "warning");
      return null;
    }
  }, [showNotice]);

  const selectContact = useCallback((contactId) => {
    setContacts((current) => ({ ...current, selectedContactId: contactId }));
    setContactThread((current) =>
      current.contactId === contactId ? current : createInitialContactThreadState(),
    );
  }, []);

  const updateComposeField = useCallback((field, value) => {
    setCompose((current) => ({
      ...current,
      [field]: value,
    }));
  }, []);

  const setComposeMode = useCallback((mode) => {
    setCompose((current) => ({
      ...current,
      mode,
    }));
  }, []);

  const addComposeAttachments = useCallback((attachments) => {
    const nextAttachments = normalizeArray(attachments).filter(Boolean);
    if (!nextAttachments.length) {
      return;
    }
    setCompose((current) => ({
      ...current,
      attachments: [...normalizeArray(current.attachments), ...nextAttachments].slice(0, 20),
    }));
  }, []);

  const uploadComposeAttachments = useCallback(async (attachments) => {
    const nextAttachments = normalizeArray(attachments).filter(Boolean);
    if (!nextAttachments.length) {
      return [];
    }
    try {
      const uploaded = await postOfficeApi.uploadAttachments(nextAttachments);
      addComposeAttachments(uploaded);
      return uploaded;
    } catch (error) {
      showNotice(error?.message || "附件上传失败，请重新选择", "warning");
      return [];
    }
  }, [addComposeAttachments, showNotice]);

  const removeComposeAttachment = useCallback((attachmentId) => {
    const normalizedId = normalizeString(attachmentId);
    setCompose((current) => ({
      ...current,
      attachments: normalizeArray(current.attachments).filter((attachment) => attachment?.id !== normalizedId),
    }));
  }, []);

  const openComposeForContact = useCallback((contact) => {
    setCompose({
      ...initialCompose,
      mode: "normal",
      recipients: contact.email,
      subject: "",
      body: "",
    });
    setCurrentViewState("compose");
  }, []);

  const loadContactThread = useCallback(async (contact) => {
    if (!contact) {
      setContactThread(createInitialContactThreadState());
      return [];
    }

    setContactThread((current) => ({
      ...current,
      contactId: contact.id,
      isLoading: true,
    }));

    try {
      const backendThread = await postOfficeApi.getContactThread(contact.id);
      const threadPage = paginateThreadItems(normalizeArray(backendThread?.items));
      setContactThread({
        contactId: contact.id,
        ...threadPage,
        total: Number(backendThread?.total || threadPage.total || 0),
        isLoading: false,
      });
      return normalizeArray(backendThread?.items);
    } catch (error) {
      setContactThread((current) => ({
        ...current,
        contactId: contact.id,
        isLoading: false,
      }));
      showNotice(error?.message || "联系人往来记录加载失败，请稍后重试", "warning");
      return [];
    }
  }, [showNotice]);

  const loadMoreContactThread = useCallback(() => {
    setContactThread((current) => {
      if (!current.hasMore) {
        return current;
      }

      return {
        ...current,
        ...paginateThreadItems(current.allItems, current.page + 1, current.pageSize),
        contactId: current.contactId,
        isLoading: false,
      };
    });
  }, []);

  const openContactThreadMessage = useCallback(async (message) => {
    if (!message?.id || !message?.folderId) {
      return false;
    }

    setMailbox((current) => ({ ...current, isLoading: true }));

    try {
      const payload = await postOfficeApi.listMessages({
        folderId: message.folderId,
        filter: POST_OFFICE_DEFAULT_FILTER,
        search: "",
      });

      setFolderCounts(payload.folderCounts || createInitialFolderCounts());
      setMailbox({
        activeFolder: message.folderId,
        query: { search: "", filter: POST_OFFICE_DEFAULT_FILTER },
        items: payload.items,
        selectedMailId:
          payload.items.find((item) => item.id === message.id)?.id ||
          payload.items[0]?.id ||
          null,
        isLoading: false,
      });
      setCurrentViewState(message.folderId);
      return true;
    } catch (error) {
      setMailbox((current) => ({ ...current, isLoading: false }));
      showNotice(error?.message || "邮件详情打开失败，请稍后重试", "warning");
      return false;
    }
  }, [showNotice]);

  const openContactHistory = useCallback(async (contact) => {
    if (!contact) {
      return false;
    }

    const matches = await loadContactThread(contact);
    if (!matches.length) {
      showNotice(`暂未找到与 ${contact.name} 的往来邮件`, "warning");
      return false;
    }

    const opened = await openContactThreadMessage(matches[0]);
    if (opened) {
      showNotice(`已定位到与 ${contact.name} 的往来邮件`, "success");
    }
    return opened;
  }, [loadContactThread, openContactThreadMessage, showNotice]);

  const prepareReply = useCallback((mail, type = "reply") => {
    const recipient = type === "forward" ? "" : mail.senderEmail;
    const subjectPrefix = type === "forward" ? "Fwd: " : "Re: ";
    const body = `\n\n-------- 原始邮件 --------\n主题：${mail.subject}\n发件人：${mail.sender} <${mail.senderEmail}>\n时间：${mail.dateTimeLabel}\n\n${mail.snippet}`;
    setCompose({
      ...initialCompose,
      mode: "normal",
      recipients: recipient,
      subject: `${subjectPrefix}${mail.subject}`,
      body,
    });
    setCurrentViewState("compose");
  }, []);

  const sendCompose = useCallback(async () => {
    try {
      if (compose.mode === "invite") {
        const recipients = parseRecipients(compose.inviteEmails);
        const result = await postOfficeApi.sendInvite({
          role: compose.inviteRole,
          recipients,
        });
        setCompose(initialCompose);
        setCurrentViewState("sent");
        await loadFolder("sent", { search: "", filter: POST_OFFICE_DEFAULT_FILTER });
        await loadContacts("");
        showNotice(
          `已向 ${result?.recipientCount || recipients.length} 位${resolveRoleName(result?.role || compose.inviteRole)}对象发送通知邮件`,
          "success",
        );
        return result;
      }

      const recipients = parseRecipients(compose.recipients);
      const result = await postOfficeApi.sendMessage({
        recipients,
        subject: compose.subject,
        body: compose.body,
        attachments: normalizeArray(compose.attachments),
      });

      setCompose(initialCompose);
      setCurrentViewState("sent");
      await loadFolder("sent", { search: "", filter: POST_OFFICE_DEFAULT_FILTER });
      await loadContacts("");
      showNotice(`邮件已发送至 ${recipients.length || 1} 位收件人`, "success");
      return result;
    } catch (error) {
      showNotice(error?.message || "发送失败，请稍后重试", "warning");
      return null;
    }
  }, [compose, loadContacts, loadFolder, showNotice]);

  const saveDraft = useCallback(async () => {
    try {
      const result = await postOfficeApi.saveDraft({
        recipients: parseRecipients(compose.recipients),
        subject: compose.subject,
        body: compose.body,
        attachments: normalizeArray(compose.attachments),
      });

      setCurrentViewState("drafts");
      await loadFolder("drafts", { search: "", filter: POST_OFFICE_DEFAULT_FILTER });
      await loadContacts("");
      showNotice("草稿已保存", "success");
      return result;
    } catch (error) {
      showNotice(error?.message || "草稿保存失败，请稍后重试", "warning");
      return null;
    }
  }, [compose, loadContacts, loadFolder, showNotice]);

  const discardCompose = useCallback(() => {
    setCompose(initialCompose);
    showNotice("已清空当前编辑内容", "success");
  }, [showNotice]);

  const saveSettings = useCallback(async (patch) => {
    const nextPatch = patch && typeof patch === "object" ? patch : {};
    const previousSettings = settings.data;

    setSettings((current) => ({
      ...current,
      isLoading: true,
      data: {
        ...current.data,
        ...nextPatch,
      },
    }));

    try {
      const nextSettings = await postOfficeApi.updateSettings(nextPatch);
      setSettings({
        data: nextSettings,
        isLoading: false,
      });
      showNotice("设置已保存", "success");
      return nextSettings;
    } catch (error) {
      setSettings({
        data: previousSettings,
        isLoading: false,
      });
      showNotice(error?.message || "设置保存失败，请稍后重试", "warning");
      return null;
    }
  }, [settings.data, showNotice]);

  const loadSecuritySessions = useCallback(async () => {
    if (typeof postOfficeApi.listSecuritySessions !== "function") {
      showNotice("登录设备接口不可用，请检查 BFF 服务", "warning");
      return null;
    }
    setSecuritySessions((current) => ({ ...current, isLoading: true }));
    try {
      const payload = await postOfficeApi.listSecuritySessions();
      setSecuritySessions({
        items: normalizeArray(payload?.items),
        isLoading: false,
      });
      return payload;
    } catch (error) {
      setSecuritySessions((current) => ({ ...current, isLoading: false }));
      showNotice(error?.message || "登录设备加载失败，请稍后重试", "warning");
      return null;
    }
  }, [showNotice]);

  const logoutOtherSecuritySessions = useCallback(async () => {
    if (typeof postOfficeApi.logoutOtherSecuritySessions !== "function") {
      showNotice("登录设备接口不可用，请检查 BFF 服务", "warning");
      return null;
    }
    setSecuritySessions((current) => ({ ...current, isLoading: true }));
    try {
      const payload = await postOfficeApi.logoutOtherSecuritySessions();
      setSecuritySessions({
        items: normalizeArray(payload?.items),
        isLoading: false,
      });
      showNotice(`已退出 ${Number(payload?.removed || 0)} 个其他登录设备`, "success");
      return payload;
    } catch (error) {
      setSecuritySessions((current) => ({ ...current, isLoading: false }));
      showNotice(error?.message || "退出其他设备失败，请稍后重试", "warning");
      return null;
    }
  }, [showNotice]);

  const revokeSecuritySession = useCallback(async (sessionId) => {
    if (typeof postOfficeApi.revokeSecuritySession !== "function") {
      showNotice("登录设备接口不可用，请检查 BFF 服务", "warning");
      return null;
    }
    setSecuritySessions((current) => ({ ...current, isLoading: true }));
    try {
      const payload = await postOfficeApi.revokeSecuritySession(sessionId);
      setSecuritySessions({
        items: normalizeArray(payload?.items),
        isLoading: false,
      });
      showNotice("登录设备已移除", "success");
      return payload;
    } catch (error) {
      setSecuritySessions((current) => ({ ...current, isLoading: false }));
      showNotice(error?.message || "移除登录设备失败，请稍后重试", "warning");
      return null;
    }
  }, [showNotice]);

  const saveAdminConfig = useCallback(async (patch) => {
    if (typeof postOfficeApi.updateAdminConfig !== "function") {
      showNotice("管理配置接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    setAdminConfigLoading(true);
    try {
      const nextConfig = await postOfficeApi.updateAdminConfig(patch);
      setAdminConfig(nextConfig || createInitialAdminConfig());
      showNotice("后台配置已保存", "success");
      return nextConfig;
    } catch (error) {
      showNotice(error?.message || "后台配置保存失败", "warning");
      return null;
    } finally {
      setAdminConfigLoading(false);
    }
  }, [showNotice]);

  const createMailboxInvite = useCallback(async (payload) => {
    if (typeof postOfficeApi.createMailboxInvite !== "function") {
      showNotice("注册链接接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    try {
      const invite = await postOfficeApi.createMailboxInvite(payload);
      await loadAdminConfig();
      showNotice(`已生成 ${invite.email} 的注册链接`, "success");
      return invite;
    } catch (error) {
      showNotice(error?.message || "注册链接生成失败", "warning");
      return null;
    }
  }, [loadAdminConfig, showNotice]);

  const verifyMailboxDomain = useCallback(async () => {
    if (typeof postOfficeApi.verifyMailboxDomain !== "function") {
      showNotice("域名验证接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    setAdminConfigLoading(true);
    try {
      const nextConfig = await postOfficeApi.verifyMailboxDomain();
      setAdminConfig(nextConfig || createInitialAdminConfig());
      const status = nextConfig?.mailbox?.dns?.status;
      showNotice(status === "verified" ? "域名 DNS 已全部通过" : "域名 DNS 已检查，请查看未通过记录", status === "verified" ? "success" : "warning");
      return nextConfig;
    } catch (error) {
      showNotice(error?.message || "域名 DNS 验证失败", "warning");
      return null;
    } finally {
      setAdminConfigLoading(false);
    }
  }, [showNotice]);

  const testMailServer = useCallback(async () => {
    if (typeof postOfficeApi.testMailServer !== "function") {
      showNotice("邮件服务测试接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    setAdminConfigLoading(true);
    try {
      const nextConfig = await postOfficeApi.testMailServer();
      setAdminConfig(nextConfig || createInitialAdminConfig());
      const status = nextConfig?.mailbox?.server?.status;
      showNotice(status === "online" ? "邮件服务连接正常" : "邮件服务暂未连通", status === "online" ? "success" : "warning");
      return nextConfig;
    } catch (error) {
      showNotice(error?.message || "邮件服务测试失败", "warning");
      return null;
    } finally {
      setAdminConfigLoading(false);
    }
  }, [showNotice]);

  const disableMailboxAccount = useCallback(async (accountId) => {
    if (typeof postOfficeApi.disableMailboxAccount !== "function") {
      showNotice("账号禁用接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    try {
      const result = await postOfficeApi.disableMailboxAccount(accountId);
      await loadAdminConfig();
      showNotice("账号已禁用，会话已清理", "success");
      return result;
    } catch (error) {
      showNotice(error?.message || "账号禁用失败", "warning");
      return null;
    }
  }, [loadAdminConfig, showNotice]);

  const enableMailboxAccount = useCallback(async (accountId) => {
    if (typeof postOfficeApi.enableMailboxAccount !== "function") {
      showNotice("账号启用接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    try {
      const result = await postOfficeApi.enableMailboxAccount(accountId);
      await loadAdminConfig();
      showNotice("账号已启用", "success");
      return result;
    } catch (error) {
      showNotice(error?.message || "账号启用失败", "warning");
      return null;
    }
  }, [loadAdminConfig, showNotice]);

  const resetMailboxAccountPassword = useCallback(async (accountId) => {
    if (typeof postOfficeApi.resetMailboxAccountPassword !== "function") {
      showNotice("重置密码接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    try {
      const result = await postOfficeApi.resetMailboxAccountPassword(accountId);
      await loadAdminConfig();
      showNotice(`临时密码：${result?.temporaryPassword || "-"}`, "success");
      return result;
    } catch (error) {
      showNotice(error?.message || "重置密码失败", "warning");
      return null;
    }
  }, [loadAdminConfig, showNotice]);

  const retryMailboxProvision = useCallback(async (accountId) => {
    if (typeof postOfficeApi.retryMailboxProvision !== "function") {
      showNotice("邮箱开通接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    try {
      const result = await postOfficeApi.retryMailboxProvision(accountId);
      await loadAdminConfig();
      showNotice(result?.message || "邮箱已真实开通", "success");
      return result;
    } catch (error) {
      showNotice(error?.message || "邮箱开通重试失败", "warning");
      return null;
    }
  }, [loadAdminConfig, showNotice]);

  const runOperationalTasks = useCallback(async () => {
    if (typeof postOfficeApi.runOperationalTasks !== "function") {
      showNotice("任务中心接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    try {
      const result = await postOfficeApi.runOperationalTasks();
      const nextConfig = result?.config || result;
      setAdminConfig(nextConfig || createInitialAdminConfig());
      const provisioning = result?.summary?.provisioning;
      showNotice(provisioning?.message || "任务中心已执行", provisioning?.failed ? "warning" : "success");
      return result;
    } catch (error) {
      showNotice(error?.message || "任务中心执行失败", "warning");
      return null;
    }
  }, [showNotice]);

  const sendAuthSmsCode = useCallback(async (payload) => {
    if (typeof postOfficeApi.sendAuthSmsCode !== "function") {
      showNotice("短信验证码接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    try {
      const log = await postOfficeApi.sendAuthSmsCode(payload);
      await loadAdminConfig();
      showNotice(log?.provider === "aliyun" ? "验证码已通过阿里云短信发送" : `验证码已生成：${log.code}`, "success");
      return log;
    } catch (error) {
      showNotice(error?.message || "验证码发送失败", "warning");
      return null;
    }
  }, [loadAdminConfig, showNotice]);

  const registerWithPassword = useCallback(async (payload) => {
    if (typeof postOfficeApi.registerWithPassword !== "function") {
      showNotice("注册接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    setAuthFlow((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const session = await postOfficeApi.registerWithPassword(payload);
      await loadAdminConfig();
      const nextAuthFlow = buildAuthFlowFromSession(session);
      setAuthFlow(nextAuthFlow);
      if (!nextAuthFlow.requiresActivation && !nextAuthFlow.requiresProvisioning) {
        await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
      } else {
        resetWorkspaceState();
      }
      showNotice(provisioningNotice(nextAuthFlow.provisioningStatus, "注册"), nextAuthFlow.mailboxProvisioned ? "success" : "warning");
      return true;
    } catch (error) {
      setAuthFlow((current) => ({ ...current, errorMessage: error?.message || "注册失败" }));
      showNotice(error?.message || "注册失败", "warning");
      return null;
    } finally {
      setAuthFlow((current) => ({ ...current, isLoading: false }));
    }
  }, [hydrateAuthenticatedState, loadAdminConfig, resetWorkspaceState, showNotice]);

  const loginWithPassword = useCallback(async (payload) => {
    if (typeof postOfficeApi.loginWithPassword !== "function") {
      showNotice("登录接口不可用，请检查 BFF 服务", "warning");
      return null;
    }

    setAuthFlow((current) => ({ ...current, isLoading: true, errorMessage: "" }));
    try {
      const session = await postOfficeApi.loginWithPassword(payload);
      const nextAuthFlow = buildAuthFlowFromSession(session);
      setAuthFlow(nextAuthFlow);
      if (!nextAuthFlow.requiresActivation && !nextAuthFlow.requiresProvisioning) {
        await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
      } else {
        resetWorkspaceState();
      }
      showNotice(provisioningNotice(nextAuthFlow.provisioningStatus, "登录"), nextAuthFlow.mailboxProvisioned ? "success" : "warning");
      return true;
    } catch (error) {
      setAuthFlow((current) => ({ ...current, errorMessage: error?.message || "登录失败" }));
      showNotice(error?.message || "登录失败", "warning");
      return null;
    } finally {
      setAuthFlow((current) => ({ ...current, isLoading: false }));
    }
  }, [hydrateAuthenticatedState, resetWorkspaceState, showNotice]);

  const actions = useMemo(() => ({
    beginUnifiedLogin,
    activateMailbox,
    logout,
    loadFolder,
    setCurrentView,
    selectMail,
    toggleStar,
    moveMessage,
    loadContacts,
    selectContact,
    updateComposeField,
    setComposeMode,
    addComposeAttachments,
    uploadComposeAttachments,
    removeComposeAttachment,
    openComposeForContact,
    loadContactThread,
    loadMoreContactThread,
    openContactThreadMessage,
    openContactHistory,
    prepareReply,
    sendCompose,
    saveDraft,
    discardCompose,
    saveSettings,
    loadSecuritySessions,
    logoutOtherSecuritySessions,
    revokeSecuritySession,
    loadAdminAuthSession,
    loginAdmin,
    logoutAdmin,
    loadAdminConfig,
    saveAdminConfig,
    createMailboxInvite,
    verifyMailboxDomain,
    testMailServer,
    disableMailboxAccount,
    enableMailboxAccount,
    resetMailboxAccountPassword,
    retryMailboxProvision,
    runOperationalTasks,
    sendAuthSmsCode,
    registerWithPassword,
    loginWithPassword,
    refreshSession: bootstrap,
    dismissNotice: clearNotice,
  }), [
    beginUnifiedLogin,
    activateMailbox,
    logout,
    loadFolder,
    setCurrentView,
    selectMail,
    toggleStar,
    moveMessage,
    loadContacts,
    selectContact,
    updateComposeField,
    setComposeMode,
    addComposeAttachments,
    uploadComposeAttachments,
    removeComposeAttachment,
    openComposeForContact,
    loadContactThread,
    loadMoreContactThread,
    openContactThreadMessage,
    openContactHistory,
    prepareReply,
    sendCompose,
    saveDraft,
    discardCompose,
    saveSettings,
    loadSecuritySessions,
    logoutOtherSecuritySessions,
    revokeSecuritySession,
    loadAdminAuthSession,
    loginAdmin,
    logoutAdmin,
    loadAdminConfig,
    saveAdminConfig,
    createMailboxInvite,
    verifyMailboxDomain,
    testMailServer,
    disableMailboxAccount,
    enableMailboxAccount,
    resetMailboxAccountPassword,
    retryMailboxProvision,
    runOperationalTasks,
    sendAuthSmsCode,
    registerWithPassword,
    loginWithPassword,
    bootstrap,
    clearNotice,
  ]);

  const value = useMemo(() => {
    const selectedMail = mailbox.items.find((item) => item.id === mailbox.selectedMailId) || null;
    const selectedContact = contacts.items.find((item) => item.id === contacts.selectedContactId) || null;

    return {
      isBootstrapping,
      isAuthenticated,
      authFlow,
      profile,
      health,
      folderCounts,
      currentView,
      mailbox,
      selectedMail,
      contacts,
      selectedContact,
      contactThread,
      settings,
      securitySessions,
      templates,
      compose,
      notice,
      adminAuth,
      adminConfig,
      adminConfigLoading,
      mailViews: MAILBOX_VIEWS,
      actions,
    };
  }, [
    isBootstrapping,
    isAuthenticated,
    authFlow,
    profile,
    health,
    folderCounts,
    currentView,
    mailbox,
    contacts,
    contactThread,
    settings,
    securitySessions,
    templates,
    compose,
    notice,
    adminAuth,
    adminConfig,
    adminConfigLoading,
    actions,
  ]);

  return <PostOfficeContext.Provider value={value}>{children}</PostOfficeContext.Provider>;
}

export function usePostOffice() {
  const context = useContext(PostOfficeContext);
  if (!context) {
    throw new Error("usePostOffice must be used within PostOfficeProvider");
  }
  return context;
}
