import { HTTP_ENVELOPE_CODES } from "./http.js";

function deepFreeze(value) {
  if (!value || typeof value !== "object" || Object.isFrozen(value)) {
    return value;
  }

  for (const entry of Object.values(value)) {
    deepFreeze(entry);
  }
  return Object.freeze(value);
}

function stringSchema(description, extra = {}) {
  return {
    type: "string",
    description,
    ...extra,
  };
}

function booleanSchema(description, extra = {}) {
  return {
    type: "boolean",
    description,
    ...extra,
  };
}

function numberSchema(description, extra = {}) {
  return {
    type: "number",
    description,
    ...extra,
  };
}

function enumSchema(values, description, extra = {}) {
  return {
    type: "string",
    enum: [...values],
    description,
    ...extra,
  };
}

function arraySchema(items, description, extra = {}) {
  return {
    type: "array",
    items,
    description,
    ...extra,
  };
}

function objectSchema(properties, required = [], description = "", extra = {}) {
  return {
    type: "object",
    description,
    additionalProperties: false,
    properties,
    required,
    ...extra,
  };
}

function nullableSchema(schema, description = schema?.description || "") {
  return {
    anyOf: [schema, { type: "null" }],
    description,
  };
}

function partialObjectSchema(baseSchema, description = "") {
  return {
    ...baseSchema,
    description: description || baseSchema.description || "",
    required: [],
  };
}

function buildEnvelopeSchema(dataSchema, message) {
  return objectSchema(
    {
      request_id: stringSchema("Standardized request trace id", {
        minLength: 1,
        maxLength: 128,
      }),
      code: enumSchema(
        HTTP_ENVELOPE_CODES,
        "Shared envelope response code",
      ),
      message: stringSchema("Standardized response message", {
        minLength: 1,
        maxLength: 200,
        example: message,
      }),
      data: dataSchema,
      success: booleanSchema("Whether the request completed successfully"),
    },
    ["request_id", "code", "message", "data", "success"],
    "Shared HTTP response envelope used across Yuexiang platform APIs",
  );
}

const PRESENTATION_EMAIL_SCHEMA = stringSchema("Email address or mailbox identity presented to the client", {
  minLength: 1,
  maxLength: 160,
});

const TIMESTAMP_SCHEMA = stringSchema("RFC3339 timestamp", {
  format: "date-time",
  minLength: 20,
  maxLength: 40,
});

const DISPLAY_TIME_SCHEMA = stringSchema("Localized display time already formatted for the client UI", {
  minLength: 1,
  maxLength: 32,
});

const IDENTIFIER_SCHEMA = stringSchema("Stable business identifier", {
  minLength: 1,
  maxLength: 128,
});

const TEXTAREA_SCHEMA = stringSchema("Long text field", {
  minLength: 0,
  maxLength: 20000,
});

const HTML_BODY_SCHEMA = stringSchema("HTML email body or rendered rich content", {
  minLength: 0,
  maxLength: 200000,
});

const POST_OFFICE_API_VERSION = "2026-05-05";
const POST_OFFICE_API_BASE_PATH = "/api/v1/post-office";
const POST_OFFICE_MAIL_FOLDERS = deepFreeze([
  "inbox",
  "starred",
  "sent",
  "drafts",
  "trash",
  "archive",
]);
const POST_OFFICE_MESSAGE_FILTERS = deepFreeze([
  "all",
  "unread",
  "important",
  "attachment",
]);
const POST_OFFICE_MESSAGE_BODY_FORMATS = deepFreeze([
  "text",
  "html",
  "both",
]);
const POST_OFFICE_MESSAGE_SOURCES = deepFreeze([
  "mailbox",
  "draft",
  "invite",
  "workflow",
  "system",
  "imported",
]);
const POST_OFFICE_MESSAGE_DELIVERY_STATUSES = deepFreeze([
  "draft",
  "queued",
  "accepted",
  "sent",
  "delivered",
  "failed",
  "received",
]);
const POST_OFFICE_MAILBOX_PROVISIONING_STATUSES = deepFreeze([
  "pending",
  "active",
  "recycling",
  "reclaimed",
]);
const POST_OFFICE_SETTINGS_FIELDS = deepFreeze([
  "defaultSenderName",
  "signature",
  "autoReplyEnabled",
  "autoReplyMessage",
]);

