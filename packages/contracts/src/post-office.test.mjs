import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

import {
  POST_OFFICE_API_BASE_PATH,
  POST_OFFICE_API_CONTRACT,
  POST_OFFICE_API_PATHS,
  POST_OFFICE_MAIL_FOLDERS,
  POST_OFFICE_MESSAGE_FILTERS,
  POST_OFFICE_MAILBOX_PROVISIONING_STATUSES,
  POST_OFFICE_SETTINGS_FIELDS,
  getPostOfficeContractOperation,
  listPostOfficeContractOperations,
} from "./post-office.js";

const require = createRequire(import.meta.url);
const cjsPostOffice = require("./post-office.cjs");

test("post office contract publishes the standalone mailbox API surface", () => {
  assert.equal(POST_OFFICE_API_BASE_PATH, "/api/v1/post-office");
  assert.equal(POST_OFFICE_API_CONTRACT.basePath, "/api/v1/post-office");
  assert.deepEqual(POST_OFFICE_MAIL_FOLDERS, [
    "inbox",
    "starred",
    "sent",
    "drafts",
    "trash",
    "archive",
  ]);
  assert.deepEqual(POST_OFFICE_MESSAGE_FILTERS, [
    "all",
    "unread",
    "important",
    "attachment",
  ]);
  assert.deepEqual(POST_OFFICE_MAILBOX_PROVISIONING_STATUSES, [
    "pending_config",
    "queued",
    "failed",
    "provisioned",
  ]);
  assert.deepEqual(POST_OFFICE_SETTINGS_FIELDS, [
    "defaultSenderName",
    "signature",
    "autoReplyEnabled",
    "autoReplyMessage",
  ]);
});

test("post office contract operations expose formal request and response schemas", () => {
  assert.equal(getPostOfficeContractOperation("activateMailbox")?.path, "/mailboxes/activate");
  assert.equal(getPostOfficeContractOperation("listMessages")?.querySchema.properties.folderId.default, "inbox");
  assert.equal(POST_OFFICE_API_PATHS.sendMessage.requestSchema.required.includes("body"), true);
  assert.equal(POST_OFFICE_API_PATHS.createDraft.path, "/drafts");
  assert.equal(POST_OFFICE_API_PATHS.updateSettings.requestSchema.additionalProperties, false);
  assert.equal(listPostOfficeContractOperations("messages").length, 6);
});

test("post office CJS bridge stays aligned with ESM exports", () => {
  assert.equal(cjsPostOffice.POST_OFFICE_API_BASE_PATH, POST_OFFICE_API_BASE_PATH);
  assert.deepEqual(cjsPostOffice.POST_OFFICE_MAIL_FOLDERS, POST_OFFICE_MAIL_FOLDERS);
  assert.equal(
    cjsPostOffice.getPostOfficeContractOperation("getSettings")?.path,
    POST_OFFICE_API_PATHS.getSettings.path,
  );
  assert.equal(
    cjsPostOffice.listPostOfficeContractOperations("drafts").length,
    listPostOfficeContractOperations("drafts").length,
  );
});
