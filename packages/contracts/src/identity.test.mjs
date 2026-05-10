import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

import {
  decodeBase64UrlToJSON,
  extractUnifiedPrincipalIdentity,
  normalizeBearerToken,
  normalizePrincipalType,
  parseUnifiedTokenPayload,
  UnifiedTokenKinds,
} from "./identity.js";

const require = createRequire(import.meta.url);
const cjsIdentity = require("./identity.cjs");

function encodeBase64Url(value) {
  return Buffer.from(value)
    .toString("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/g, "");
}

test("identity contracts normalize unified token helpers deterministically", () => {
  const payload = {
    principal_type: "admin",
    principal_id: "admin_uid_18",
    principal_legacy_id: 18,
    role: "super_admin",
    session_id: "admin_session_18",
    token_kind: "access",
  };
  const encodedPayload = encodeBase64Url(JSON.stringify(payload));
  const token = `${encodedPayload}.signature`;

  assert.equal(normalizeBearerToken(`Bearer ${token}`), token);
  assert.equal(normalizePrincipalType(" Merchant "), "merchant");
  assert.deepEqual(decodeBase64UrlToJSON(encodedPayload), payload);
  assert.deepEqual(parseUnifiedTokenPayload(token), payload);
  assert.deepEqual(extractUnifiedPrincipalIdentity(payload, { normalizeType: true }), {
    principalType: "admin",
    principalId: "admin_uid_18",
    legacyId: "18",
    role: "super_admin",
    sessionId: "admin_session_18",
    tokenKind: "access",
    phone: "",
    name: "",
  });
});

test("identity contracts can require standardized claims without legacy aliases", () => {
  const payload = {
    sub: "merchant_uid_9",
    principal_type: "merchant",
    principal_id: "merchant_uid_9",
    role: "merchant",
    session_id: "merchant_session_9",
    token_kind: "access",
    id: "legacy-merchant-id",
    userId: 9,
    type: "refresh",
  };

  assert.deepEqual(
    extractUnifiedPrincipalIdentity(payload, {
      normalizeType: true,
      allowLegacyFallback: false,
    }),
    {
      principalType: "merchant",
      principalId: "merchant_uid_9",
      legacyId: "",
      role: "merchant",
      sessionId: "merchant_session_9",
      tokenKind: "access",
      phone: "",
      name: "",
    },
  );
});

test("identity contracts keep CommonJS bridge aligned with ESM exports", () => {
  const payload = {
    phone: "13800138000",
    principal_id: "merchant_uid_2",
    principal_legacy_id: 2,
    role: "merchant",
    token_kind: "refresh",
  };

  assert.deepEqual(cjsIdentity.PrincipalTypes, {
    USER: "user",
    MERCHANT: "merchant",
    RIDER: "rider",
    ADMIN: "admin",
    PARTNER: "partner",
  });
  assert.deepEqual(cjsIdentity.UnifiedTokenKinds, UnifiedTokenKinds);
  assert.deepEqual(
    cjsIdentity.extractUnifiedPrincipalIdentity(payload, { normalizeType: true }),
    extractUnifiedPrincipalIdentity(payload, { normalizeType: true }),
  );
  assert.equal(
    cjsIdentity.parseUnifiedTokenPayload(
      `${encodeBase64Url(JSON.stringify(payload))}.signature`,
    )?.principal_id,
    "merchant_uid_2",
  );
});
