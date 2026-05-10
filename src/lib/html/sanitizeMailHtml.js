import DOMPurify from "dompurify";

export function sanitizeMailHtml(html) {
  return DOMPurify.sanitize(html || "", {
    USE_PROFILES: { html: true },
  });
}
