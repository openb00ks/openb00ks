<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api/client';
  import AdminGuard from '$lib/components/AdminGuard.svelte';

  type SettingsResponse = {
    settings: {
      require_mfa: boolean;
      enforce_session_timeout: boolean;
    };
    integrations: {
      ai_provider: string;
      ai_model: string;
      receipt_storage: string;
      receipt_local_dir: string;
      receipt_max_bytes: number;
    };
    updated_at?: string;
  };

  let loading = $state(false);
  let error = $state('');
  let saving = $state(false);
  let success = $state('');

  let requireMFA = $state(false);
  let enforceSessionTimeout = $state(false);

  let integrations = $state({
    ai_provider: '',
    ai_model: '',
    receipt_storage: '',
    receipt_local_dir: '',
    receipt_max_bytes: 0
  });

  function formatBytes(size: number) {
    if (!size) {
      return '—';
    }
    if (size < 1024) {
      return `${size} B`;
    }
    if (size < 1024 * 1024) {
      return `${(size / 1024).toFixed(1)} KB`;
    }
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }

  async function loadSettings() {
    loading = true;
    error = '';
    try {
      const response = await apiFetch<SettingsResponse>('/settings/system');
      requireMFA = response.settings?.require_mfa ?? false;
      enforceSessionTimeout = response.settings?.enforce_session_timeout ?? false;
      integrations = response.integrations ?? integrations;
    } catch (err) {
      error = errorMessage(err, 'Unable to load system settings.');
    } finally {
      loading = false;
    }
  }

  async function updateSetting(payload: { require_mfa?: boolean; enforce_session_timeout?: boolean }) {
    saving = true;
    success = '';
    error = '';
    try {
      const response = await apiFetch<SettingsResponse>('/settings/system', {
        method: 'PATCH',
        body: payload
      });
      requireMFA = response.settings?.require_mfa ?? requireMFA;
      enforceSessionTimeout =
        response.settings?.enforce_session_timeout ?? enforceSessionTimeout;
      success = 'Settings updated.';
    } catch (err) {
      error = errorMessage(err, 'Unable to update system settings.');
    } finally {
      saving = false;
    }
  }

  onMount(() => {
    loadSettings();
  });
</script>

<AdminGuard>
<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/settings">← Back to settings</a>

  <div>
    <h1 class="text-2xl font-semibold tracking-tight">System settings</h1>
    <p class="mt-2 text-sm text-muted">Platform-wide configuration (system admins only).</p>
  </div>

  {#if error}
    <p class="status-message-sm status-error">
      {error}
    </p>
  {/if}
  {#if success}
    <p class="status-message-sm status-success">
      {success}
    </p>
  {/if}

  <div class="grid gap-4 md:grid-cols-2">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Security</h2>
      {#if loading}
        <p class="mt-4 text-sm text-muted">Loading settings…</p>
      {:else}
        <div class="mt-4 grid gap-3 text-sm text-muted">
          <label class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
            <span>Require MFA</span>
            <input
              type="checkbox"
              class="h-4 w-4"
              checked={requireMFA}
              disabled={saving}
              onchange={(event) => {
                requireMFA = event.currentTarget.checked;
                updateSetting({ require_mfa: requireMFA });
              }}
            />
          </label>
          <label class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
            <span>Enforce session timeout</span>
            <input
              type="checkbox"
              class="h-4 w-4"
              checked={enforceSessionTimeout}
              disabled={saving}
              onchange={(event) => {
                enforceSessionTimeout = event.currentTarget.checked;
                updateSetting({ enforce_session_timeout: enforceSessionTimeout });
              }}
            />
          </label>
        </div>
        {#if requireMFA}
          <div class="status-message-sm status-info">
            <p class="font-semibold text-ink">MFA is required.</p>
            <p class="mt-1 text-sm">
              Users without MFA configured will be blocked from signing in until they enroll.
              Use user management to reset or re-enroll accounts.
            </p>
            <a class="mt-2 inline-flex text-sm font-semibold text-ink underline" href="/users">
              Open user management
            </a>
          </div>
        {/if}
      {/if}
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Integrations</h2>
      <p class="mt-2 text-sm text-muted">Manage AI providers and storage.</p>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <div class="rounded-xl border border-line px-4 py-3">
          AI provider: {integrations.ai_provider || 'None'}
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          AI model: {integrations.ai_model || 'Default'}
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Receipt storage: {integrations.receipt_storage || 'Unknown'}
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Receipt local dir: {integrations.receipt_local_dir || '—'}
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Receipt max size: {formatBytes(integrations.receipt_max_bytes)}
        </div>
        <div class="status-message-sm status-info">
          <p class="font-semibold text-ink">Recovery path</p>
          <p class="mt-1 text-sm">
            Use user management to reset MFA when someone loses their authenticator or recovery codes.
          </p>
        </div>
      </div>
    </div>
  </div>
</section>
</AdminGuard>

