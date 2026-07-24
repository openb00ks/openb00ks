<script lang="ts">
  import { errorMessage } from "$lib/utils/errors";
  import { goto } from "$app/navigation";
  import { getApiErrorCode } from "$lib/api/errors";
  import { apiFetch, apiFetchPublic } from "$lib/api/client";
  import { setSession } from "$lib/stores/session";
  import { publicRegistrationEnabled } from "$lib/utils/auth-mode";
  import { onMount } from "svelte";
  import { decideSetupRoute } from "$lib/utils/setup";

  let email = $state("");
  let password = $state("");
  let tenantName = $state("");
  let error = $state("");
  let loading = $state(false);
  let setupRequired = $state(false);
  const registrationEnabled = publicRegistrationEnabled();

  async function checkSetup() {
    if (!registrationEnabled) {
      await goto("/login");
      return;
    }
    try {
      const status = await apiFetchPublic<{ required: boolean }>("/setup/status");
      setupRequired = status.required;
      if (status.required) {
        await goto(decideSetupRoute(true));
      }
    } catch {
      // Ignore setup status errors and let registration request surface a stable error.
    }
  }

  async function submit() {
    error = "";
    loading = true;
    const cleanEmail = email.trim();
    const cleanPassword = password;
    const cleanTenantName = tenantName.trim();

    try {
      const response = await apiFetch<{
        token?: string;
        token_type?: string;
        expires_in?: number;
        refresh_token?: string;
        refresh_expires_in?: number;
        tenant_id?: string;
      }>("/auth/register", {
        method: "POST",
        body: {
          email: cleanEmail,
          password: cleanPassword,
          tenant_name: cleanTenantName || undefined,
        },
      });
      const token = response.token;
      if (!token) {
        throw new Error("Registration succeeded but no token returned");
      }
      const tokenType = response.token_type || "Bearer";
      const expiresIn = response.expires_in ?? 3600;
      const refreshToken = response.refresh_token;
      const refreshExpiresIn = response.refresh_expires_in ?? 0;
      setSession({
        token,
        tokenType,
        expiresAt: Date.now() + expiresIn * 1000,
        refreshToken,
        refreshExpiresAt: refreshToken
          ? Date.now() + refreshExpiresIn * 1000
          : undefined,
      });
      await goto("/");
    } catch (err) {
      const code = getApiErrorCode(err);
      error =
        errorMessage(err, "Registration failed. Please try again.");
      if (code === "REGISTRATION_DISABLED") {
        await goto("/login");
        return;
      }
      if (code === "SETUP_REQUIRED") {
        setupRequired = true;
      }
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    checkSetup();
  });
</script>

<section class="mx-auto grid max-w-4xl gap-6 lg:grid-cols-[1.05fr_0.95fr]">
  <div class="rounded-3xl bg-surface p-8 shadow-sm">
    <p class="text-xs uppercase tracking-[0.2em] text-muted">Public registration</p>
    <h1 class="mt-2 text-2xl font-semibold tracking-tight">Create your workspace</h1>
    <p class="mt-2 text-sm text-muted">
      Registration creates a new user and a new tenant so you can start capturing receipts immediately.
    </p>

    <form class="mt-6 grid gap-4" onsubmit={(event) => { event.preventDefault(); void submit(); }}>
      <label class="grid gap-2 text-sm font-medium">
        Email
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          type="email"
          autocomplete="email"
          bind:value={email}
          required
        />
      </label>

      <label class="grid gap-2 text-sm font-medium">
        Password
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          type="password"
          autocomplete="new-password"
          bind:value={password}
          required
        />
      </label>

      <label class="grid gap-2 text-sm font-medium">
        Workspace name (optional)
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          type="text"
          placeholder="Default Tenant"
          bind:value={tenantName}
        />
      </label>

      {#if error}
        <p class="status-message-sm status-error">
          {error}
        </p>
      {/if}

      <button
        class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
        type="submit"
        disabled={loading || setupRequired}
      >
        {loading ? "Creating account…" : "Create account"}
      </button>

      <p class="text-sm text-muted">
        Already have an account?
        <a class="font-semibold text-ink underline" href="/login">Sign in</a>.
      </p>
    </form>
  </div>

  <div class="rounded-3xl border border-line bg-surface p-8">
    <h2 class="text-lg font-semibold">What happens after signup</h2>
    <div class="mt-4 grid gap-3 text-sm text-muted">
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">1. A new tenant is created</p>
        <p class="mt-2">Your account gets its own workspace instead of joining an existing tenant.</p>
      </div>
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">2. You sign in immediately</p>
        <p class="mt-2">Registration returns the same authenticated session contract as sign-in, so you land in the app ready to work.</p>
      </div>
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">3. Posting still stays manual</p>
        <p class="mt-2">After signup, create or select an entity, capture receipts, review drafts, then choose when to post.</p>
      </div>
    </div>
  </div>
</section>