const PostOfficeMailboxProfileSchema = objectSchema(
  {
    id: IDENTIFIER_SCHEMA,
    displayName: stringSchema("Display name shown in mailbox UI", {
      minLength: 1,
      maxLength: 80,
    }),
    avatarInitial: stringSchema("Single-character avatar fallback used in the client UI", {
      minLength: 1,
      maxLength: 4,
    }),
    unifiedAccountPhone: stringSchema("Masked unified account phone shown in settings", {
      minLength: 0,
      maxLength: 32,
    }),
    rolePrefix: stringSchema("Immutable mailbox local-part prefix derived from platform role", {
      minLength: 1,
      maxLength: 32,
      pattern: "^[a-z][a-z0-9-]*-$",
    }),
    emailPrefix: stringSchema("Chosen mailbox local-part suffix owned by the user", {
      minLength: 0,
      maxLength: 48,
      pattern: "^[a-z0-9-]*$",
    }),
    email: stringSchema("Provisioned mailbox address under the platform domain", {
      minLength: 0,
      maxLength: 160,
    }),
    mailboxDomain: stringSchema("Platform-controlled mailbox domain", {
      minLength: 1,
      maxLength: 120,
    }),
    mailboxProvisioned: booleanSchema("Whether the mailbox has already been activated"),
    provisioningStatus: enumSchema(
      POST_OFFICE_MAILBOX_PROVISIONING_STATUSES,
      "Lifecycle status of the mailbox account",
    ),
    authMode: enumSchema(
      ["user", "merchant", "rider"],
      "Unified principal type currently bound to this mailbox",
    ),
    sourceUserId: IDENTIFIER_SCHEMA,
    createdAt: nullableSchema(TIMESTAMP_SCHEMA, "Mailbox record creation time"),
    updatedAt: nullableSchema(TIMESTAMP_SCHEMA, "Mailbox record last update time"),
    provisionedAt: nullableSchema(TIMESTAMP_SCHEMA, "Mailbox activation completion time"),
  },
  [
    "id",
    "displayName",
    "avatarInitial",
    "unifiedAccountPhone",
    "rolePrefix",
    "emailPrefix",
    "email",
    "mailboxDomain",
    "mailboxProvisioned",
    "provisioningStatus",
    "authMode",
    "sourceUserId",
  ],
  "Client-facing mailbox profile returned by the standalone Post Office service",
);

const PostOfficeMailboxCapabilitySchema = objectSchema(
  {
    supportsRichHtml: booleanSchema("Whether HTML compose and HTML rendering are supported"),
    supportsAttachments: booleanSchema("Whether upload-backed file attachments are supported"),
    supportsAutoReply: booleanSchema("Whether auto-reply settings are available"),
    supportsInviteTemplates: booleanSchema("Whether invite template generation is available"),
    recyclesOnAccountClosure: booleanSchema("Whether mailbox data is reclaimed when the unified account is closed"),
  },
  [
    "supportsRichHtml",
    "supportsAttachments",
    "supportsAutoReply",
    "supportsInviteTemplates",
    "recyclesOnAccountClosure",
  ],
  "Capability flags that let the client decide which mailbox features can be enabled",
);

const PostOfficeFolderCountsSchema = objectSchema(
  {
    inbox: numberSchema("Unread counter for inbox navigation badge", { minimum: 0 }),
    starred: numberSchema("Count of starred messages across non-trash folders", { minimum: 0 }),
    sent: numberSchema("Count of sent messages", { minimum: 0 }),
    drafts: numberSchema("Count of draft messages", { minimum: 0 }),
    trash: numberSchema("Count of trash messages", { minimum: 0 }),
  },
  ["inbox", "starred", "sent", "drafts", "trash"],
  "Client sidebar counters",
);

