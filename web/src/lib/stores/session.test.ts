import { get } from "svelte/store";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  clearSession,
  initSession,
  session,
  SESSION_COOKIE_KEY,
  setSession,
} from "./session";

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

function seedCookie(value: string) {
  document.cookie = [
    `${encodeURIComponent(SESSION_COOKIE_KEY)}=${encodeURIComponent(value)}`,
    "Path=/",
    "SameSite=Lax",
  ].join("; ");
}

describe("session store", () => {
  const NOW = new Date("2026-05-28T12:00:00Z").getTime();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
    document.cookie = `${encodeURIComponent(SESSION_COOKIE_KEY)}=; Path=/; Max-Age=0`;
    clearSession();
  });

  afterEach(() => {
    vi.useRealTimers();
    document.cookie = `${encodeURIComponent(SESSION_COOKIE_KEY)}=; Path=/; Max-Age=0`;
    clearSession();
  });

  it("drops malformed JSON from cookies", () => {
    seedCookie("{bad-json");

    initSession();

    expect(get(session)).toBeNull();
    expect(readCookieValue(SESSION_COOKIE_KEY)).toBeNull();
  });

  it("drops malformed session objects from cookies", () => {
    seedCookie(
      JSON.stringify({ token: "", tokenType: "Bearer", expiresAt: NOW + 1_000 }),
    );

    initSession();

    expect(get(session)).toBeNull();
    expect(readCookieValue(SESSION_COOKIE_KEY)).toBeNull();
  });

  it("clears expired sessions when refresh window is invalid", () => {
    seedCookie(
      JSON.stringify({
        token: "token-1",
        tokenType: "Bearer",
        expiresAt: NOW - 1_000,
        refreshToken: "refresh-1",
      }),
    );

    initSession();

    expect(get(session)).toBeNull();
    expect(readCookieValue(SESSION_COOKIE_KEY)).toBeNull();
  });

  it("retains expired sessions when refresh window is still valid", () => {
    seedCookie(
      JSON.stringify({
        token: "token-1",
        tokenType: "Bearer",
        expiresAt: NOW - 1_000,
        refreshToken: "refresh-1",
        refreshExpiresAt: NOW + 60_000,
      }),
    );

    initSession();

    expect(get(session)).toMatchObject({
      token: "token-1",
      refreshToken: "refresh-1",
    });
  });

  it("refuses to persist invalid sessions via setSession", () => {
    setSession({
      token: "",
      tokenType: "Bearer",
      expiresAt: NOW + 60_000,
    });

    expect(get(session)).toBeNull();
    expect(readCookieValue(SESSION_COOKIE_KEY)).toBeNull();
  });
});
