import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/environment", () => ({
  browser: true,
}));

import { initTheme, setTheme } from "./theme";

describe("theme store", () => {
  beforeEach(() => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query.includes("dark"),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
  });

  afterEach(() => {
    document.documentElement.classList.remove("dark");
    localStorage.removeItem("ob_theme");
    vi.unstubAllGlobals();
  });

  it("toggles the dark class when selecting dark mode", () => {
    setTheme("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("ob_theme")).toBe("dark");
  });

  it("clears the dark class when selecting light mode", () => {
    document.documentElement.classList.add("dark");
    setTheme("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(localStorage.getItem("ob_theme")).toBe("light");
  });

  it("defaults to the browser/system theme until overridden", async () => {
    await initTheme();
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("ob_theme")).toBeNull();
  });
});
