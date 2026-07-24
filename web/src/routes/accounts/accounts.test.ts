import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  let activeEntityValue: string | null = "entity-1";
  const subscribers = new Set<(value: string | null) => void>();

  const activeEntity = {
    subscribe(run: (value: string | null) => void) {
      run(activeEntityValue);
      subscribers.add(run);
      return () => subscribers.delete(run);
    },
    set(value: string | null) {
      activeEntityValue = value;
      for (const run of subscribers) {
        run(activeEntityValue);
      }
    },
    update(updater: (value: string | null) => string | null) {
      activeEntityValue = updater(activeEntityValue);
      for (const run of subscribers) {
        run(activeEntityValue);
      }
    },
  };

  return {
    activeEntity,
    apiFetch: vi.fn(),
  };
});

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
}));

vi.mock("$lib/stores/entity", () => ({
  activeEntity: mocks.activeEntity,
}));

import AccountsPage from "./+page.svelte";

function setInputValue(label: string, value: string) {
  const input = screen.getAllByLabelText(label, {
    exact: true,
  })[0] as HTMLInputElement;
  input.value = value;
  // input for text fields, change for <select> bind:value.
  input.dispatchEvent(new Event("input", { bubbles: true }));
  input.dispatchEvent(new Event("change", { bubbles: true }));
  return input;
}

