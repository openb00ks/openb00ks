import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

vi.mock("$app/environment", () => ({
  browser: true,
}));

import {
  ACTIVE_ENTITY_STORAGE_KEY,
  clearActiveEntity,
} from "./entity";

describe("clearActiveEntity", () => {
  beforeEach(() => {
    localStorage.setItem(ACTIVE_ENTITY_STORAGE_KEY, "entity-1");
  });

  afterEach(() => {
    localStorage.removeItem(ACTIVE_ENTITY_STORAGE_KEY);
  });

  it("removes the stored active entity", () => {
    clearActiveEntity();
    expect(localStorage.getItem(ACTIVE_ENTITY_STORAGE_KEY)).toBeNull();
  });
});
