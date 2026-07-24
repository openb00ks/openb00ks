<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { activeEntity } from '$lib/stores/entity';
  import { apiFetch } from '$lib/api/client';
  import type { Account } from '$lib/api/types';
  import { errorRecoveryHint } from '$lib/utils/error-hints';

  type VendorRule = {
    id: string;
    entity_id: string;
    match_type: string;
    pattern: string;
    account_id: string;
    created_at: string;
  };

  type AccountRow = Pick<Account, 'id' | 'name'>;

  let rules: VendorRule[] = $state([]);
  let accounts: Record<string, AccountRow> = $state({});
  let loading = $state(false);
  let error = $state('');
  let editingId = $state('');
  let editMatchType = $state('contains');
  let editPattern = $state('');
  let editAccountId = $state('');
  let editError = $state('');
  let editSaving = $state(false);
  let previousRule: VendorRule | null = null;

  let createMatchType = $state('contains');
  let createPattern = $state('');
  let createAccountId = $state('');
  let createError = $state('');
  let creating = $state(false);

  async function loadRules() {
    if (!$activeEntity) {
      rules = [];
      return;
    }
    loading = true;
    error = '';
    try {
      const [rulesResp, accountsResp] = await Promise.all([
        apiFetch<{ rows: VendorRule[] }>(
          `/vendor-rules?entity_id=${encodeURIComponent($activeEntity)}`
        ),
        // The accounts endpoint returns a bare array, not { rows }.
        apiFetch<AccountRow[]>(
          `/entities/${encodeURIComponent($activeEntity)}/accounts?limit=1000`
        )
      ]);
      rules = rulesResp.rows ?? [];
      accounts = {};
      for (const row of accountsResp ?? []) {
        accounts[row.id] = row;
      }
    } catch (err) {
      error = errorMessage(err, 'Unable to load rules.');
    } finally {
      loading = false;
    }
  }

  async function createRule() {
    if (!$activeEntity) {
      return;
    }
    createError = '';
    if (!createPattern.trim() || !createAccountId) {
      createError = 'Match pattern and account are required.';
      return;
    }
    creating = true;
    try {
      await apiFetch('/vendor-rules', {
        method: 'POST',
        body: {
          entity_id: $activeEntity,
          match_type: createMatchType,
          pattern: createPattern.trim(),
          account_id: createAccountId
        }
      });
      createPattern = '';
      createAccountId = '';
      await loadRules();
    } catch (err) {
      createError = errorMessage(err, 'Unable to create rule.');
    } finally {
      creating = false;
    }
  }

  async function deleteRule(id: string) {
    if (!$activeEntity) {
      return;
    }
    try {
      await apiFetch(`/vendor-rules/${id}?entity_id=${encodeURIComponent($activeEntity)}`, {
        method: 'DELETE'
      });
      rules = rules.filter((rule) => rule.id !== id);
    } catch (err) {
      error = errorMessage(err, 'Unable to delete rule.');
    }
  }

  function startEdit(rule: VendorRule) {
    editingId = rule.id;
    editMatchType = rule.match_type;
    editPattern = rule.pattern;
    editAccountId = rule.account_id;
    editError = '';
  }

  function cancelEdit() {
    editingId = '';
    editPattern = '';
    editAccountId = '';
    editMatchType = 'contains';
    editError = '';
  }

  async function saveEdit() {
    if (!editingId || !$activeEntity) {
      return;
    }
    editError = '';
    if (!editPattern.trim() || !editAccountId) {
      editError = 'Match pattern and account are required.';
      return;
    }
    editSaving = true;
    previousRule = rules.find((rule) => rule.id === editingId) ?? null;
    const nextPattern = editPattern.trim();
    const nextMatchType = editMatchType;
    const nextAccountId = editAccountId;
    rules = rules.map((rule) =>
      rule.id === editingId
        ? {
            ...rule,
            match_type: nextMatchType,
            pattern: nextPattern,
            account_id: nextAccountId
          }
        : rule
    );
    try {
      await apiFetch(`/vendor-rules/${editingId}`, {
        method: 'PATCH',
        body: {
          entity_id: $activeEntity,
          match_type: nextMatchType,
          pattern: nextPattern,
          account_id: nextAccountId
        }
      });
      cancelEdit();
    } catch (err) {
      if (previousRule) {
        rules = rules.map((rule) =>
          rule.id === previousRule?.id ? previousRule : rule
        );
      }
      editError = errorMessage(err, 'Unable to update rule.');
    } finally {
      editSaving = false;
      previousRule = null;
    }
  }

  $effect(() => {
    if ($activeEntity) {
      loadRules();
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Vendor rules</h1>
      <p class="mt-2 text-sm text-muted">Map recurring vendors to accounts and entities.</p>
    </div>
    <div class="rounded-full border border-line px-4 py-2 text-xs font-semibold text-muted">
      Use the form below to add rules
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Rules</h2>
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
      <p class="mt-4 text-sm text-muted">Loading rules…</p>
    {:else if rules.length === 0}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        {#if !$activeEntity}
          <p class="font-semibold text-ink">Select an entity to manage vendor rules.</p>
          <p class="mt-2">Vendor rules are applied within the active entity only.</p>
        {:else}
          <p class="font-semibold text-ink">No vendor rules yet.</p>
          <p class="mt-2">Create a rule for a recurring merchant to speed up future suggestions and reviews.</p>
        {/if}
      </div>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each rules as rule}
          <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.4fr_1fr_1fr_0.9fr] md:items-center">
            <div>
              {#if editingId === rule.id}
                <label class="grid gap-2 text-xs font-semibold text-muted">
                  Match type
                  <select
                    class="rounded-xl border border-line px-3 py-2 text-sm"
                    bind:value={editMatchType}
                  >
                    <option value="contains">Contains</option>
                    <option value="exact">Exact</option>
                  </select>
                </label>
                <label class="mt-2 grid gap-2 text-xs font-semibold text-muted">
                  Pattern
                  <input
                    class="rounded-xl border border-line px-3 py-2 text-sm"
                    type="text"
                    bind:value={editPattern}
                  />
                </label>
              {:else}
                <p class="text-sm font-semibold">{rule.pattern}</p>
                <p class="text-xs text-muted">Match {rule.match_type}</p>
              {/if}
            </div>
            <div class="text-sm text-muted">
              {#if editingId === rule.id}
                <label class="grid gap-2 text-xs font-semibold text-muted">
                  Account
                  <select
                    class="rounded-xl border border-line px-3 py-2 text-sm"
                    bind:value={editAccountId}
                  >
                    <option value="">Select account</option>
                    {#each Object.values(accounts) as account}
                      <option value={account.id}>{account.name}</option>
                    {/each}
                  </select>
                </label>
              {:else}
                {accounts[rule.account_id]?.name ?? rule.account_id}
              {/if}
            </div>
            <div class="text-sm text-muted">{rule.created_at.slice(0, 10)}</div>
            <div class="flex gap-2">
              {#if editingId === rule.id}
                <button
                  class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
                  type="button"
                  disabled={editSaving}
                  onclick={saveEdit}
                >
                  {editSaving ? 'Saving…' : 'Save'}
                </button>
                <button
                  class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
                  type="button"
                  onclick={cancelEdit}
                >
                  Cancel
                </button>
              {:else}
                <button
                  class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold"
                  type="button"
                  onclick={() => startEdit(rule)}
                >
                  Edit
                </button>
                <button
                  class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold"
                  type="button"
                  onclick={() => deleteRule(rule.id)}
                >
                  Delete
                </button>
              {/if}
            </div>
          </div>
          {#if editingId === rule.id && editError}
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

  <div class="grid gap-4 md:grid-cols-2">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Create rule</h2>
      <form class="mt-4 grid gap-3 text-sm" onsubmit={(event) => { event.preventDefault(); void createRule(); }}>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Match type
          <select
            class="rounded-xl border border-line px-3 py-2 text-base"
            bind:value={createMatchType}
          >
            <option value="contains">Contains</option>
            <option value="exact">Exact</option>
          </select>
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Pattern
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            placeholder="blue bottle"
            bind:value={createPattern}
          />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Account
          <select
            class="rounded-xl border border-line px-3 py-2 text-base"
            bind:value={createAccountId}
          >
            <option value="">Select account</option>
            {#each Object.values(accounts) as account}
              <option value={account.id}>{account.name}</option>
            {/each}
          </select>
        </label>
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
          {creating ? 'Saving…' : 'Create rule'}
        </button>
        {#if !$activeEntity}
          <p class="text-xs text-muted">Select an entity to create rules.</p>
        {/if}
      </form>
    </div>
  </div>
</section>