const PostOfficeAttachmentSchema = objectSchema(
  {
    id: IDENTIFIER_SCHEMA,
    assetId: nullableSchema(IDENTIFIER_SCHEMA, "Opaque asset reference managed by the mail service"),
    name: stringSchema("Attachment display name", {
      minLength: 1,
      maxLength: 255,
    }),
    type: stringSchema("Client-facing file type label", {
      minLength: 1,
      maxLength: 64,
    }),
    contentType: nullableSchema(stringSchema("MIME type for downstream processing", {
      minLength: 1,
      maxLength: 128,
    })),
    sizeBytes: nullableSchema(numberSchema("Raw attachment size in bytes", {
      minimum: 0,
    })),
    sizeLabel: stringSchema("Human-readable attachment size string", {
      minLength: 1,
      maxLength: 32,
    }),
    downloadUrl: nullableSchema(stringSchema("Authenticated download url or pre-signed asset link", {
      minLength: 1,
      maxLength: 500,
    })),
    previewUrl: nullableSchema(stringSchema("Optional inline preview url", {
      minLength: 1,
      maxLength: 500,
    })),
  },
  ["id", "name", "type", "sizeLabel"],
  "File attachment metadata rendered inside message detail view",
);

const PostOfficeMessageSchema = objectSchema(
  {
    id: IDENTIFIER_SCHEMA,
    threadId: nullableSchema(IDENTIFIER_SCHEMA, "Conversation or thread id if threading is enabled"),
    folder: enumSchema(POST_OFFICE_MAIL_FOLDERS, "Current folder containing this message"),
    previousFolder: nullableSchema(enumSchema(POST_OFFICE_MAIL_FOLDERS, "Previous folder before the latest move action")),
    sender: stringSchema("Display sender name used in list and detail UIs", {
      minLength: 1,
      maxLength: 120,
    }),
    senderEmail: PRESENTATION_EMAIL_SCHEMA,
    recipients: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Primary recipient list", {
      minItems: 0,
      maxItems: 100,
    }),
    cc: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Carbon-copy recipient list", {
      minItems: 0,
      maxItems: 100,
    }),
    bcc: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Blind-carbon-copy recipient list", {
      minItems: 0,
      maxItems: 100,
    }),
    avatar: stringSchema("Avatar glyph or upstream avatar text fallback", {
      minLength: 1,
      maxLength: 8,
    }),
    role: stringSchema("Sender role label displayed in badges", {
      minLength: 1,
      maxLength: 40,
    }),
    subject: stringSchema("Message subject line", {
      minLength: 1,
      maxLength: 240,
    }),
    snippet: stringSchema("Plain-text preview used in the message list", {
      minLength: 0,
      maxLength: 500,
    }),
    time: DISPLAY_TIME_SCHEMA,
    dateTimeLabel: DISPLAY_TIME_SCHEMA,
    sortAt: TIMESTAMP_SCHEMA,
    sentAt: nullableSchema(TIMESTAMP_SCHEMA, "Message send timestamp"),
    receivedAt: nullableSchema(TIMESTAMP_SCHEMA, "Message receive timestamp"),
    isUnread: booleanSchema("Unread state for current principal"),
    isStarred: booleanSchema("Starred marker stored by the mailbox service"),
    hasAttachment: booleanSchema("Whether at least one attachment exists"),
    tags: arraySchema(stringSchema("Client-visible tag label", {
      minLength: 1,
      maxLength: 40,
    }), "Tag list rendered as badges", {
      minItems: 0,
      maxItems: 20,
    }),
    isOutgoing: booleanSchema("Whether the current principal originated this message"),
    content: HTML_BODY_SCHEMA,
    attachments: arraySchema(PostOfficeAttachmentSchema, "Attachment list", {
      minItems: 0,
      maxItems: 50,
    }),
    source: enumSchema(POST_OFFICE_MESSAGE_SOURCES, "Source channel that created this mail object"),
    deliveryStatus: enumSchema(
      POST_OFFICE_MESSAGE_DELIVERY_STATUSES,
      "Delivery lifecycle state",
    ),
    replyToMessageId: nullableSchema(IDENTIFIER_SCHEMA, "Reply/forward source message id"),
    meta: {
      type: "object",
      description: "Forward-compatible metadata bucket for service-side extensions",
      additionalProperties: true,
    },
  },
  [
    "id",
    "folder",
    "sender",
    "senderEmail",
    "recipients",
    "avatar",
    "role",
    "subject",
    "snippet",
    "time",
    "dateTimeLabel",
    "sortAt",
    "isUnread",
    "isStarred",
    "hasAttachment",
    "tags",
    "isOutgoing",
    "content",
    "attachments",
    "source",
    "deliveryStatus",
  ],
  "Client-ready mail object shared by list, detail, sent, draft and invite flows",
);

