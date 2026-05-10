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
const CONTACT_HISTORY_FOLDERS = ["inbox", "sent", "drafts", "starred", "trash"];
const CONTACT_THREAD_PAGE_SIZE = 5;

const initialCompose = {
  mode: "normal",
  recipients: "",
  subject: "",
  body: "",
  inviteRole: "merchant",
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
    hasAuthenticatedSession: false,
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

function normalizeArray(value) {
  return Array.isArray(value) ? value : [];
}

function resolveRoleName(role) {
  switch (normalizeString(role).toLowerCase()) {
    case "merchant":
      return "商户";
    case "rider":
      return "骑手";
    default:
      return "用户";
  }
}

function matchesContactMessage(message, contact) {
  const contactEmail = normalizeString(contact?.email).toLowerCase();
  const contactName = normalizeString(contact?.name).toLowerCase();
  const senderEmail = normalizeString(message?.senderEmail).toLowerCase();
  const recipients = normalizeArray(message?.recipients).map((item) => normalizeString(item).toLowerCase());
  const haystack = [
    message?.sender,
    message?.subject,
    message?.snippet,
    ...normalizeArray(message?.tags),
  ]
    .join(" ")
    .toLowerCase();

  if (contactEmail && (senderEmail === contactEmail || recipients.includes(contactEmail))) {
    return true;
  }

  if (contactName && haystack.includes(contactName)) {
    return true;
  }

  return false;
}

function compareMessageTime(left, right) {
  return new Date(right?.sortAt || 0).getTime() - new Date(left?.sortAt || 0).getTime();
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
  const [templates, setTemplates] = useState([]);
  const [compose, setCompose] = useState(initialCompose);
  const [notice, setNotice] = useState(null);

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
      setAuthFlow({
        rolePrefix: session.rolePrefix || "user-",
        requiresActivation: Boolean(session.requiresActivation),
        hasAuthenticatedSession: Boolean(session.isAuthenticated),
        isLoading: false,
        errorMessage: "",
      });

      if (session.isAuthenticated && !session.requiresActivation) {
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
      setAuthFlow((current) => ({
        ...current,
        rolePrefix: result.rolePrefix || current.rolePrefix,
        requiresActivation: Boolean(result?.requiresActivation),
        hasAuthenticatedSession: Boolean(result?.isAuthenticated),
        errorMessage: result?.errorMessage || "",
      }));

      if (result?.isAuthenticated && !result.requiresActivation) {
        await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
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
  }, [hydrateAuthenticatedState]);

  const activateMailbox = useCallback(
    async (emailPrefix) => {
      setAuthFlow((current) => ({ ...current, isLoading: true }));
      try {
        await postOfficeApi.activateMailbox({ emailPrefix });
        setAuthFlow((current) => ({
          ...current,
          errorMessage: "",
          hasAuthenticatedSession: true,
          requiresActivation: false,
        }));
        await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
        showNotice("邮箱已开通，已进入悦享邮局", "success");
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
    [hydrateAuthenticatedState, showNotice],
  );

  const logout = useCallback(async () => {
    await postOfficeApi.logout();
    clearNotice();
    resetWorkspaceState();

    const session = await postOfficeApi.getSession();
    setAuthFlow({
      rolePrefix: session.rolePrefix || "user-",
      requiresActivation: Boolean(session.requiresActivation),
      hasAuthenticatedSession: Boolean(session.isAuthenticated),
      isLoading: false,
      errorMessage: "",
    });
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
    setMailbox((current) => ({
      ...current,
      selectedMailId: messageId,
    }));
  }, []);

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
      const folderPayloads = await Promise.all(
        CONTACT_HISTORY_FOLDERS.map(async (folderId) => ({
          folderId,
          payload: await postOfficeApi.listMessages({
            folderId,
            filter: POST_OFFICE_DEFAULT_FILTER,
            search: "",
          }),
        })),
      );

      const matches = folderPayloads
        .flatMap(({ folderId, payload }) =>
          normalizeArray(payload.items).map((item) => ({
            ...item,
            folderId,
          })),
        )
        .filter((item) => matchesContactMessage(item, contact))
        .reduce((accumulator, item) => {
          if (!accumulator.some((entry) => entry.id === item.id)) {
            accumulator.push(item);
          }
          return accumulator;
        }, [])
        .sort(compareMessageTime);

      const threadPage = paginateThreadItems(matches);
      setContactThread({
        contactId: contact.id,
        ...threadPage,
        isLoading: false,
      });
      return matches;
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
          `已向 ${result?.recipientCount || recipients.length} 位${resolveRoleName(result?.role || compose.inviteRole)}对象发送邀请函`,
          "success",
        );
        return result;
      }

      const recipients = parseRecipients(compose.recipients);
      const result = await postOfficeApi.sendMessage({
        recipients,
        subject: compose.subject,
        body: compose.body,
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

  const switchRole = useCallback(async (role) => {
    if (!runtimeConfig.devSsoEnabled) {
      showNotice("当前环境未开启开发角色切换", "warning");
      return null;
    }

    const nextRole = normalizeString(role).toLowerCase() || "user";
    setIsBootstrapping(true);
    clearNotice();

    try {
      const session = await postOfficeApi.switchDevRole(nextRole);
      resetWorkspaceState();
      setAuthFlow({
        rolePrefix: session?.rolePrefix || "user-",
        requiresActivation: Boolean(session?.requiresActivation),
        hasAuthenticatedSession: Boolean(session?.isAuthenticated),
        isLoading: false,
        errorMessage: "",
      });

      if (session?.isAuthenticated && !session.requiresActivation) {
        await hydrateAuthenticatedState(POST_OFFICE_DEFAULT_FOLDER);
        showNotice(`已切换到${resolveRoleName(nextRole)}视角`, "success");
      } else {
        showNotice(`已切换到${resolveRoleName(nextRole)}视角，请开通对应邮箱`, "success");
      }

      return session;
    } catch (error) {
      setAuthFlow((current) => ({
        ...current,
        isLoading: false,
        errorMessage: error?.message || "角色切换失败，请稍后重试",
      }));
      showNotice(error?.message || "角色切换失败，请稍后重试", "warning");
      return null;
    } finally {
      setIsBootstrapping(false);
    }
  }, [clearNotice, hydrateAuthenticatedState, resetWorkspaceState, showNotice]);

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
    openComposeForContact,
    loadContactThread,
    loadMoreContactThread,
    openContactThreadMessage,
    openContactHistory,
    prepareReply,
    sendCompose,
    saveDraft,
    saveSettings,
    switchRole,
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
    openComposeForContact,
    loadContactThread,
    loadMoreContactThread,
    openContactThreadMessage,
    openContactHistory,
    prepareReply,
    sendCompose,
    saveDraft,
    saveSettings,
    switchRole,
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
      templates,
      compose,
      notice,
      roleSwitcherEnabled: runtimeConfig.devSsoEnabled,
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
    templates,
    compose,
    notice,
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