describe("Accounts page", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    mocks.activeEntity.set("entity-1");
    mocks.apiFetch.mockReset();
  });

  async function openCreateForm() {
    screen.getByRole("button", { name: "New account" }).click();
    await vi.waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Create account" }),
      ).toBeInTheDocument();
    });
  }

  it("creates an account successfully", async () => {
    mocks.apiFetch.mockImplementation(
      (path: string, init?: { method?: string }) => {
        if (path === "/entities/entity-1/accounts" && !init?.method) {
          // The accounts endpoint returns a bare array, not { rows }.
          return Promise.resolve([
            { id: "acct-1", name: "Cash", type: "asset", code: "1000", role_tags: [] },
          ]);
        }
        if (path === "/entities/entity-1/accounts" && init?.method === "POST") {
          return Promise.resolve({
            id: "acct-2",
            name: "Travel",
            type: "expense",
            code: "5080",
            role_tags: [],
          });
        }
        return Promise.reject(new Error(`Unexpected call: ${path}`));
      },
    );

    render(AccountsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Cash")).toBeInTheDocument();
    });

    await openCreateForm();

    setInputValue("Code", "5080");
    setInputValue("Name", "Travel");
    setInputValue("Type", "expense");

    screen.getByRole("button", { name: "Create account" }).click();

    await vi.waitFor(() => {
      expect(screen.getByText("Account created.")).toBeInTheDocument();
      expect(screen.getByText("Travel")).toBeInTheDocument();
    });

    expect(mocks.apiFetch).toHaveBeenCalledWith("/entities/entity-1/accounts", {
      method: "POST",
      body: { name: "Travel", type: "expense", code: "5080", role_tags: [] },
    });
  });

  it("assigns role tags when creating an account", async () => {
    mocks.apiFetch.mockImplementation(
      (path: string, init?: { method?: string }) => {
        if (path === "/entities/entity-1/accounts" && !init?.method) {
          return Promise.resolve([]);
        }
        if (path === "/entities/entity-1/accounts" && init?.method === "POST") {
          return Promise.resolve({
            id: "acct-2",
            name: "Utilities",
            type: "expense",
            role_tags: ["utilities", "internet"],
          });
        }
        return Promise.reject(new Error(`Unexpected call: ${path}`));
      },
    );

    render(AccountsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("No accounts yet.")).toBeInTheDocument();
    });

    await openCreateForm();

    setInputValue("Name", "Utilities");
    setInputValue("Type", "expense");
    screen.getByLabelText("Utilities").click();
    screen.getByLabelText("Home internet").click();

    screen.getByRole("button", { name: "Create account" }).click();

    await vi.waitFor(() => {
      expect(screen.getByText("Account created.")).toBeInTheDocument();
    });

    expect(mocks.apiFetch).toHaveBeenCalledWith("/entities/entity-1/accounts", {
      method: "POST",
      body: { name: "Utilities", type: "expense", code: "", role_tags: ["utilities", "internet"] },
    });
  });

  it("shows create validation and API errors", async () => {
    mocks.apiFetch.mockImplementation(
      (path: string, init?: { method?: string }) => {
        if (path === "/entities/entity-1/accounts" && !init?.method) {
          return Promise.resolve([]);
        }
        if (path === "/entities/entity-1/accounts" && init?.method === "POST") {
          return Promise.reject(new Error("ACCOUNT_EXISTS"));
        }
        return Promise.reject(new Error(`Unexpected call: ${path}`));
      },
    );

    render(AccountsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("No accounts yet.")).toBeInTheDocument();
    });

    await openCreateForm();

    screen.getByRole("button", { name: "Create account" }).click();

    await vi.waitFor(() => {
      expect(
        screen.getByText("Name and type are required."),
      ).toBeInTheDocument();
    });

    setInputValue("Name", "Cash");
    setInputValue("Type", "asset");
    screen.getByRole("button", { name: "Create account" }).click();

    await vi.waitFor(() => {
      expect(screen.getByText("ACCOUNT_EXISTS")).toBeInTheDocument();
    });
  });

  it("deletes an account successfully", async () => {
    let deleted = false;
    mocks.apiFetch.mockImplementation(
      (path: string, init?: { method?: string }) => {
        if (path === "/entities/entity-1/accounts" && !init?.method) {
          return Promise.resolve([
            { id: "acct-1", name: "Cash", type: "asset", role_tags: [] },
            { id: "acct-2", name: "Travel", type: "expense", role_tags: [] },
          ]);
        }
        if (path === "/accounts/acct-2" && init?.method === "DELETE") {
          deleted = true;
          return Promise.resolve(undefined);
        }
        return Promise.reject(new Error(`Unexpected call: ${path}`));
      },
    );

    render(AccountsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Travel")).toBeInTheDocument();
    });

    screen.getAllByRole("button", { name: "Delete" })[1].click();

    await vi.waitFor(() => {
      expect(screen.queryByText("Travel")).not.toBeInTheDocument();
    });

    expect(deleted).toBe(true);
  });

  it("blocks deleting an account that has transactions", async () => {
    mocks.apiFetch.mockImplementation(
      (path: string, init?: { method?: string }) => {
        if (path === "/entities/entity-1/accounts" && !init?.method) {
          return Promise.resolve([{ id: "acct-1", name: "Cash", type: "asset", code: "1000", role_tags: [] }]);
        }
        if (path === "/accounts/acct-1" && init?.method === "DELETE") {
          return Promise.reject(new Error("ACCOUNT_IN_USE"));
        }
        return Promise.reject(new Error(`Unexpected call: ${path}`));
      },
    );

    render(AccountsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Cash")).toBeInTheDocument();
    });

    screen.getByRole("button", { name: "Delete" }).click();

    await vi.waitFor(() => {
      expect(screen.getByText(/has transactions and can.t be deleted/i)).toBeInTheDocument();
    });
    // The account is restored (optimistic removal rolled back).
    expect(screen.getByText("Cash")).toBeInTheDocument();
  });

  it("edits an account successfully", async () => {
    mocks.apiFetch.mockImplementation(
      (path: string, init?: { method?: string; body?: unknown }) => {
        if (path === "/entities/entity-1/accounts" && !init?.method) {
          return Promise.resolve([
            { id: "acct-1", name: "Cash", type: "asset", code: "1000", role_tags: [] },
          ]);
        }
        if (path === "/accounts/acct-1" && init?.method === "PATCH") {
          return Promise.resolve(undefined);
        }
        return Promise.reject(new Error(`Unexpected call: ${path}`));
      },
    );

    render(AccountsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Cash")).toBeInTheDocument();
    });

    screen.getByRole("button", { name: "Edit" }).click();

    await vi.waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    });

    setInputValue("Name", "Cash On Hand");
    setInputValue("Type", "liability");
    screen.getByRole("button", { name: "Save" }).click();

    await vi.waitFor(() => {
      expect(screen.getByText("Cash On Hand")).toBeInTheDocument();
    });

    expect(mocks.apiFetch).toHaveBeenCalledWith("/accounts/acct-1", {
      method: "PATCH",
      body: { name: "Cash On Hand", type: "liability", code: "1000", role_tags: [] },
    });
  });

  it("rolls back account deletion when the API fails", async () => {
    let rejectDelete: ((reason?: unknown) => void) | null = null;
    mocks.apiFetch.mockImplementation(
      (path: string, init?: { method?: string }) => {
        if (path === "/entities/entity-1/accounts" && !init?.method) {
          return Promise.resolve([
            { id: "acct-1", name: "Cash", type: "asset", role_tags: [] },
          ]);
        }
        if (path === "/accounts/acct-1" && init?.method === "DELETE") {
          return new Promise((_, reject) => {
            rejectDelete = reject;
          });
        }
        return Promise.reject(new Error(`Unexpected call: ${path}`));
      },
    );

    render(AccountsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Cash")).toBeInTheDocument();
    });

    screen.getByRole("button", { name: "Delete" }).click();

    await vi.waitFor(() => {
      expect(screen.queryByText("Cash")).not.toBeInTheDocument();
    });

    const pendingDeleteReject = rejectDelete as
      | ((reason?: unknown) => void)
      | null;

    if (!pendingDeleteReject) {
      throw new Error("Expected delete request to be pending");
    }

    (pendingDeleteReject as (reason?: unknown) => void)(
      new Error("DELETE_FAILED"),
    );

    await vi.waitFor(() => {
      expect(screen.getByText("Cash")).toBeInTheDocument();
      expect(screen.getByText("DELETE_FAILED")).toBeInTheDocument();
    });
  });
});
