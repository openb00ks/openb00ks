<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatLongDate as formatDate } from '$lib/utils/date';
  import { onMount } from 'svelte';
  import { setTheme, theme } from '$lib/stores/theme';
  import { apiFetch } from '$lib/api/client';
  import { clearSession } from '$lib/stores/session';
  import { currentUser } from '$lib/stores/current-user';
  import { entities } from '$lib/stores/entity';

  onMount(() => {
    void loadMfaStatus();
    void loadPreferences();
  });

  // The default entity is a user-level preference (persisted via /me/preferences), so it lives here rather
  // than under a specific entity's settings.
  let defaultEntityId = $state('');
  let prefsLoading = $state(false);
  let prefsError = $state('');
  let prefsSaving = $state(false);
  let prefsSuccess = $state('');

  async function loadPreferences() {
    prefsLoading = true;
    prefsError = '';
    try {
      const prefs = await apiFetch<{ default_entity_id?: string }>('/me/preferences');
      defaultEntityId = prefs.default_entity_id ?? '';
    } catch (err) {
      prefsError = errorMessage(err, 'Unable to load preferences.');
    } finally {
      prefsLoading = false;
    }
  }

  async function savePreferences() {
    prefsSaving = true;
    prefsError = '';
    prefsSuccess = '';
    try {
      await apiFetch('/me/preferences', {
        method: 'PATCH',
        body: { default_entity_id: defaultEntityId }
      });
      prefsSuccess = 'Preferences updated.';
    } catch (err) {
      prefsError = errorMessage(err, 'Unable to update preferences.');
    } finally {
      prefsSaving = false;
    }
  }

  let logoutAllLoading = $state(false);
  let logoutAllError = $state('');
  let logoutAllSuccess = $state('');
  let mfaLoading = $state(false);
  let mfaError = $state('');
  let mfaSuccess = $state('');
  let mfaConfigured = $state(false);
  let mfaEnabled = $state(false);
  let mfaSecret = $state('');
  let mfaUri = $state('');
  let mfaRecoveryCodes = $state<string[]>([]);
  let mfaCode = $state('');

  type ThemePreference = 'light' | 'dark' | 'system';

  async function logoutAllSessions() {
    logoutAllError = '';
    logoutAllSuccess = '';
    logoutAllLoading = true;
    try {
      await apiFetch('/auth/logout-all', { method: 'POST' });
      clearSession();
      logoutAllSuccess = 'Signed out of all sessions.';
    } catch (err) {
      logoutAllError = errorMessage(err, 'Unable to sign out all sessions.');
    } finally {
      logoutAllLoading = false;
    }
  }

  function handleThemeChange(event: Event) {
    const value = (event.currentTarget as HTMLSelectElement).value as ThemePreference;
    setTheme(value);
  }

  async function loadMfaStatus() {
    mfaLoading = true;
    mfaError = '';
    try {
      const response = await apiFetch<{
        configured?: boolean;
        enabled?: boolean;
      }>('/me/mfa');
      mfaConfigured = response.configured ?? false;
      mfaEnabled = response.enabled ?? false;
    } catch (err) {
      mfaError = errorMessage(err, 'Unable to load MFA status.');
    } finally {
      mfaLoading = false;
    }
  }

  async function beginMfaSetup() {
    mfaLoading = true;
    mfaError = '';
    mfaSuccess = '';
    try {
      const response = await apiFetch<{
        configured?: boolean;
        enabled?: boolean;
        secret?: string;
        uri?: string;
        recovery_codes?: string[];
      }>('/me/mfa/setup', {
        method: 'POST'
      });
      mfaConfigured = response.configured ?? true;
      mfaEnabled = response.enabled ?? false;
      mfaSecret = response.secret ?? '';
      mfaUri = response.uri ?? '';
      mfaRecoveryCodes = response.recovery_codes ?? [];
      mfaCode = '';
      mfaSuccess = 'Scan the secret in your authenticator app, then confirm the code below.';
    } catch (err) {
      mfaError = errorMessage(err, 'Unable to start MFA setup.');
    } finally {
      mfaLoading = false;
    }
  }

  async function confirmMfaSetup() {
    mfaLoading = true;
    mfaError = '';
    mfaSuccess = '';
    try {
      const response = await apiFetch<{
        configured?: boolean;
        enabled?: boolean;
      }>('/me/mfa/confirm', {
        method: 'POST',
        body: { code: mfaCode }
      });
      mfaConfigured = response.configured ?? true;
      mfaEnabled = response.enabled ?? true;
      mfaSecret = '';
      mfaUri = '';
      mfaRecoveryCodes = [];
      mfaCode = '';
      mfaSuccess = 'MFA is enabled for this account.';
    } catch (err) {
      mfaError = errorMessage(err, 'Unable to confirm MFA.');
    } finally {
      mfaLoading = false;
    }
  }

  async function disableMfa() {
    mfaLoading = true;
    mfaError = '';
    mfaSuccess = '';
    try {
      await apiFetch('/me/mfa', { method: 'DELETE' });
      mfaConfigured = false;
      mfaEnabled = false;
      mfaSecret = '';
      mfaUri = '';
      mfaRecoveryCodes = [];
      mfaCode = '';
      mfaSuccess = 'MFA has been disabled for this account.';
    } catch (err) {
      mfaError = errorMessage(err, 'Unable to disable MFA.');
    } finally {
      mfaLoading = false;
    }
  }
