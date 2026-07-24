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
  let mfaCode = $state("");
  let challengeToken = $state("");
  let mfaRequired = $state(false);
  let error = $state("");
  let loading = $state(false);
  const registrationEnabled = publicRegistrationEnabled();

  async function checkSetup() {
    try {
      const status = await apiFetchPublic<{ required: boolean }>("/setup/status");
      if (status.required) {
        await goto(decideSetupRoute(status.required));
      }
    } catch {
      // Ignore setup status errors on login.
    }
  }

  async function submit() {
    error = "";
    loading = true;
    const cleanEmail = email.trim();
    const cleanPassword = password;

    try {
      const response = await apiFetch<{
        token?: string;
        token_type?: string;
        expires_in?: number;
        refresh_token?: string;
        refresh_expires_in?: number;
        mfa_required?: boolean;
        challenge_token?: string;
      }>("/auth/login", {
        method: "POST",
        body: { email: cleanEmail, password: cleanPassword },
      });
      if (response.mfa_required) {
        const nextChallengeToken = response.challenge_token;
        if (!nextChallengeToken) {
          throw new Error("Login requires MFA but no challenge was returned");
        }
        challengeToken = nextChallengeToken;
        mfaRequired = true;
        error = "Enter the MFA code from your authenticator app.";
        return;
      }
      const token = response.token;
      if (!token) {
        throw new Error("Login succeeded but no token returned");
      }
      const tokenType = response.token_type || "Bearer";
      const expiresIn = response.expires_in ?? 3600;
      const refreshToken = response.refresh_token;
      const refreshExpiresIn = response.refresh_expires_in ?? 0;
      setSession({
        token,
        tokenType,
        expiresAt: Date.now() + expiresIn * 1000,
        refreshToken: refreshToken,
        refreshExpiresAt: refreshToken
          ? Date.now() + refreshExpiresIn * 1000
          : undefined,
      });
      await goto("/");
    } catch (err) {
      const code = getApiErrorCode(err);
      error = errorMessage(err, "Login failed. Please try again.");
      if (code === "SETUP_REQUIRED") {
        await goto(decideSetupRoute(true));
      }
      if (code === "MFA_SETUP_REQUIRED") {
        error = "MFA is required for this account. Set it up from user settings first.";
      }
    } finally {
      loading = false;
    }
  }

  async function submitMfa() {
    error = "";
    loading = true;
    try {
      const response = await apiFetch<{
        token?: string;
        token_type?: string;
        expires_in?: number;
        refresh_token?: string;
        refresh_expires_in?: number;
      }>("/auth/login/mfa", {
        method: "POST",
        body: { challenge_token: challengeToken, code: mfaCode.trim() },
      });
      const token = response.token;
      if (!token) {
        throw new Error("MFA succeeded but no token returned");
      }
      const tokenType = response.token_type || "Bearer";
      const expiresIn = response.expires_in ?? 3600;
      const refreshToken = response.refresh_token;
      const refreshExpiresIn = response.refresh_expires_in ?? 0;
      setSession({
        token,
        tokenType,
        expiresAt: Date.now() + expiresIn * 1000,
        refreshToken: refreshToken,
        refreshExpiresAt: refreshToken
          ? Date.now() + refreshExpiresIn * 1000
          : undefined,
      });
      await goto("/");
    } catch (err) {
      const code = getApiErrorCode(err);
      error = errorMessage(err, "MFA verification failed. Please try again.");
      if (code === "MFA_CHALLENGE_EXPIRED") {
        mfaRequired = false;
        challengeToken = "";
        mfaCode = "";
      }
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    checkSetup();
  });
</script>

<section class="mx-auto max-w-md rounded-3xl bg-surface p-8 shadow-sm">
  <h1 class="text-2xl font-semibold tracking-tight">Sign in</h1>
  <p class="mt-2 text-sm text-muted">
    Sign in to continue review and posting work for your entities.
  </p>
  {#if registrationEnabled}
    <p class="mt-2 text-sm text-muted">
      Need a new workspace? <a class="font-semibold text-ink underline" href="/register">Create an account</a>.
    </p>
  {/if}

  <form
    class="mt-6 grid gap-4"
    onsubmit={(event) => {
      event.preventDefault();
      if (mfaRequired) {
        void submitMfa();
        return;
      }
      void submit();
    }}
  >
    {#if mfaRequired}
      <label class="grid gap-2 text-sm font-medium">
        MFA code
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          type="text"
          inputmode="numeric"
          autocomplete="one-time-code"
          bind:value={mfaCode}
          required
        />
      </label>
    {/if}
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
        autocomplete="current-password"
        bind:value={password}
        required
      />
    </label>

    {#if error}
      <p
        class="status-message-sm status-error"
      >
        {error}
      </p>
    {/if}

    {#if mfaRequired}
      <button
        class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
        type="submit"
        disabled={loading}
      >
        {loading ? "Verifying…" : "Verify code"}
      </button>
    {:else}
      <button
        class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
        type="submit"
        disabled={loading}
      >
        {loading ? "Signing in…" : "Sign in"}
      </button>
    {/if}
  </form>
</section>

