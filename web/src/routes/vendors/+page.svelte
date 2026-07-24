<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { activeEntity } from '$lib/stores/entity';
  import { apiFetch } from '$lib/api/client';
  import type { Account } from '$lib/api/types';
  import { errorRecoveryHint } from '$lib/utils/error-hints';

  type Vendor = {
    id: string;
    entity_id: string;
    name: string;
    normalized_name: string;
    match_pattern?: string;
    tax_id?: string;
    website?: string;
    default_account_id?: string;
    receipt_count?: number;
    last_seen?: string;
  };

  type AccountRow = Pick<Account, 'id' | 'name'>;

  let vendors: Vendor[] = $state([]);
  let accounts: Record<string, AccountRow> = $state({});
  let loading = $state(false);
  let error = $state('');

  let editingId = $state('');
  let editName = $state('');
  let editPattern = $state('');
  let editAccountId = $state('');
  let editTaxId = $state('');
  let editWebsite = $state('');
  let editError = $state('');
  let editSaving = $state(false);

  let createName = $state('');
  let createPattern = $state('');
  let createAccountId = $state('');
  let createTaxId = $state('');
  let createWebsite = $state('');
  let createError = $state('');
  let creating = $state(false);

  // Ensure a stored website opens as an absolute URL (vendors may be saved without a scheme).
  function websiteHref(url: string) {
    return /^https?:\/\//i.test(url) ? url : `https://${url}`;
  }

  async function loadVendors() {
    if (!$activeEntity) {
      vendors = [];
      return;
    }
    loading = true;
    error = '';
    try {
      const [vendorsResp, accountsResp] = await Promise.all([
        apiFetch<{ rows: Vendor[] }>(`/vendors?entity_id=${encodeURIComponent($activeEntity)}`),
        apiFetch<AccountRow[]>(`/entities/${encodeURIComponent($activeEntity)}/accounts?limit=1000`)
      ]);
      vendors = vendorsResp.rows ?? [];
      accounts = {};
      for (const row of accountsResp ?? []) {
        accounts[row.id] = row;
      }
    } catch (err) {
      error = errorMessage(err, 'Unable to load vendors.');
    } finally {
      loading = false;
    }
  }

  async function createVendor() {
    if (!$activeEntity) {
      return;
    }
    createError = '';
    if (!createName.trim()) {
      createError = 'Vendor name is required.';
      return;
    }
    creating = true;
    try {
      await apiFetch('/vendors', {
        method: 'POST',
        body: {
          entity_id: $activeEntity,
          name: createName.trim(),
          match_pattern: createPattern.trim(),
          default_account_id: createAccountId,
          tax_id: createTaxId.trim(),
          website: createWebsite.trim()
        }
      });
      createName = '';
      createPattern = '';
      createAccountId = '';
      createTaxId = '';
      createWebsite = '';
      await loadVendors();
    } catch (err) {
      createError = errorMessage(err, 'Unable to create vendor.');
    } finally {
      creating = false;
    }
  }

  async function deleteVendor(id: string) {
    try {
      await apiFetch(`/vendors/${id}`, { method: 'DELETE' });
      vendors = vendors.filter((vendor) => vendor.id !== id);
    } catch (err) {
      error = errorMessage(err, 'Unable to delete vendor.');
    }
  }

  function startEdit(vendor: Vendor) {
    editingId = vendor.id;
    editName = vendor.name;
    editPattern = vendor.match_pattern ?? '';
    editAccountId = vendor.default_account_id ?? '';
    editTaxId = vendor.tax_id ?? '';
    editWebsite = vendor.website ?? '';
    editError = '';
  }

  function cancelEdit() {
    editingId = '';
    editError = '';
  }

  async function saveEdit() {
    if (!editingId || !$activeEntity) {
      return;
    }
    editError = '';
    if (!editName.trim()) {
      editError = 'Vendor name is required.';
      return;
    }
    editSaving = true;
    try {
      const updated = await apiFetch<Vendor>(`/vendors/${editingId}`, {
        method: 'PATCH',
        body: {
          entity_id: $activeEntity,
          name: editName.trim(),
          match_pattern: editPattern.trim(),
          default_account_id: editAccountId,
          tax_id: editTaxId.trim(),
          website: editWebsite.trim()
        }
      });
      vendors = vendors.map((vendor) => (vendor.id === editingId ? updated : vendor));
      cancelEdit();
    } catch (err) {
      editError = errorMessage(err, 'Unable to update vendor.');
    } finally {
      editSaving = false;
    }
  }

  $effect(() => {
    if ($activeEntity) {
      loadVendors();
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Vendors</h1>
      <p class="mt-2 text-sm text-muted">
        Vendors the receipt pipeline has learned, plus any you add. A vendor's default account is reused
        automatically when a receipt matches it.
      </p>
    </div>
    <div class="rounded-full border border-line px-4 py-2 text-xs font-semibold text-muted">
      {vendors.length} vendor(s)
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Known vendors</h2>
      <span class="text-xs uppercase tracking-[0.2em] text-muted">Live</span>
    </div>
    {#if error}
      <div class="mt-4 status-message-sm status-error">
        <p>{error}</p>
        {#if errorRecoveryHint(error)}
          <p class="mt-1 text-xs">{errorRecoveryHint(error)}</p>
        {/if}
      </div>
    {:else if loading}
      <p class="mt-4 text-sm text-muted">Loading vendors…</p>
    {:else if vendors.length === 0}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        {#if !$activeEntity}
          <p class="font-semibold text-ink">Select an entity to manage vendors.</p>
          <p class="mt-2">Vendors are scoped to the active entity only.</p>
        {:else}
          <p class="font-semibold text-ink">No vendors yet.</p>
          <p class="mt-2">The pipeline creates a vendor the first time it processes a receipt, or add one below.</p>
        {/if}
      </div>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each vendors as vendor}
          <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.6fr_1fr_1fr_0.9fr] md:items-center">
            <div>
              {#if editingId === vendor.id}
                <label class="grid gap-2 text-xs font-semibold text-muted">
                  Name
                  <input class="rounded-xl border border-line px-3 py-2 text-sm" type="text" bind:value={editName} />
                </label>
                <label class="mt-2 grid gap-2 text-xs font-semibold text-muted">
                  Match pattern
                  <input class="rounded-xl border border-line px-3 py-2 text-sm" type="text" bind:value={editPattern} />
                </label>
                <label class="mt-2 grid gap-2 text-xs font-semibold text-muted">
                  Tax ID
                  <input class="rounded-xl border border-line px-3 py-2 text-sm" type="text" bind:value={editTaxId} />
                </label>
                <label class="mt-2 grid gap-2 text-xs font-semibold text-muted">
                  Website
                  <input class="rounded-xl border border-line px-3 py-2 text-sm" type="text" bind:value={editWebsite} />
                </label>
              {:else}
                <p class="text-sm font-semibold">{vendor.name}</p>
                {#if vendor.match_pattern}
                  <p class="text-xs text-muted">Matches “{vendor.match_pattern}”</p>
                {/if}
                {#if vendor.website}
                  <a
                    class="text-xs text-primary hover:underline"
                    href={websiteHref(vendor.website)}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {vendor.website}
                  </a>
                {/if}
              {/if}
            </div>
            <div class="text-sm text-muted">
              {#if editingId === vendor.id}
                <label class="grid gap-2 text-xs font-semibold text-muted">
                  Default account
                  <select class="rounded-xl border border-line px-3 py-2 text-sm" bind:value={editAccountId}>
                    <option value="">No default</option>
                    {#each Object.values(accounts) as account}
                      <option value={account.id}>{account.name}</option>
                    {/each}
                  </select>
                </label>
              {:else if vendor.default_account_id}
                {accounts[vendor.default_account_id]?.name ?? vendor.default_account_id}
              {:else}
                <span class="text-xs">No default account</span>
              {/if}
            </div>
            <div class="text-sm text-muted">
              {#if vendor.receipt_count}
                Seen {vendor.receipt_count}×
              {:else}
                <span class="text-xs">Not yet seen</span>
              {/if}
              {#if vendor.last_seen}
                <span class="block text-xs">{vendor.last_seen.slice(0, 10)}</span>
              {/if}
            </div>
            <div class="flex gap-2">
              {#if editingId === vendor.id}
                <button
                  class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
                  type="button"
                  disabled={editSaving}
                  onclick={saveEdit}
                >
                  {editSaving ? 'Saving…' : 'Save'}
                </button>
                <button class="rounded-full border border-line px-4 py-2 text-sm font-semibold" type="button" onclick={cancelEdit}>
                  Cancel
                </button>
              {:else}
                <button class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold" type="button" onclick={() => startEdit(vendor)}>
                  Edit
                </button>
                <button class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold" type="button" onclick={() => deleteVendor(vendor.id)}>
                  Delete
                </button>
              {/if}
            </div>
          </div>
          {#if editingId === vendor.id && editError}
            <div class="status-message-xs status-error">
              <p>{editError}</p>
              {#if errorRecoveryHint(editError)}
                <p class="mt-1">{errorRecoveryHint(editError)}</p>
              {/if}
            </div>
          {/if}
        {/each}
      </div>
    {/if}
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6 md:max-w-xl">
    <h2 class="text-lg font-semibold">Add vendor</h2>
    <p class="mt-2 text-sm text-muted">
      Add a vendor ahead of time so its first receipt matches and files to the right account.
    </p>
    <form class="mt-4 grid gap-3 text-sm" onsubmit={(event) => { event.preventDefault(); void createVendor(); }}>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Name
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="text" placeholder="Blue Bottle Coffee" bind:value={createName} />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Match pattern
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="text" placeholder="BLUE BOTTLE" bind:value={createPattern} />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Default account
        <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={createAccountId}>
          <option value="">No default</option>
          {#each Object.values(accounts) as account}
            <option value={account.id}>{account.name}</option>
          {/each}
        </select>
      </label>
      <div class="grid gap-3 md:grid-cols-2">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Tax ID
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="text" bind:value={createTaxId} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Website
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="text" bind:value={createWebsite} />
        </label>
      </div>
      {#if createError}
        <div class="status-message-sm status-error">
          <p>{createError}</p>
          {#if errorRecoveryHint(createError)}
            <p class="mt-1 text-xs">{errorRecoveryHint(createError)}</p>
          {/if}
        </div>
      {/if}
      <button
        class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
        type="submit"
        disabled={creating || !$activeEntity}
      >
        {creating ? 'Saving…' : 'Add vendor'}
      </button>
      {#if !$activeEntity}
        <p class="text-xs text-muted">Select an entity to add vendors.</p>
      {/if}
    </form>
  </div>
</section>