const PostOfficeBodyPayloadSchema = objectSchema(
  {
    format: enumSchema(
      POST_OFFICE_MESSAGE_BODY_FORMATS,
      "Body payload format negotiated between the editor and the mail service",
    ),
    text: nullableSchema(TEXTAREA_SCHEMA, "Plain text body"),
    html: nullableSchema(HTML_BODY_SCHEMA, "HTML body for rich presentation"),
  },
  ["format"],
  "Composable message body payload",
);

const PostOfficeMailboxSettingsSchema = objectSchema(
  {
    defaultSenderName: stringSchema("Default sender display name applied to new outgoing messages", {
      minLength: 1,
      maxLength: 80,
    }),
    signature: TEXTAREA_SCHEMA,
    autoReplyEnabled: booleanSchema("Whether automatic out-of-office replies are enabled"),
    autoReplyMessage: TEXTAREA_SCHEMA,
    updatedAt: nullableSchema(TIMESTAMP_SCHEMA, "Last settings update time"),
  },
  ["defaultSenderName", "signature", "autoReplyEnabled", "autoReplyMessage"],
  "Mailbox settings surfaced in the standalone client settings page",
);

const PostOfficeDraftSchema = objectSchema(
  {
    ...PostOfficeMessageSchema.properties,
    draftVersion: numberSchema("Monotonic draft version for optimistic updates", {
      minimum: 1,
    }),
    autosavedAt: nullableSchema(TIMESTAMP_SCHEMA, "Most recent autosave timestamp"),
  },
  [...PostOfficeMessageSchema.required, "draftVersion"],
  "Draft payload shape shared by manual save and autosave flows",
);

const PostOfficeActivateMailboxRequestSchema = objectSchema(
  {
    emailPrefix: stringSchema("Chosen immutable mailbox local-part suffix", {
      minLength: 2,
      maxLength: 48,
      pattern: "^[a-z0-9-]+$",
    }),
  },
  ["emailPrefix"],
  "Request body for first-time mailbox activation",
);

const PostOfficeActivateMailboxResponseDataSchema = objectSchema(
  {
    mailbox: PostOfficeMailboxProfileSchema,
    capabilities: PostOfficeMailboxCapabilitySchema,
  },
  ["mailbox", "capabilities"],
  "Activation response data",
);

const PostOfficeMailboxSummaryResponseDataSchema = objectSchema(
  {
    profile: PostOfficeMailboxProfileSchema,
    capabilities: PostOfficeMailboxCapabilitySchema,
    folderCounts: PostOfficeFolderCountsSchema,
  },
  ["profile", "capabilities", "folderCounts"],
  "Mailbox profile bootstrap data",
);

