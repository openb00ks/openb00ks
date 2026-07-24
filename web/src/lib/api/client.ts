import { browser } from "$app/environment";
import { PUBLIC_API_BASE_URL } from "$env/static/public";
import { toApiError } from "$lib/api/errors";
import { DEFAULT_API_BASE_URL } from "$lib/config";
import {
  clearSession,
  SESSION_COOKIE_KEY,
  setSession,
} from "$lib/stores/session";

function resolveBaseUrl() {
  const base = PUBLIC_API_BASE_URL || DEFAULT_API_BASE_URL;
  return base.replace(/\/$/, "");
}

type StoredSession = {
  token: string;
  tokenType: string;
  expiresAt: number;
  refreshToken?: string;
  refreshExpiresAt?: number;
};

type RefreshResponse = {
  token?: string;
  token_type?: string;
  expires_in?: number;
  refresh_token?: string;
  refresh_expires_in?: number;
};

const REFRESH_BUFFER_MS = 30_000;
const INVALID_SESSION_MESSAGE = "Session expired. Please sign in again.";
let refreshState: {
  key: string;
  promise: Promise<StoredSession | null>;
} | null = null;

function hasRefreshCapacity(session: StoredSession, now = Date.now()) {
  return Boolean(
    session.refreshToken &&
    session.refreshExpiresAt &&
    Number.isFinite(session.refreshExpiresAt) &&
    now < session.refreshExpiresAt,
  );
}

function readCookie(name: string) {
  if (!browser) {
    return null;
  }
  const prefix = `${encodeURIComponent(name)}=`;
  const parts = document.cookie ? document.cookie.split("; ") : [];
  for (const part of parts) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length));
    }
  }
  return null;
}

function readStoredSession(): StoredSession | null {
  if (!browser) {
    return null;
  }
  const raw = readCookie(SESSION_COOKIE_KEY);
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as StoredSession;
    if (
      !parsed.token ||
      !parsed.tokenType ||
      !parsed.expiresAt ||
      !Number.isFinite(parsed.expiresAt)
    ) {
      writeStoredSession(null);
      return null;
    }
    if (Date.now() >= parsed.expiresAt && !hasRefreshCapacity(parsed)) {
      writeStoredSession(null);
      return null;
    }
    return parsed;
  } catch {
    writeStoredSession(null);
    return null;
  }
}

function writeStoredSession(next: StoredSession | null) {
  if (!next) {
    clearSession();
    return;
  }
  setSession(next);
}

function getSessionKey(session: StoredSession) {
  return session.refreshToken || `${session.token}:${session.expiresAt}`;
}

function matchesSession(expected: StoredSession) {
  const current = readStoredSession();
  if (!current) {
    return false;
  }
  return (
    current.token === expected.token &&
    current.tokenType === expected.tokenType &&
    current.expiresAt === expected.expiresAt &&
    current.refreshToken === expected.refreshToken &&
    current.refreshExpiresAt === expected.refreshExpiresAt
  );
}

function invalidateSession(expected?: StoredSession) {
  if (expected && !matchesSession(expected)) {
    return false;
  }
  writeStoredSession(null);
  return true;
}

async function refreshSession(
  current: StoredSession,
): Promise<StoredSession | null> {
  if (!hasRefreshCapacity(current)) {
    return null;
  }
  const url = `${resolveBaseUrl()}/auth/refresh`;
  let response: Response;
  try {
    response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ refresh_token: current.refreshToken }),
    });
  } catch {
    return null;
  }
  if (!response.ok) {
    return null;
  }
  let payload: RefreshResponse;
  try {
    payload = (await response.json()) as RefreshResponse;
  } catch {
    return null;
  }
  if (!payload.token) {
    return null;
  }
  const expiresIn = payload.expires_in ?? 3600;
  if (!Number.isFinite(expiresIn) || expiresIn <= 0) {
    return null;
  }
  const tokenType = payload.token_type || current.tokenType || "Bearer";
  const refreshToken = payload.refresh_token || current.refreshToken;
  const refreshExpiresIn = payload.refresh_expires_in ?? 0;
  const next = {
    token: payload.token,
    tokenType,
    expiresAt: Date.now() + expiresIn * 1000,
    refreshToken,
    refreshExpiresAt:
      refreshToken && refreshExpiresIn
        ? Date.now() + refreshExpiresIn * 1000
        : current.refreshExpiresAt,
  };
  if (Date.now() >= next.expiresAt) {
    return null;
  }
  return next;
}

async function ensureValidSession(
  forceRefresh = false,
): Promise<StoredSession | null> {
  const stored = readStoredSession();
  if (!stored) {
    return null;
  }
  const now = Date.now();
  if (!forceRefresh && now < stored.expiresAt - REFRESH_BUFFER_MS) {
    return stored;
  }
  const refreshKey = getSessionKey(stored);
  if (!refreshState || refreshState.key !== refreshKey) {
    const promise = refreshSession(stored)
      .then((next) => {
        if (next) {
          if (matchesSession(stored)) {
            writeStoredSession(next);
            return next;
          }
          return readStoredSession();
        }
        if (matchesSession(stored)) {
          invalidateSession(stored);
          return null;
        }
        return readStoredSession();
      })
      .finally(() => {
        if (refreshState?.key === refreshKey) {
          refreshState = null;
        }
      });
    refreshState = { key: refreshKey, promise };
  }
  return refreshState.promise;
}

