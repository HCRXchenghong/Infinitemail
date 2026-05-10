import {
  POST_OFFICE_API_BASE_PATH,
  POST_OFFICE_API_CONTRACT,
  POST_OFFICE_API_VERSION,
  POST_OFFICE_MAIL_FOLDERS,
  POST_OFFICE_MESSAGE_FILTERS,
  POST_OFFICE_SETTINGS_FIELDS,
  getPostOfficeContractOperation,
} from "@infinitech/contracts/post-office";

export { getPostOfficeContractOperation };

function buildSchemaNameMap() {
  return Object.keys(POST_OFFICE_API_CONTRACT.schemas).reduce((accumulator, name) => {
    accumulator[name] = name;
    return accumulator;
  }, {});
}

function assertSchemaName(name) {
  if (!POST_OFFICE_API_CONTRACT.schemas[name]) {
    throw new Error(`Unknown Post Office contract schema: ${name}`);
  }
  return name;
}

const ALL_MAIL_FOLDER_SET = new Set(POST_OFFICE_MAIL_FOLDERS);
const VISIBLE_MAIL_FOLDERS = POST_OFFICE_MAIL_FOLDERS.filter((folderId) => folderId !== "archive");
const VISIBLE_MAIL_FOLDER_SET = new Set(VISIBLE_MAIL_FOLDERS);
const MESSAGE_FILTER_SET = new Set(POST_OFFICE_MESSAGE_FILTERS);
const SETTINGS_FIELD_SET = new Set(POST_OFFICE_SETTINGS_FIELDS);

export const POST_OFFICE_SCHEMA_NAMES = Object.freeze(buildSchemaNameMap());
export const POST_OFFICE_VISIBLE_MAIL_FOLDERS = Object.freeze(VISIBLE_MAIL_FOLDERS);
export const POST_OFFICE_DEFAULT_FOLDER =
  POST_OFFICE_VISIBLE_MAIL_FOLDERS.find((folderId) => folderId === "inbox") ||
  POST_OFFICE_VISIBLE_MAIL_FOLDERS[0] ||
  "inbox";
export const POST_OFFICE_DEFAULT_FILTER =
  POST_OFFICE_MESSAGE_FILTERS.find((filterId) => filterId === "all") ||
  POST_OFFICE_MESSAGE_FILTERS[0] ||
  "all";
export const POST_OFFICE_SETTINGS_FIELD_SET = SETTINGS_FIELD_SET;

export const POST_OFFICE_OPERATION_SCHEMA_NAMES = Object.freeze({
  getMailboxProfile: Object.freeze({
    request: null,
    responseData: assertSchemaName("MailboxSummaryResponseData"),
  }),
  activateMailbox: Object.freeze({
    request: assertSchemaName("ActivateMailboxRequest"),
    responseData: assertSchemaName("ActivateMailboxResponseData"),
  }),
  listMessages: Object.freeze({
    request: assertSchemaName("ListMessagesQuery"),
    responseData: assertSchemaName("ListMessagesResponseData"),
  }),
  getMessageDetail: Object.freeze({
    request: null,
    responseData: assertSchemaName("Message"),
  }),
  sendMessage: Object.freeze({
    request: assertSchemaName("SendMessageRequest"),
    responseData: assertSchemaName("SendMessageResponseData"),
  }),
  updateMessageStar: Object.freeze({
    request: assertSchemaName("PatchMessageStarRequest"),
    responseData: assertSchemaName("Message"),
  }),
  moveMessage: Object.freeze({
    request: assertSchemaName("MoveMessageRequest"),
    responseData: assertSchemaName("MoveMessageResponseData"),
  }),
  createDraft: Object.freeze({
    request: assertSchemaName("SaveDraftRequest"),
    responseData: assertSchemaName("SaveDraftResponseData"),
  }),
  getDraft: Object.freeze({
    request: null,
    responseData: assertSchemaName("Draft"),
  }),
  updateDraft: Object.freeze({
    request: assertSchemaName("SaveDraftRequest"),
    responseData: assertSchemaName("SaveDraftResponseData"),
  }),
  getSettings: Object.freeze({
    request: null,
    responseData: assertSchemaName("MailboxSettingsResponseData"),
  }),
  updateSettings: Object.freeze({
    request: assertSchemaName("MailboxSettingsPatch"),
    responseData: assertSchemaName("MailboxSettingsResponseData"),
  }),
});

export function isPostOfficeVisibleFolder(folderId) {
  return VISIBLE_MAIL_FOLDER_SET.has(String(folderId || "").trim());
}

export function isPostOfficeFolder(folderId) {
  return ALL_MAIL_FOLDER_SET.has(String(folderId || "").trim());
}

export function normalizePostOfficeFolder(folderId, fallback = POST_OFFICE_DEFAULT_FOLDER) {
  const normalized = String(folderId || "").trim();
  return isPostOfficeVisibleFolder(normalized) ? normalized : fallback;
}

export function isPostOfficeMessageFilter(filterId) {
  return MESSAGE_FILTER_SET.has(String(filterId || "").trim());
}

export function normalizePostOfficeMessageFilter(filterId, fallback = POST_OFFICE_DEFAULT_FILTER) {
  const normalized = String(filterId || "").trim();
  return isPostOfficeMessageFilter(normalized) ? normalized : fallback;
}

export function getPostOfficeOperationSchemaNames(operationId) {
  return POST_OFFICE_OPERATION_SCHEMA_NAMES[operationId] || Object.freeze({
    request: null,
    responseData: null,
  });
}

export function resolvePostOfficeOperationPath(operationId, pathParams = {}) {
  const operation = getPostOfficeContractOperation(operationId);
  if (!operation) {
    throw new Error(`Unknown Post Office contract operation: ${operationId}`);
  }

  return operation.path.replace(/:([a-zA-Z0-9_]+)/g, (_match, paramName) => {
    const rawValue = pathParams[paramName];
    if (rawValue === undefined || rawValue === null || rawValue === "") {
      throw new Error(`Missing path param "${paramName}" for Post Office operation ${operationId}`);
    }
    return encodeURIComponent(String(rawValue));
  });
}

export function buildPostOfficeContractHeaders(operationId) {
  const operation = getPostOfficeContractOperation(operationId);
  if (!operation) {
    throw new Error(`Unknown Post Office contract operation: ${operationId}`);
  }

  const schemaNames = getPostOfficeOperationSchemaNames(operationId);
  const headers = {
    "X-Post-Office-Contract": POST_OFFICE_API_CONTRACT.name,
    "X-Post-Office-Contract-Version": POST_OFFICE_API_VERSION,
    "X-Post-Office-Base-Path": POST_OFFICE_API_BASE_PATH,
    "X-Post-Office-Operation-Id": operation.operationId,
    "X-Post-Office-Operation-Method": operation.method,
    "X-Post-Office-Operation-Tag": operation.tag,
  };

  if (schemaNames.request) {
    headers["X-Post-Office-Request-Schema"] = schemaNames.request;
  }

  if (schemaNames.responseData) {
    headers["X-Post-Office-Response-Schema"] = schemaNames.responseData;
  }

  return headers;
}

export function pickPostOfficeSettingsPatch(source) {
  const patch = {};
  if (!source || typeof source !== "object" || Array.isArray(source)) {
    return patch;
  }

  for (const field of POST_OFFICE_SETTINGS_FIELDS) {
    if (Object.prototype.hasOwnProperty.call(source, field)) {
      patch[field] = source[field];
    }
  }

  return patch;
}
