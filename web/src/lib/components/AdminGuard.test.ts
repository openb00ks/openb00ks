import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import { currentUser } from "$lib/stores/current-user";
import AdminGuard from "./AdminGuard.svelte";

// A tiny child is rendered via a slot-less wrapper: AdminGuard renders its children snippet, so we assert
// on the guard's own branches (loading / denied) plus the admin pass-through using the real store.

describe("AdminGuard", () => {
  afterEach(() => {
    cleanup();
    currentUser.set(null);
  });

  it("shows a checking state before the user loads", () => {
    currentUser.set(null);
    render(AdminGuard);
    expect(screen.getByText(/Checking access/i)).toBeInTheDocument();
  });

  it("denies a non-admin", () => {
    currentUser.set({ id: "u1", email: "member@test.local", is_admin: false });
    render(AdminGuard);
    expect(screen.getByText(/Admins only/i)).toBeInTheDocument();
  });

  it("admits an admin (no denial shown)", () => {
    currentUser.set({ id: "u1", email: "admin@test.local", is_admin: true });
    render(AdminGuard);
    expect(screen.queryByText(/Admins only/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Checking access/i)).not.toBeInTheDocument();
  });
});
