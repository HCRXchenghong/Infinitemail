import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

import {
  UPLOAD_DOMAINS,
  isUploadDomain,
  normalizeUploadDomain,
} from "./upload.js";

const require = createRequire(import.meta.url);
const cjsUpload = require("./upload.cjs");

test("upload domains expose shared normalized values", () => {
  assert.equal(UPLOAD_DOMAINS.AFTER_SALES_EVIDENCE, "after_sales_evidence");
  assert.equal(UPLOAD_DOMAINS.CHAT_ATTACHMENT, "chat_attachment");
  assert.equal(UPLOAD_DOMAINS.APP_PACKAGE, "app_package");
  assert.equal(UPLOAD_DOMAINS.ONBOARDING_DOCUMENT, "onboarding_document");
  assert.equal(UPLOAD_DOMAINS.ADMIN_ASSET, "admin_asset");
});

test("upload domain normalization trims and lowercases supported values", () => {
  assert.equal(
    normalizeUploadDomain("  PROFILE_IMAGE  "),
    UPLOAD_DOMAINS.PROFILE_IMAGE,
  );
  assert.equal(
    normalizeUploadDomain("", UPLOAD_DOMAINS.REVIEW_MEDIA),
    UPLOAD_DOMAINS.REVIEW_MEDIA,
  );
});

test("upload domain helpers reject unsupported values", () => {
  assert.equal(isUploadDomain("chat_attachment"), true);
  assert.equal(isUploadDomain("CHAT_ATTACHMENT"), true);
  assert.equal(isUploadDomain("unknown_domain"), false);
  assert.throws(
    () => normalizeUploadDomain("unknown_domain"),
    /Unsupported upload domain/,
  );
});

test("upload contracts keep CommonJS bridge aligned with ESM exports", () => {
  assert.deepEqual(cjsUpload.UPLOAD_DOMAINS, UPLOAD_DOMAINS);
  assert.equal(
    cjsUpload.normalizeUploadDomain(" ONBOARDING_DOCUMENT "),
    UPLOAD_DOMAINS.ONBOARDING_DOCUMENT,
  );
  assert.equal(cjsUpload.isUploadDomain("admin_asset"), true);
});
