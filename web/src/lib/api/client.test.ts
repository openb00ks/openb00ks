import { get } from "svelte/store";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  clearSession,
  SESSION_COOKIE_KEY,
  setSession,
} from "$lib/stores/session";

type StoredSession = {
  token: string;
  tokenType: string;
  expiresAt: number;
  refreshToken?: string;
  refreshExpiresAt?: number;
};

const NOW = new Date("2026-02-28T12:00:00Z").getTime();

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "Content-Type": "application/json",
    },
  });
}

function storeSession(overrides: Partial<StoredSession> = {}) {
  const session: StoredSession = {
    token: "token-1",
    tokenType: "Bearer",
    expiresAt: NOW + 1_000,
    refreshToken: "refresh-1",
    refreshExpiresAt: NOW + 60_000,
    ...overrides,
  };
  setSession(session);
  return session;
}

function readCookieValue(name: string) {
  const prefix = `${encodeURIComponent(name)}=`;
  const parts = document.cookie ? document.cookie.split("; ") : [];
  for (const part of parts) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length));
    }
  }
  return null;
}

async function loadClient() {
  vi.resetModules();
  vi.doMock("$app/environment", () => ({
    browser: true,
  }));
  vi.doMock("$env/static/public", () => ({
    PUBLIC_API_BASE_URL: "",
  }));
  const [{ apiFetch, apiFetchPublic }, { session }] = await Promise.all([
    import("./client"),
    import("$lib/stores/session"),
  ]);
  return { apiFetch, apiFetchPublic, session };
}