const PostOfficeListMessagesQuerySchema = objectSchema(
  {
    folderId: enumSchema(POST_OFFICE_MAIL_FOLDERS, "Target folder to list", {
      default: "inbox",
    }),
    filter: enumSchema(POST_OFFICE_MESSAGE_FILTERS, "Optional list filter", {
      default: "all",
    }),
    search: stringSchema("Full-text search term across sender, subject and snippet", {
      minLength: 0,
      maxLength: 120,
    }),
    cursor: nullableSchema(stringSchema("Opaque pagination cursor", {
      minLength: 1,
      maxLength: 256,
    })),
    limit: numberSchema("Page size for mailbox listing", {
      minimum: 1,
      maximum: 100,
      default: 50,
    }),
  },
  [],
  "Query parameters accepted by message listing endpoint",
);

const PostOfficeListMessagesResponseDataSchema = objectSchema(
  {
    items: arraySchema(PostOfficeMessageSchema, "Message list items", {
      minItems: 0,
      maxItems: 100,
    }),
    folderCounts: PostOfficeFolderCountsSchema,
    nextCursor: nullableSchema(stringSchema("Opaque next-page cursor", {
      minLength: 1,
      maxLength: 256,
    })),
    hasMore: booleanSchema("Whether additional pages can be fetched"),
  },
  ["items", "folderCounts", "nextCursor", "hasMore"],
  "Message listing response data",
);

const PostOfficeSendMessageRequestSchema = objectSchema(
  {
    recipients: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Primary recipients", {
      minItems: 1,
      maxItems: 100,
    }),
    cc: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Carbon-copy recipients", {
      minItems: 0,
      maxItems: 100,
    }),
    bcc: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Blind-carbon-copy recipients", {
      minItems: 0,
      maxItems: 100,
    }),
    subject: stringSchema("Message subject line", {
      minLength: 1,
      maxLength: 240,
    }),
    body: PostOfficeBodyPayloadSchema,
    attachments: arraySchema(PostOfficeAttachmentSchema, "Attachment references", {
      minItems: 0,
      maxItems: 50,
    }),
    templateId: nullableSchema(IDENTIFIER_SCHEMA, "Invite template or reusable composition template id"),
    replyToMessageId: nullableSchema(IDENTIFIER_SCHEMA, "Message being replied to or forwarded"),
    source: enumSchema(
      ["manual", "invite", "workflow", "system"],
      "Origin of the outgoing action",
    ),
  },
  ["recipients", "subject", "body", "attachments", "source"],
  "Request body for sending a final outbound email",
);

const PostOfficeSendMessageResponseDataSchema = objectSchema(
  {
    message: PostOfficeMessageSchema,
    acceptedAt: TIMESTAMP_SCHEMA,
    providerMessageId: nullableSchema(IDENTIFIER_SCHEMA, "Downstream provider acceptance id"),
  },
  ["message", "acceptedAt", "providerMessageId"],
  "Send response data after the mail service accepts the outbound message",
);

const PostOfficePatchStarRequestSchema = objectSchema(
  {
    starred: booleanSchema("Explicit target state for the star marker"),
  },
  ["starred"],
  "Request body for starring or unstarring a message",
);

const PostOfficeMoveMessageRequestSchema = objectSchema(
  {
    targetFolder: enumSchema(
      POST_OFFICE_MAIL_FOLDERS.filter((item) => item !== "starred"),
      "Folder to move the message into",
    ),
  },
  ["targetFolder"],
  "Request body for moving a message between mailbox folders",
);

const PostOfficeMoveMessageResponseDataSchema = objectSchema(
  {
    messageId: IDENTIFIER_SCHEMA,
    previousFolder: nullableSchema(enumSchema(POST_OFFICE_MAIL_FOLDERS, "Previous folder")),
    folder: enumSchema(POST_OFFICE_MAIL_FOLDERS, "Current folder"),
    movedAt: TIMESTAMP_SCHEMA,
  },
  ["messageId", "previousFolder", "folder", "movedAt"],
  "Move result payload",
);

