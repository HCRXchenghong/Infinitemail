function withQuery(path, query) {
  if (!query || Object.keys(query).length === 0) {
    return path;
  }

  const searchParams = new URLSearchParams();
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") {
      searchParams.set(key, String(value));
    }
  });

  const queryString = searchParams.toString();
  return queryString ? `${path}?${queryString}` : path;
}

function resolveUrl(baseUrl, path) {
  if (/^https?:\/\//i.test(path)) {
    return path;
  }

  const normalizedBase = String(baseUrl || "").replace(/\/+$/, "");
  const normalizedPath = String(path || "");

  if (!normalizedBase) {
    return normalizedPath;
  }

  if (!normalizedPath) {
    return normalizedBase;
  }

  if (normalizedPath.startsWith("/")) {
    return `${normalizedBase}${normalizedPath}`;
  }

  return `${normalizedBase}/${normalizedPath}`;
}

export function createHttpClient({ baseUrl, timeoutMs, getHeaders }) {
  async function request(method, path, { query, body, headers } = {}) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), timeoutMs);

    try {
      const mergedHeaders = {
        "Content-Type": "application/json",
        ...(typeof getHeaders === "function" ? getHeaders({ method, path, query, body }) : {}),
        ...headers,
      };

      const response = await fetch(resolveUrl(baseUrl, withQuery(path, query)), {
        method,
        credentials: "include",
        headers: mergedHeaders,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      const payload = await response.json().catch(() => null);

      if (!response.ok) {
        const message = payload?.message || `Request failed with status ${response.status}`;
        const error = new Error(message);
        error.status = response.status;
        error.payload = payload;
        throw error;
      }

      return payload;
    } finally {
      clearTimeout(timeout);
    }
  }

  return {
    get: (path, options) => request("GET", path, options),
    post: (path, options) => request("POST", path, options),
    put: (path, options) => request("PUT", path, options),
    patch: (path, options) => request("PATCH", path, options),
  };
}