</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/settings">← Back to settings</a>

  <div>
    <h1 class="text-2xl font-semibold tracking-tight">User settings</h1>
    <p class="mt-2 text-sm text-muted">Personal preferences and local configuration.</p>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Profile</h2>
    {#if $currentUser}
      <div class="mt-4 grid gap-3 text-sm text-muted md:grid-cols-3">
        <div class="grid gap-1 rounded-xl border border-line px-4 py-3">
          <span class="text-xs uppercase tracking-[0.14em] text-muted">Email</span>
          <span class="truncate text-sm font-semibold text-ink">{$currentUser.email}</span>
        </div>
        <div class="grid gap-1 rounded-xl border border-line px-4 py-3">
          <span class="text-xs uppercase tracking-[0.14em] text-muted">Role</span>
          <span class="text-sm font-semibold text-ink">{$currentUser.is_admin ? 'Administrator' : 'Member'}</span>
        </div>
        {#if $currentUser.created_at}
          <div class="grid gap-1 rounded-xl border border-line px-4 py-3">
            <span class="text-xs uppercase tracking-[0.14em] text-muted">Member since</span>
            <span class="text-sm font-semibold text-ink">{formatDate($currentUser.created_at)}</span>
          </div>
        {/if}
      </div>
    {:else}
      <p class="mt-4 text-sm text-muted">Loading your profile…</p>
    {/if}
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Default entity</h2>
    <p class="mt-2 text-sm text-muted">The entity selected for you when you sign in.</p>
    {#if prefsError}
      <p class="mt-4 status-message-sm status-error">{prefsError}</p>
    {/if}
    {#if prefsSuccess}
      <p class="mt-4 status-message-sm status-success">{prefsSuccess}</p>
    {/if}
    {#if prefsLoading}
      <p class="mt-4 text-sm text-muted">Loading preferences…</p>
    {:else}
      <div class="mt-4 grid gap-3 text-sm text-muted sm:max-w-md">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Default entity
          <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={defaultEntityId}>
            <option value="">No default</option>
            {#each $entities as entity}
              <option value={entity.id}>{entity.name}</option>
            {/each}
          </select>
        </label>
        <div>
          <button
            class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
            type="button"
            disabled={prefsSaving}
            onclick={savePreferences}
          >
            {prefsSaving ? 'Saving…' : 'Save preferences'}
          </button>
        </div>
      </div>
    {/if}
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Preferences</h2>
    <p class="mt-2 text-sm text-muted">
      Theme follows your system setting by default and is stored locally in this browser when you override it.
    </p>
    <div class="mt-4 grid gap-3 text-sm text-muted">
      <label class="grid gap-2 rounded-xl border border-line px-4 py-3">
        <span class="text-sm font-medium text-ink">Theme</span>
        <select
          class="rounded-xl border border-line px-3 py-2 text-base"
          value={$theme}
          onchange={handleThemeChange}
        >
          <option value="system">System</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </label>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Security</h2>
    <p class="mt-2 text-sm text-muted">End every active session for this account.</p>
    {#if logoutAllError}
      <p class="mt-4 status-message-sm status-error">
        {logoutAllError}
      </p>
    {/if}
    {#if logoutAllSuccess}
      <p class="mt-4 status-message-sm status-success">
        {logoutAllSuccess}
      </p>
    {/if}
    <div class="mt-4">
      <button
        class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold disabled:opacity-60"
        type="button"
        disabled={logoutAllLoading}
        onclick={logoutAllSessions}
      >
        {logoutAllLoading ? 'Signing out…' : 'Sign out all sessions'}
      </button>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">MFA</h2>
    <p class="mt-2 text-sm text-muted">
      Set up an authenticator app to protect sign-in on this account.
    </p>
    {#if mfaError}
      <p class="mt-4 status-message-sm status-error">
        {mfaError}
      </p>
    {/if}
    {#if mfaSuccess}
      <p class="mt-4 status-message-sm status-success">
        {mfaSuccess}
      </p>
    {/if}
    <div class="mt-4 grid gap-3 text-sm text-muted">
      <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
        <span>Status</span>
        <span class="font-semibold text-ink">{mfaEnabled ? 'Enabled' : mfaConfigured ? 'Pending confirmation' : 'Not set up'}</span>
      </div>
      {#if mfaSecret}
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.2em] text-muted">Secret</p>
          <p class="mt-2 font-mono text-sm text-ink break-all">{mfaSecret}</p>
        </div>
      {/if}
      {#if mfaUri}
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.2em] text-muted">Provisioning URI</p>
          <p class="mt-2 break-all font-mono text-xs text-ink">{mfaUri}</p>
        </div>
      {/if}
      {#if mfaRecoveryCodes.length > 0}
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.2em] text-muted">Recovery codes</p>
          <div class="mt-2 grid gap-1 font-mono text-sm text-ink">
            {#each mfaRecoveryCodes as code}
              <span>{code}</span>
            {/each}
          </div>
          <p class="mt-2 text-xs text-muted">
            Save these somewhere safe. Each code can be used once if your authenticator is unavailable.
          </p>
        </div>
      {/if}
      <label class="grid gap-2 rounded-xl border border-line px-4 py-3">
        <span class="text-sm font-medium text-ink">Verification code</span>
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          type="text"
          inputmode="numeric"
          autocomplete="one-time-code"
          bind:value={mfaCode}
          disabled={mfaLoading}
        />
      </label>
    </div>
    <div class="mt-4 flex flex-wrap gap-2">
      <button
        class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
        type="button"
        disabled={mfaLoading}
        onclick={beginMfaSetup}
      >
        {mfaConfigured ? 'Regenerate setup' : 'Set up MFA'}
      </button>
      <button
        class="rounded-full border border-line px-4 py-2 text-sm font-semibold disabled:opacity-60"
        type="button"
        disabled={mfaLoading || !mfaSecret || !mfaCode}
        onclick={confirmMfaSetup}
      >
        Confirm code
      </button>
      <button
        class="rounded-full border border-line px-4 py-2 text-sm font-semibold disabled:opacity-60"
        type="button"
        disabled={mfaLoading || !mfaConfigured}
        onclick={disableMfa}
      >
        Disable MFA
      </button>
    </div>
  </div>
</section>