const PostOfficeSaveDraftRequestSchema = objectSchema(
  {
    draftId: nullableSchema(IDENTIFIER_SCHEMA, "Existing draft id for idempotent overwrite"),
    recipients: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Draft recipients", {
      minItems: 0,
      maxItems: 100,
    }),
    cc: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Draft cc recipients", {
      minItems: 0,
      maxItems: 100,
    }),
    bcc: arraySchema(PRESENTATION_EMAIL_SCHEMA, "Draft bcc recipients", {
      minItems: 0,
      maxItems: 100,
    }),
    subject: stringSchema("Draft subject line", {
      minLength: 0,
      maxLength: 240,
    }),
    body: PostOfficeBodyPayloadSchema,
    attachments: arraySchema(PostOfficeAttachmentSchema, "Draft attachments", {
      minItems: 0,
      maxItems: 50,
    }),
    autosave: booleanSchema("Whether the save request comes from autosave"),
  },
  ["recipients", "subject", "body", "attachments", "autosave"],
  "Request body for creating or updating a draft",
);

const PostOfficeSaveDraftResponseDataSchema = objectSchema(
  {
    draft: PostOfficeDraftSchema,
  },
  ["draft"],
  "Draft save response data",
);

const PostOfficeMailboxSettingsPatchSchema = partialObjectSchema(
  objectSchema(
    {
      defaultSenderName: PostOfficeMailboxSettingsSchema.properties.defaultSenderName,
      signature: PostOfficeMailboxSettingsSchema.properties.signature,
      autoReplyEnabled: PostOfficeMailboxSettingsSchema.properties.autoReplyEnabled,
      autoReplyMessage: PostOfficeMailboxSettingsSchema.properties.autoReplyMessage,
    },
    [],
    "Patch payload for mailbox settings",
  ),
  "Patch payload for mailbox settings",
);

const PostOfficeMailboxSettingsResponseDataSchema = objectSchema(
  {
    settings: PostOfficeMailboxSettingsSchema,
  },
  ["settings"],
  "Mailbox settings payload",
);

const POST_OFFICE_PATH_PARAM_SCHEMAS = deepFreeze({
  messageId: IDENTIFIER_SCHEMA,
  draftId: IDENTIFIER_SCHEMA,
});

