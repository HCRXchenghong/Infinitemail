export const UPLOAD_DOMAINS = Object.freeze({
  AFTER_SALES_EVIDENCE: "after_sales_evidence",
  PROFILE_IMAGE: "profile_image",
  CHAT_ATTACHMENT: "chat_attachment",
  SHOP_MEDIA: "shop_media",
  REVIEW_MEDIA: "review_media",
  SERVICE_SOUND: "service_sound",
  APP_DOWNLOAD_QR: "app_download_qr",
  APP_PACKAGE: "app_package",
  MERCHANT_DOCUMENT: "merchant_document",
  MEDICAL_DOCUMENT: "medical_document",
  ONBOARDING_DOCUMENT: "onboarding_document",
  ADMIN_ASSET: "admin_asset",
});

const uploadDomainSet = new Set(Object.values(UPLOAD_DOMAINS));

export function isUploadDomain(value) {
  const normalized = String(value || "").trim().toLowerCase();
  return uploadDomainSet.has(normalized);
}

export function normalizeUploadDomain(value, fallback = "") {
  const normalized = String(value || fallback || "").trim().toLowerCase();
  if (!normalized || !uploadDomainSet.has(normalized)) {
    throw new Error(`Unsupported upload domain: ${value}`);
  }
  return normalized;
}