export type ApiRequestInit = Omit<RequestInit, "body"> & {
  body?: unknown;
};

function isBodyInit(value: unknown): value is BodyInit {
  if (typeof value === "string") {
    return true;
  }
  if (value instanceof ArrayBuffer) {
    return true;
  }
  if (typeof Blob !== "undefined" && value instanceof Blob) {
    return true;
  }
  if (typeof FormData !== "undefined" && value instanceof FormData) {
    return true;
  }
  if (
    typeof URLSearchParams !== "undefined" &&
    value instanceof URLSearchParams
  ) {
    return true;
  }
  return false;
}

type PreparedRequest = {
  body: BodyInit | undefined;
  headers: Headers;
  url: string;
};

type RequestResult = {
  authAttached: boolean;
  response: Response;
};

function prepareRequest(path: string, init: ApiRequestInit): PreparedRequest {
  const apiBaseUrl = resolveBaseUrl();
  const url = `${apiBaseUrl}${path.startsWith("/") ? "" : "/"}${path}`;
  const headers = new Headers(init.headers);
  let body: BodyInit | undefined;

  if (init.body !== undefined) {
    if (isBodyInit(init.body)) {
      body = init.body;
    } else {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(init.body);
    }
  }

  return { body, headers, url };
}

async function performRequest(
  init: ApiRequestInit,
  prepared: PreparedRequest,
  forceRefresh = false,
  useSession = true,
): Promise<RequestResult> {
  const headers = new Headers(prepared.headers);
  let authAttached = false;

  if (useSession && browser && !headers.has("Authorization")) {
    const hadStoredSession = Boolean(readStoredSession());
    const stored = await ensureValidSession(forceRefresh);
    if (hadStoredSession && !stored) {
      throw new Error(INVALID_SESSION_MESSAGE);
    }
    if (stored?.token && stored?.tokenType && Date.now() < stored.expiresAt) {
      headers.set("Authorization", `${stored.tokenType} ${stored.token}`);
      authAttached = true;
    }
  }

  const response = await fetch(prepared.url, {
    ...init,
    headers,
    body: prepared.body,
  });

  return { authAttached, response };
}

async function requestWithSession(
  path: string,
  init: ApiRequestInit,
): Promise<Response> {
  const prepared = prepareRequest(path, init);
  let result = await performRequest(init, prepared);

  if (browser && result.authAttached && result.response.status === 401) {
    const refreshed = await ensureValidSession(true);
    if (refreshed?.token && Date.now() < refreshed.expiresAt) {
      result = await performRequest(init, prepared);
      if (result.response.status !== 401) {
        return result.response;
      }
    }
    invalidateSession();
    throw new Error(INVALID_SESSION_MESSAGE);
  }

  return result.response;
}

async function requestWithoutSession(
  path: string,
  init: ApiRequestInit,
): Promise<Response> {
  const prepared = prepareRequest(path, init);
  const result = await performRequest(init, prepared, false, false);
  return result.response;
}

export async function apiFetch<T>(
  path: string,
  init: ApiRequestInit = {},
): Promise<T> {
  const response = await requestWithSession(path, init);

  if (!response.ok) {
    const contentType = response.headers.get("content-type") ?? "";
    if (contentType.includes("application/json")) {
      let payload: { error?: string } | null;
      try {
        payload = (await response.json()) as { error?: string };
      } catch {
        payload = null;
      }
      if (payload?.error) {
        throw toApiError(payload.error, response.status);
      }
    }
    const message = await response.text();
    throw toApiError(
      message || `Request failed: ${response.status}`,
      response.status,
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export async function apiFetchPublic<T>(
  path: string,
  init: ApiRequestInit = {},
): Promise<T> {
  const response = await requestWithoutSession(path, init);

  if (!response.ok) {
    const contentType = response.headers.get("content-type") ?? "";
    if (contentType.includes("application/json")) {
      let payload: { error?: string } | null;
      try {
        payload = (await response.json()) as { error?: string };
      } catch {
        payload = null;
      }
      if (payload?.error) {
        throw toApiError(payload.error, response.status);
      }
    }
    const message = await response.text();
    throw toApiError(
      message || `Request failed: ${response.status}`,
      response.status,
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export async function apiFetchBlob(
  path: string,
  init: ApiRequestInit = {},
): Promise<Blob> {
  const response = await requestWithSession(path, init);

  if (!response.ok) {
    const contentType = response.headers.get("content-type") ?? "";
    if (contentType.includes("application/json")) {
      let payload: { error?: string } | null;
      try {
        payload = (await response.json()) as { error?: string };
      } catch {
        payload = null;
      }
      if (payload?.error) {
        throw toApiError(payload.error, response.status);
      }
    }
    const message = await response.text();
    throw toApiError(
      message || `Request failed: ${response.status}`,
      response.status,
    );
  }

  return await response.blob();
}