const POST_OFFICE_API_PATHS = deepFreeze({
  getMailboxProfile: {
    tag: "mailboxes",
    method: "GET",
    path: "/mailboxes/me",
    operationId: "getMailboxProfile",
    summary: "Fetch mailbox profile, capabilities and sidebar counters for the current unified principal",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    responseSchema: buildEnvelopeSchema(
      PostOfficeMailboxSummaryResponseDataSchema,
      "Mailbox profile loaded",
    ),
  },
  activateMailbox: {
    tag: "mailboxes",
    method: "POST",
    path: "/mailboxes/activate",
    operationId: "activateMailbox",
    summary: "Activate the caller's first mailbox under the platform-owned domain",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    requestSchema: PostOfficeActivateMailboxRequestSchema,
    responseSchema: buildEnvelopeSchema(
      PostOfficeActivateMailboxResponseDataSchema,
      "Mailbox activated",
    ),
  },
  listMessages: {
    tag: "messages",
    method: "GET",
    path: "/messages",
    operationId: "listMessages",
    summary: "List mailbox messages using folder, filter and search criteria",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    querySchema: PostOfficeListMessagesQuerySchema,
    responseSchema: buildEnvelopeSchema(
      PostOfficeListMessagesResponseDataSchema,
      "Messages loaded",
    ),
  },
  getMessageDetail: {
    tag: "messages",
    method: "GET",
    path: "/messages/:messageId",
    operationId: "getMessageDetail",
    summary: "Load a single message with full rendered content and attachments",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    pathParams: { messageId: POST_OFFICE_PATH_PARAM_SCHEMAS.messageId },
    responseSchema: buildEnvelopeSchema(
      objectSchema({ message: PostOfficeMessageSchema }, ["message"], "Message detail response"),
      "Message loaded",
    ),
  },
  sendMessage: {
    tag: "messages",
    method: "POST",
    path: "/messages/send",
    operationId: "sendMessage",
    summary: "Send a rich text or HTML email through the standalone Post Office service",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    requestSchema: PostOfficeSendMessageRequestSchema,
    responseSchema: buildEnvelopeSchema(
      PostOfficeSendMessageResponseDataSchema,
      "Message accepted for delivery",
    ),
  },
  updateMessageStar: {
    tag: "messages",
    method: "PATCH",
    path: "/messages/:messageId/star",
    operationId: "updateMessageStar",
    summary: "Explicitly set or clear the star marker for a message",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    pathParams: { messageId: POST_OFFICE_PATH_PARAM_SCHEMAS.messageId },
    requestSchema: PostOfficePatchStarRequestSchema,
    responseSchema: buildEnvelopeSchema(
      objectSchema(
        {
          messageId: IDENTIFIER_SCHEMA,
          starred: booleanSchema("Resulting star state"),
          updatedAt: TIMESTAMP_SCHEMA,
        },
        ["messageId", "starred", "updatedAt"],
        "Star update result payload",
      ),
      "Message star updated",
    ),
  },
  moveMessage: {
    tag: "messages",
    method: "POST",
    path: "/messages/:messageId/move",
    operationId: "moveMessage",
    summary: "Move a message into another mailbox folder",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    pathParams: { messageId: POST_OFFICE_PATH_PARAM_SCHEMAS.messageId },
    requestSchema: PostOfficeMoveMessageRequestSchema,
    responseSchema: buildEnvelopeSchema(
      PostOfficeMoveMessageResponseDataSchema,
      "Message moved",
    ),
  },
  createDraft: {
    tag: "drafts",
    method: "POST",
    path: "/drafts",
    operationId: "createDraft",
    summary: "Create a new draft or overwrite an existing draft using an idempotent request body",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    requestSchema: PostOfficeSaveDraftRequestSchema,
    responseSchema: buildEnvelopeSchema(
      PostOfficeSaveDraftResponseDataSchema,
      "Draft saved",
    ),
  },
  getDraft: {
    tag: "drafts",
    method: "GET",
    path: "/drafts/:draftId",
    operationId: "getDraft",
    summary: "Load a single draft by id for compose resume flows",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    pathParams: { draftId: POST_OFFICE_PATH_PARAM_SCHEMAS.draftId },
    responseSchema: buildEnvelopeSchema(
      objectSchema({ draft: PostOfficeDraftSchema }, ["draft"], "Draft detail response"),
      "Draft loaded",
    ),
  },
  updateDraft: {
    tag: "drafts",
    method: "PUT",
    path: "/drafts/:draftId",
    operationId: "updateDraft",
    summary: "Update an existing draft with optimistic versioning semantics",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    pathParams: { draftId: POST_OFFICE_PATH_PARAM_SCHEMAS.draftId },
    requestSchema: PostOfficeSaveDraftRequestSchema,
    responseSchema: buildEnvelopeSchema(
      PostOfficeSaveDraftResponseDataSchema,
      "Draft updated",
    ),
  },
  getSettings: {
    tag: "settings",
    method: "GET",
    path: "/settings",
    operationId: "getSettings",
    summary: "Fetch mailbox settings for the current unified principal",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    responseSchema: buildEnvelopeSchema(
      PostOfficeMailboxSettingsResponseDataSchema,
      "Mailbox settings loaded",
    ),
  },
  updateSettings: {
    tag: "settings",
    method: "PUT",
    path: "/settings",
    operationId: "updateSettings",
    summary: "Patch mailbox settings without replacing unspecified fields",
    auth: {
      type: "bearer",
      tokenKinds: ["access"],
      principalTypes: ["user", "merchant", "rider"],
    },
    requestSchema: PostOfficeMailboxSettingsPatchSchema,
    responseSchema: buildEnvelopeSchema(
      PostOfficeMailboxSettingsResponseDataSchema,
      "Mailbox settings updated",
    ),
  },
});

