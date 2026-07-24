<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatShortDateYear as formatDate } from '$lib/utils/date';
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api/client';
  import AdminGuard from '$lib/components/AdminGuard.svelte';

  type UserRow = {
    id: string;
    email: string;
    is_admin: boolean;
    mfa_enabled: boolean;
    mfa_configured: boolean;
    created_at: string;
  };

  let users: UserRow[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let actionError = $state('');
  let actionSuccess = $state('');
  let resettingId = $state('');
  let confirmResetId = $state('');

  async function loadUsers() {
    loading = true;
    error = '';
    try {
      users = await apiFetch<UserRow[]>('/users');
    } catch (err) {
      error = errorMessage(err, 'Unable to load users.');
    } finally {
      loading = false;
    }
  }

  async function resetMfa(user: UserRow) {
    actionError = '';
    actionSuccess = '';
    confirmResetId = '';
    resettingId = user.id;
    try {
      await apiFetch(`/users/${user.id}/mfa/reset`, { method: 'POST' });
      users = users.map((row) =>
        row.id === user.id
          ? { ...row, mfa_enabled: false, mfa_configured: false }
          : row
      );
      actionSuccess = `MFA reset for ${user.email}. They will need to re-enroll.`;
    } catch (err) {
      actionError = errorMessage(err, 'Unable to reset MFA.');
    } finally {
      resettingId = '';
    }
  }

  onMount(() => {
    void loadUsers();
  });
</script>

<AdminGuard>
<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/settings">← Back to settings</a>

  <div>
    <h1 class="text-2xl font-semibold tracking-tight">Users</h1>
    <p class="mt-2 text-sm text-muted">Admin access, MFA state, and recovery controls.</p>
  </div>

  {#if error}
    <p class="status-message-sm status-error">{error}</p>
  {/if}
  {#if actionError}
    <p class="status-message-sm status-error">{actionError}</p>
  {/if}
  {#if actionSuccess}
    <p class="status-message-sm status-success">{actionSuccess}</p>
  {/if}

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold">User accounts</h2>
        <p class="mt-2 text-sm text-muted">Reset MFA to clear the current enrollment and force a new setup.</p>
      </div>
      <button
        class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
        type="button"
        onclick={loadUsers}
        disabled={loading}
      >
        {loading ? 'Refreshing…' : 'Refresh'}
      </button>
    </div>

    {#if loading}
      <p class="mt-4 text-sm text-muted">Loading users…</p>
    {:else if users.length === 0}
      <p class="mt-4 text-sm text-muted">No users found.</p>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each users as user}
          <div class="grid gap-3 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.6fr_0.8fr_0.8fr_0.7fr] md:items-center">
            <div>
              <p class="text-sm font-semibold text-ink">{user.email}</p>
              <p class="mt-1 text-xs text-muted">{formatDate(user.created_at)}</p>
            </div>
            <div class="text-sm text-muted">
              <span class="status-pill">{user.is_admin ? 'Admin' : 'User'}</span>
            </div>
            <div class="text-sm text-muted">
              <span class="status-pill">
                {user.mfa_enabled ? 'MFA enabled' : user.mfa_configured ? 'MFA pending' : 'MFA off'}
              </span>
            </div>
            <div class="flex justify-end gap-2">
              {#if confirmResetId === user.id}
                <button
                  class="rounded-full border status-error px-4 py-2 text-sm font-semibold text-error disabled:opacity-60"
                  type="button"
                  disabled={resettingId === user.id}
                  onclick={() => resetMfa(user)}
                >
                  {resettingId === user.id ? 'Resetting…' : 'Confirm reset'}
                </button>
                <button
                  class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
                  type="button"
                  onclick={() => (confirmResetId = '')}
                >
                  Cancel
                </button>
              {:else}
                <button
                  class="rounded-full border border-line px-4 py-2 text-sm font-semibold disabled:opacity-60"
                  type="button"
                  disabled={resettingId === user.id}
                  onclick={() => (confirmResetId = user.id)}
                >
                  Reset MFA
                </button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</section>
</AdminGuard>