describe("api client session handling", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
    document.cookie = `${encodeURIComponent(SESSION_COOKIE_KEY)}=; Path=/; Max-Age=0`;
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.doUnmock("$app/environment");
    vi.doUnmock("$env/static/public");
    clearSession();
  });

  it("passes through a valid token without attempting refresh", async () => {
    storeSession({ expiresAt: NOW + 120_000 });
    const authHeaders: Array<string | null> = [];

    const fetchMock = vi.fn((_: RequestInfo | URL, init?: RequestInit) => {
      authHeaders.push(new Headers(init?.headers).get("Authorization"));
      return Promise.resolve(jsonResponse({ ok: true }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetch } = await loadClient();
    const result = await apiFetch<{ ok: boolean }>("/receipts");

    expect(result).toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(authHeaders).toEqual(["Bearer token-1"]);
  }, 15_000);

  it("does not attach session credentials for public requests", async () => {
    storeSession();
    const authHeaders: Array<string | null> = [];

    const fetchMock = vi.fn((_: RequestInfo | URL, init?: RequestInit) => {
      authHeaders.push(new Headers(init?.headers).get("Authorization"));
      return Promise.resolve(jsonResponse({ required: true }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetchPublic } = await loadClient();
    const result = await apiFetchPublic<{ required: boolean }>("/setup/status");

    expect(result).toEqual({ required: true });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(authHeaders).toEqual([null]);
  }, 15_000);

  it("uses the local API default port when no server URL is configured", async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(jsonResponse({ ok: true })),
    );

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetch } = await loadClient();
    await apiFetch<{ ok: boolean }>("/healthz");

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8181/healthz",
      expect.any(Object),
    );
  }, 15_000);

  it("refreshes a near-expiry token before sending the request", async () => {
    storeSession({ expiresAt: NOW + 5_000 });
    const authHeaders: Array<string | null> = [];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/auth/refresh")) {
        return Promise.resolve(
          jsonResponse({
            token: "token-2",
            expires_in: 3_600,
            refresh_token: "refresh-2",
            refresh_expires_in: 7_200,
          }),
        );
      }

      authHeaders.push(new Headers(init?.headers).get("Authorization"));
      return Promise.resolve(jsonResponse({ ok: true }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetch } = await loadClient();
    const result = await apiFetch<{ ok: boolean }>("/transactions");

    expect(result).toEqual({ ok: true });
    const refreshCalls = fetchMock.mock.calls.filter(([input]) =>
      String(input).endsWith("/auth/refresh"),
    );
    expect(refreshCalls.length).toBeGreaterThanOrEqual(1);
    expect(authHeaders.length).toBeGreaterThanOrEqual(1);
    expect(authHeaders.every((value) => value === "Bearer token-2")).toBe(true);

    const stored = JSON.parse(readCookieValue(SESSION_COOKIE_KEY) ?? "null") as
      | StoredSession
      | null;
    expect(stored?.token).toBe("token-2");
    expect(stored?.refreshToken).toBe("refresh-2");
  }, 15_000);

  it("deduplicates concurrent refreshes and reuses the refreshed token", async () => {
    storeSession();
    const authHeaders: Array<string | null> = [];
    let resolveRefresh: ((value: Response) => void) | undefined;
    const refreshResponse = new Promise<Response>((resolve) => {
      resolveRefresh = resolve;
    });

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/auth/refresh")) {
        return refreshResponse;
      }

      authHeaders.push(new Headers(init?.headers).get("Authorization"));
      return Promise.resolve(jsonResponse({ ok: true }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetch } = await loadClient();
    const first = apiFetch<{ ok: boolean }>("/transactions");
    const second = apiFetch<{ ok: boolean }>("/accounts");

    await Promise.resolve();
    await Promise.resolve();

    expect(resolveRefresh).toBeDefined();
    resolveRefresh?.(
      jsonResponse({
        token: "token-2",
        expires_in: 3_600,
        refresh_token: "refresh-2",
        refresh_expires_in: 7_200,
      }),
    );

    await Promise.all([first, second]);

    expect(fetchMock).toHaveBeenCalledTimes(3);
    const refreshCalls = fetchMock.mock.calls.filter(([input]) =>
      String(input).endsWith("/auth/refresh"),
    );
    expect(refreshCalls).toHaveLength(1);
    expect(authHeaders).toEqual(["Bearer token-2", "Bearer token-2"]);

    const stored = JSON.parse(readCookieValue(SESSION_COOKIE_KEY) ?? "null") as
      | StoredSession
      | null;
    expect(stored?.token).toBe("token-2");
    expect(stored?.refreshToken).toBe("refresh-2");
  }, 15_000);

  it("clears the session and fails fast when refresh cannot recover", async () => {
    storeSession();

    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/auth/refresh")) {
        return Promise.resolve(jsonResponse({ error: "expired" }, 401));
      }

      return Promise.resolve(jsonResponse({ ok: true }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetch, session } = await loadClient();

    await expect(apiFetch("/receipts")).rejects.toThrow(
      "Session expired. Please sign in again.",
    );

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(readCookieValue(SESSION_COOKIE_KEY)).toBeNull();
    expect(get(session)).toBeNull();
  }, 15_000);

  it("retries once after a 401 by forcing a token refresh", async () => {
    storeSession({ expiresAt: NOW + 120_000 });
    const authHeaders: Array<string | null> = [];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/auth/refresh")) {
        return Promise.resolve(
          jsonResponse({
            token: "token-2",
            expires_in: 3_600,
          }),
        );
      }

      authHeaders.push(new Headers(init?.headers).get("Authorization"));
      if (authHeaders.length === 1) {
        return Promise.resolve(jsonResponse({ error: "unauthorized" }, 401));
      }

      return Promise.resolve(jsonResponse({ ok: true }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetch } = await loadClient();
    const result = await apiFetch<{ ok: boolean }>("/reports");

    expect(result).toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(authHeaders).toEqual(["Bearer token-1", "Bearer token-2"]);
  }, 15_000);

  it("logs out after a failed retry path on 401 responses", async () => {
    storeSession({ expiresAt: NOW + 120_000 });

    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/auth/refresh")) {
        return Promise.resolve(jsonResponse({ error: "denied" }, 401));
      }

      return Promise.resolve(jsonResponse({ error: "unauthorized" }, 401));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { apiFetch, session } = await loadClient();

    await expect(apiFetch("/exports")).rejects.toThrow(
      "Session expired. Please sign in again.",
    );

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(readCookieValue(SESSION_COOKIE_KEY)).toBeNull();
    expect(get(session)).toBeNull();
  }, 15_000);
});