const POST_OFFICE_SCHEMA_COMPONENTS = deepFreeze({
  MailboxProfile: PostOfficeMailboxProfileSchema,
  MailboxCapability: PostOfficeMailboxCapabilitySchema,
  FolderCounts: PostOfficeFolderCountsSchema,
  Attachment: PostOfficeAttachmentSchema,
  Message: PostOfficeMessageSchema,
  Draft: PostOfficeDraftSchema,
  BodyPayload: PostOfficeBodyPayloadSchema,
  MailboxSettings: PostOfficeMailboxSettingsSchema,
  ActivateMailboxRequest: PostOfficeActivateMailboxRequestSchema,
  ActivateMailboxResponseData: PostOfficeActivateMailboxResponseDataSchema,
  MailboxSummaryResponseData: PostOfficeMailboxSummaryResponseDataSchema,
  ListMessagesQuery: PostOfficeListMessagesQuerySchema,
  ListMessagesResponseData: PostOfficeListMessagesResponseDataSchema,
  SendMessageRequest: PostOfficeSendMessageRequestSchema,
  SendMessageResponseData: PostOfficeSendMessageResponseDataSchema,
  PatchMessageStarRequest: PostOfficePatchStarRequestSchema,
  MoveMessageRequest: PostOfficeMoveMessageRequestSchema,
  MoveMessageResponseData: PostOfficeMoveMessageResponseDataSchema,
  SaveDraftRequest: PostOfficeSaveDraftRequestSchema,
  SaveDraftResponseData: PostOfficeSaveDraftResponseDataSchema,
  MailboxSettingsPatch: PostOfficeMailboxSettingsPatchSchema,
  MailboxSettingsResponseData: PostOfficeMailboxSettingsResponseDataSchema,
});

const POST_OFFICE_API_CONTRACT = deepFreeze({
  name: "yuexiang-post-office",
  version: POST_OFFICE_API_VERSION,
  basePath: POST_OFFICE_API_BASE_PATH,
  transport: {
    requestEncoding: "application/json",
    responseEnvelope: "shared-http-envelope",
  },
  tags: ["mailboxes", "messages", "drafts", "settings"],
  schemas: POST_OFFICE_SCHEMA_COMPONENTS,
  paths: POST_OFFICE_API_PATHS,
});

function getPostOfficeContractOperation(operationId) {
  return POST_OFFICE_API_PATHS[operationId] || null;
}

function listPostOfficeContractOperations(tag = "") {
  const normalizedTag = String(tag || "").trim();
  const operations = Object.values(POST_OFFICE_API_PATHS);
  if (!normalizedTag) {
    return operations;
  }
  return operations.filter((item) => item.tag === normalizedTag);
}

export {
  POST_OFFICE_API_VERSION,
  POST_OFFICE_API_BASE_PATH,
  POST_OFFICE_MAIL_FOLDERS,
  POST_OFFICE_MESSAGE_FILTERS,
  POST_OFFICE_MESSAGE_BODY_FORMATS,
  POST_OFFICE_MESSAGE_SOURCES,
  POST_OFFICE_MESSAGE_DELIVERY_STATUSES,
  POST_OFFICE_MAILBOX_PROVISIONING_STATUSES,
  POST_OFFICE_SETTINGS_FIELDS,
  POST_OFFICE_PATH_PARAM_SCHEMAS,
  POST_OFFICE_SCHEMA_COMPONENTS,
  POST_OFFICE_API_PATHS,
  POST_OFFICE_API_CONTRACT,
  getPostOfficeContractOperation,
  listPostOfficeContractOperations,
};
