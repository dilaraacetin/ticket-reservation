// Talking to the API. One place, so headers and failures are not spread across
// every call site.

export class ApiError extends Error {
  constructor(status, payload, retryAfter) {
    super(payload?.error?.message ?? `request failed with ${status}`);

    this.status = status;
    this.code = payload?.error?.code ?? "unknown";
    this.retryAfter = Number(retryAfter) || 0;
  }
}

// The token lives in memory only. sessionStorage would survive a reload, but it
// is readable by anything that ends up on the page, and a reload asking for a
// sign in is a small price.
let token = null;

export function setToken(value) {
  token = value;
}

export function hasToken() {
  return token !== null;
}

// newIdempotencyKey returns a fresh key for a write that must not be applied twice.
//
// crypto.randomUUID exists only in a secure context, so over plain http on a LAN
// address — which is how this is tried from a phone — it is undefined and
// calling it throws before the request is ever sent. crypto.getRandomValues has
// no such restriction, so the key is built from that when it has to be.
export function newIdempotencyKey() {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();

  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;

  const hex = [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");

  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export async function api(method, path, { body, idempotencyKey } = {}) {
  const headers = {};

  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

  const response = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  // 204 has no body to read, and neither does a failure that sent none.
  const payload =
    response.status === 204 ? null : await response.json().catch(() => null);

  if (!response.ok) {
    throw new ApiError(response.status, payload, response.headers.get("Retry-After"));
  }

  return payload;
}

// openStream holds a connection open and calls back on every notice.
//
// fetch rather than EventSource, which would be the obvious choice: turn_came is
// addressed to one person and the server works out who from the Authorization
// header. EventSource cannot send one, so a page built on it hears every public
// notice and never the one meant for it.
export function openStream(eventID, handlers) {
  const controller = new AbortController();
  let closed = false;

  const read = async () => {
    try {
      const response = await fetch(`/events/${encodeURIComponent(eventID)}/stream`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        signal: controller.signal,
      });

      if (!response.ok || !response.body) throw new Error("the stream did not open");

      handlers.onOpen?.();

      const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
      let buffer = "";

      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += value;

        // A blank line ends a notice. That and the two field names below are the
        // whole of the format this page has to understand.
        let end;
        while ((end = buffer.indexOf("\n\n")) >= 0) {
          dispatch(buffer.slice(0, end), handlers);
          buffer = buffer.slice(end + 2);
        }
      }
    } catch {
      // Any failure falls through to the retry below.
    }

    if (closed) return;

    handlers.onLost?.();
    // EventSource comes back on its own after a drop; this has to be told to.
    setTimeout(read, 3000);
  };

  read();

  return {
    close() {
      closed = true;
      controller.abort();
    },
    retry() {
      controller.abort();
    },
  };
}

function dispatch(block, handlers) {
  let kind = "";
  let data = "";

  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) kind = line.slice(6).trim();
    else if (line.startsWith("data:")) data += line.slice(5).trim();
  }

  // Heartbeats are comment lines, with neither a kind nor a payload.
  if (!kind || !data) return;

  handlers[kind]?.(JSON.parse(data));
}
