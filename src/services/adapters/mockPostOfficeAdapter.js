import { readMockState, updateMockState } from "../mock/store";

function delay(ms = 180) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
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
  if (!search) {
    return true;
  }

  const term = search.toLowerCase();
  return [
    message.sender,
    message.senderEmail,
    message.subject,
    message.snippet,
    ...(message.tags || []),
  ]
    .join(" ")
    .toLowerCase()
    .includes(term);
}

function sortMessages(messages) {
  return [...messages].sort((left, right) => new Date(right.sortAt).getTime() - new Date(left.sortAt).getTime());
}

function buildListPayload(state, folderId, filter = "all", search = "") {
  const items = sortMessages(
    state.messages.filter((message) => matchesFolder(message, folderId) && matchesFilter(message, filter) && matchesSearch(message, search)),
  );

  return {
    items,
    folderCounts: deriveFolderCounts(state.messages),
  };
}

function buildBootstrapPayload(state) {
  return {
    profile: state.profile,
    health: state.health,
    settings: state.settings,
    contacts: state.contacts,
    templates: state.templates,
    folderCounts: deriveFolderCounts(state.messages),
  };
}

function asHtmlContent(text) {
  return `<p class="text-slate-700 leading-relaxed">${text.replace(/\n/g, "<br/>")}</p>`;
}

function nextId(prefix = "mail") {
  return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
}

export const mockPostOfficeAdapter = {
  async getSession() {
    await delay();
    const state = readMockState();
    return {
      isAuthenticated: state.auth.isAuthenticated,
      requiresActivation: !state.profile.mailboxProvisioned,
      rolePrefix: state.profile.rolePrefix,
    };
  },

  async beginOAuthLogin() {
    await delay();
    const state = readMockState();
    return {
      requiresActivation: !state.profile.mailboxProvisioned,
      rolePrefix: state.profile.rolePrefix,
    };
  },

  async activateMailbox({ emailPrefix }) {
    await delay(250);
    return updateMockState((state) => {
      state.auth.isAuthenticated = true;
      state.profile.mailboxProvisioned = true;
      state.profile.emailPrefix = emailPrefix;
      state.profile.email = `${state.profile.rolePrefix}${emailPrefix}@${state.profile.mailboxDomain}`;
      state.profile.displayName = "MyName";
      state.settings.defaultSenderName = `悦享用户_${state.profile.displayName}`;

      state.messages = state.messages.map((message) => ({
        ...message,
        senderEmail: message.isOutgoing ? state.profile.email : message.senderEmail,
        recipients: message.isOutgoing ? message.recipients : [state.profile.email],
      }));

      return state;
    });
  },

  async logout() {
    await delay();
    updateMockState((state) => {
      state.auth.isAuthenticated = false;
      return state;
    });
    return { success: true };
  },

  async getBootstrap() {
    await delay();
    const state = readMockState();
    return buildBootstrapPayload(state);
  },

  async getHealth() {
    await delay(120);
    return readMockState().health;
  },

  async listMessages({ folderId, filter, search }) {
    await delay();
    return buildListPayload(readMockState(), folderId, filter, search);
  },

  async toggleStar(messageId) {
    await delay(120);
    return updateMockState((state) => {
      const target = state.messages.find((message) => message.id === messageId);
      if (target) {
        target.isStarred = !target.isStarred;
      }
      return state;
    });
  },

  async moveMessage(messageId, targetFolder) {
    await delay(120);
    return updateMockState((state) => {
      const target = state.messages.find((message) => message.id === messageId);
      if (target) {
        if (target.folder !== "trash") {
          target.previousFolder = target.folder;
        }
        target.folder = targetFolder;
      }
      return state;
    });
  },

  async listContacts({ search }) {
    await delay();
    const state = readMockState();
    const keyword = (search || "").trim().toLowerCase();
    if (!keyword) {
      return { items: state.contacts };
    }

    return {
      items: state.contacts.filter((contact) =>
        [contact.name, contact.email, contact.role, contact.organization, contact.note].join(" ").toLowerCase().includes(keyword),
      ),
    };
  },

  async getSettings() {
    await delay();
    return readMockState().settings;
  },

  async updateSettings(patch) {
    await delay(120);
    return updateMockState((state) => {
      state.settings = {
        ...state.settings,
        ...patch,
      };
      return state;
    }).settings;
  },

  async sendMessage(payload) {
    await delay(200);
    const nextState = updateMockState((state) => {
      const message = {
        id: nextId(),
        folder: "sent",
        previousFolder: "sent",
        sender: state.profile.displayName,
        senderEmail: state.profile.email,
        recipients: payload.recipients,
        avatar: state.profile.avatarInitial,
        role: "悦享用户",
        subject: payload.subject,
        snippet: payload.body.slice(0, 42) || payload.subject,
        time: "刚刚",
        dateTimeLabel: "2026年5月5日 刚刚",
        sortAt: new Date().toISOString(),
        isUnread: false,
        isStarred: false,
        hasAttachment: false,
        tags: ["已发送"],
        isOutgoing: true,
        content: asHtmlContent(payload.body || payload.subject),
        attachments: [],
      };
      state.messages.unshift(message);
      return state;
    });

    return nextState.messages[0];
  },

  async saveDraft(payload) {
    await delay(180);
    const nextState = updateMockState((state) => {
      const message = {
        id: nextId("draft"),
        folder: "drafts",
        previousFolder: "drafts",
        sender: state.profile.displayName,
        senderEmail: state.profile.email,
        recipients: payload.recipients,
        avatar: state.profile.avatarInitial,
        role: "悦享用户",
        subject: payload.subject || "未命名草稿",
        snippet: payload.body.slice(0, 42) || "草稿已保存",
        time: "刚刚",
        dateTimeLabel: "2026年5月5日 刚刚",
        sortAt: new Date().toISOString(),
        isUnread: false,
        isStarred: false,
        hasAttachment: false,
        tags: ["草稿"],
        isOutgoing: true,
        content: asHtmlContent(payload.body || ""),
        attachments: [],
      };
      state.messages.unshift(message);
      return state;
    });

    return nextState.messages[0];
  },

  async sendInvite(payload) {
    await delay(220);
    const nextState = updateMockState((state) => {
      const template = state.templates.find((item) => item.role === payload.role) || state.templates[0];
      const recipients = payload.recipients;
      const message = {
        id: nextId("invite"),
        folder: "sent",
        previousFolder: "sent",
        sender: state.profile.displayName,
        senderEmail: state.profile.email,
        recipients,
        avatar: state.profile.avatarInitial,
        role: "悦享用户",
        subject: template.subject,
        snippet: `已向 ${recipients.length} 位对象发送${template.subject}`,
        time: "刚刚",
        dateTimeLabel: "2026年5月5日 刚刚",
        sortAt: new Date().toISOString(),
        isUnread: false,
        isStarred: false,
        hasAttachment: false,
        tags: ["业务邀请"],
        isOutgoing: true,
        content: template.html,
        attachments: [],
      };
      state.messages.unshift(message);
      return state;
    });

    return nextState.messages[0];
  },
};
