<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { apiFetch } from '$lib/api/client';
  import type { Account } from '$lib/api/types';
  import { activeEntity } from '$lib/stores/entity';
  import { errorRecoveryHint } from '$lib/utils/error-hints';

  type AccountRow = Account;

  const roleTagOptions = [
    { value: 'utilities', label: 'Utilities' },
    { value: 'cell_phone', label: 'Cell phone' },
    { value: 'internet', label: 'Home internet' },
  ];

  // The chart-of-accounts type is a fixed set; constrain it to a select so a typo can't create an
  // account the pipeline and reports can't classify.
  const accountTypeOptions = [
    { value: 'asset', label: 'Asset' },
    { value: 'liability', label: 'Liability' },
    { value: 'equity', label: 'Equity' },
    { value: 'income', label: 'Income' },
    { value: 'expense', label: 'Expense' },
  ];

  let accounts: AccountRow[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let showCreateForm = $state(false);
  let createName = $state('');
  let createType = $state('');
  let createCode = $state('');
  let createRoleTags = $state<string[]>([]);
  let createError = $state('');
  let createSuccess = $state('');
  let creating = $state(false);
  let deletingId = $state('');
  let deleteError = $state('');
  let editingId = $state('');
  let editName = $state('');
  let editType = $state('');
  let editCode = $state('');
  let editRoleTags = $state<string[]>([]);
  let editError = $state('');
  let editSaving = $state(false);
  let previousAccount: AccountRow | null = null;
  let balances = $state<Record<string, number>>({});

  const TYPE_ORDER = ['asset', 'liability', 'equity', 'income', 'expense', 'other'];
  const TYPE_LABELS: Record<string, string> = {
    asset: 'Assets',
    liability: 'Liabilities',
    equity: 'Equity',
    income: 'Income',
    expense: 'Expenses',
    other: 'Other',
  };

  // Group the chart of accounts into standard sections with a per-section balance subtotal. Accounts keep
  // their code order (the API already sorts by code) within each section.
  let groupedAccounts = $derived.by(() => {
    const groups = new Map<string, AccountRow[]>();
    for (const account of accounts) {
      const key = TYPE_LABELS[account.type] ? account.type : 'other';
      const bucket = groups.get(key) ?? [];
      bucket.push(account);
      groups.set(key, bucket);
    }
    return TYPE_ORDER.filter((key) => groups.has(key)).map((key) => {
      const items = groups.get(key) ?? [];
      const subtotal = items.reduce((sum, account) => sum + (balances[account.id] ?? 0), 0);
      return { key, label: TYPE_LABELS[key], accounts: items, subtotal };
    });
  });

  async function loadAccounts() {
    if (!$activeEntity) {
      accounts = [];
      balances = {};
      showCreateForm = false;
      return;
    }
    loading = true;
    error = '';
    try {
      // The accounts endpoint returns a bare array, not { rows }.
      const entity = encodeURIComponent($activeEntity);
      const [accountRows, balanceResp] = await Promise.all([
        apiFetch<AccountRow[]>(`/entities/${entity}/accounts`),
        apiFetch<{ balances: Array<{ account_id: string; balance_cents: number }> }>(
          `/entities/${entity}/account-balances`
        ).catch(() => ({ balances: [] })),
      ]);
      accounts = accountRows ?? [];
      const map: Record<string, number> = {};
      for (const row of balanceResp.balances ?? []) {
        map[row.account_id] = row.balance_cents;
      }
      balances = map;
    } catch (err) {
      error = errorMessage(err, 'Unable to load accounts.');
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if ($activeEntity) {
      loadAccounts();
    }
  });

  function startEdit(account: AccountRow) {
    createSuccess = '';
    deleteError = '';
    editingId = account.id;
    editName = account.name;
    editType = account.type;
    editCode = account.code ?? '';
    editRoleTags = [...(account.role_tags ?? [])];
    editError = '';
  }

  function cancelEdit() {
    editingId = '';
    editName = '';
    editType = '';
    editCode = '';
    editRoleTags = [];
    editError = '';
  }

  function toggleCreateForm() {
    showCreateForm = !showCreateForm;
    createError = '';
    createSuccess = '';
    if (!showCreateForm) {
      createName = '';
      createType = '';
      createCode = '';
      createRoleTags = [];
    }
  }

  function toggleCreateRoleTag(roleTag: string) {
    createRoleTags = createRoleTags.includes(roleTag)
      ? createRoleTags.filter((tag) => tag !== roleTag)
      : [...createRoleTags, roleTag];
  }

  function toggleEditRoleTag(roleTag: string) {
    editRoleTags = editRoleTags.includes(roleTag)
      ? editRoleTags.filter((tag) => tag !== roleTag)
      : [...editRoleTags, roleTag];
  }

  function formatRoleTag(roleTag: string) {
    switch (roleTag) {
      case 'utilities':
        return 'Utilities';
      case 'cell_phone':
        return 'Cell phone';
      case 'internet':
        return 'Home internet';
      default:
        return roleTag;
    }
  }

  async function createAccount() {
    createError = '';
    createSuccess = '';
    deleteError = '';
    if (!$activeEntity) {
      createError = 'Select an entity before creating an account.';
      return;
    }
    const nextName = createName.trim();
    const nextType = createType.trim();
    if (!nextName || !nextType) {
      createError = 'Name and type are required.';
      return;
    }
    creating = true;
    try {
      const created = await apiFetch<AccountRow>(
        `/entities/${encodeURIComponent($activeEntity)}/accounts`,
        {
          method: 'POST',
          body: { name: nextName, type: nextType, code: createCode.trim(), role_tags: createRoleTags }
        }
      );
      accounts = [created, ...accounts];
      createName = '';
      createType = '';
      createCode = '';
      createRoleTags = [];
      createSuccess = 'Account created.';
      showCreateForm = false;
    } catch (err) {
      createError = errorMessage(err, 'Unable to create account.');
    } finally {
      creating = false;
    }
  }

  async function saveEdit() {
    if (!editingId) {
      return;
    }
    editError = '';
    if (!editName.trim() || !editType.trim()) {
      editError = 'Name and type are required.';
      return;
    }
    editSaving = true;
    previousAccount = accounts.find((account) => account.id === editingId) ?? null;
    const nextName = editName.trim();
    const nextType = editType.trim();
    const nextCode = editCode.trim();
    accounts = accounts.map((account) =>
      account.id === editingId
        ? { ...account, name: nextName, type: nextType, code: nextCode, role_tags: [...editRoleTags] }
        : account
    );
    try {
      await apiFetch(`/accounts/${editingId}`, {
        method: 'PATCH',
        body: { name: nextName, type: nextType, code: nextCode, role_tags: editRoleTags }
      });
      cancelEdit();
    } catch (err) {
      if (previousAccount) {
        accounts = accounts.map((account) =>
          account.id === previousAccount?.id ? previousAccount : account
        );
      }
      editError = errorMessage(err, 'Unable to update account.');
    } finally {
      editSaving = false;
      previousAccount = null;
    }
  }

  async function deleteAccount(account: AccountRow) {
    deleteError = '';
    createSuccess = '';
    if (!account.id) {
      return;
    }
    deletingId = account.id;
    const previousAccounts = accounts;
    accounts = accounts.filter((row) => row.id !== account.id);
    try {
      await apiFetch(`/accounts/${account.id}`, {
        method: 'DELETE'
      });
    } catch (err) {
      accounts = previousAccounts;
      const message = errorMessage(err, '');
      deleteError =
        message === 'ACCOUNT_IN_USE'
          ? 'This account has transactions and can’t be deleted. Reassign or remove its entries first.'
          : message || 'Unable to delete account.';
    } finally {
      deletingId = '';
    }
  }
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Accounts</h1>
      <p class="mt-2 text-sm text-muted">Chart of accounts for the selected entity.</p>
    </div>
    <button
      class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
      type="button"
      disabled={!$activeEntity || creating}
      onclick={toggleCreateForm}
    >
      {showCreateForm ? 'Close form' : 'New account'}
    </button>
  </div>

  {#if showCreateForm}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">New account</h2>
      <div class="mt-4 grid gap-3 md:grid-cols-[0.5fr_1.5fr_1fr]">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Code
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            inputmode="numeric"
            placeholder="1000"
            bind:value={createCode}
            disabled={creating}
          />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Name
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            bind:value={createName}
            disabled={creating}
          />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Type
          <select
            class="rounded-xl border border-line px-3 py-2 text-base"
            bind:value={createType}
            disabled={creating}
          >
            <option value="">Select type</option>
            {#each accountTypeOptions as option}
              <option value={option.value}>{option.label}</option>
            {/each}
          </select>
        </label>
      </div>
      <div class="mt-4">
        <p class="text-sm font-medium text-ink">Role tags</p>
        <div class="mt-2 flex flex-wrap gap-2">
          {#each roleTagOptions as option}
            <label class="flex items-center gap-2 rounded-full border border-line px-3 py-2 text-sm text-ink">
              <input
                type="checkbox"
                checked={createRoleTags.includes(option.value)}
                onclick={() => toggleCreateRoleTag(option.value)}
              />
              <span>{option.label}</span>
            </label>
          {/each}
        </div>
      </div>
      {#if createError}
        <div class="mt-4 status-message-sm status-error">
          <p>{createError}</p>
          {#if errorRecoveryHint(createError)}
            <p class="mt-1 text-xs">{errorRecoveryHint(createError)}</p>
          {/if}
        </div>
      {/if}
      <div class="mt-4 flex gap-2">
        <button
          class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
          type="button"
          disabled={creating}
          onclick={createAccount}
        >
          {creating ? 'Creating…' : 'Create account'}
        </button>
        <button
          class="rounded-full border border-line px-5 py-2 text-sm font-semibold"
          type="button"
          disabled={creating}
          onclick={toggleCreateForm}
        >
          Cancel
        </button>
      </div>
    </div>
  {/if}

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Chart of accounts</h2>
      <span class="text-xs uppercase tracking-[0.2em] text-muted">Live</span>
    </div>
    {#if createSuccess}
      <p class="mt-4 status-message-sm status-success">
        {createSuccess}
      </p>
    {/if}
    {#if deleteError}
      <div class="mt-4 status-message-sm status-error">
        <p>{deleteError}</p>
        {#if errorRecoveryHint(deleteError)}
          <p class="mt-1 text-xs">{errorRecoveryHint(deleteError)}</p>
        {/if}
      </div>
    {/if}
    {#if error}
      <div class="mt-4 status-message-sm status-error">
        <p>{error}</p>
        {#if errorRecoveryHint(error)}
          <p class="mt-1 text-xs">{errorRecoveryHint(error)}</p>
        {/if}
      </div>
    {:else if loading}
      <p class="mt-4 text-sm text-muted">Loading accounts…</p>
    {:else if accounts.length === 0}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        {#if !$activeEntity}
          <p class="font-semibold text-ink">Select an entity to manage accounts.</p>
          <p class="mt-2">The chart of accounts is maintained separately for each entity.</p>
        {:else}
          <p class="font-semibold text-ink">No accounts yet.</p>
          <p class="mt-2">Create the first account so receipts and vendor rules have somewhere to post later.</p>
        {/if}
      </div>
    {:else}
      <div class="mt-4 grid gap-6">
        {#each groupedAccounts as group (group.key)}
          <div>
            <div class="flex items-center justify-between border-b border-line pb-2">
              <p class="text-xs font-semibold uppercase tracking-[0.16em] text-muted">{group.label}</p>
              <p class="text-sm font-semibold text-ink">{formatCents(group.subtotal)}</p>
            </div>
            <div class="mt-3 grid gap-3">
        {#each group.accounts as account}
          <div class="grid gap-3 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.6fr_0.8fr_1fr_0.6fr] md:items-center">
            <div>
              {#if editingId === account.id}
                <div class="grid gap-2 sm:grid-cols-[0.5fr_1.5fr]">
                  <label class="grid gap-2 text-xs font-semibold text-muted">
                    Code
                    <input
                      class="rounded-xl border border-line px-3 py-2 text-sm"
                      type="text"
                      inputmode="numeric"
                      placeholder="1000"
                      bind:value={editCode}
                    />
                  </label>
                  <label class="grid gap-2 text-xs font-semibold text-muted">
                    Name
                    <input
                      class="rounded-xl border border-line px-3 py-2 text-sm"
                      type="text"
                      bind:value={editName}
                    />
                  </label>
                </div>
              {:else}
                <a class="flex items-baseline gap-2 text-sm font-semibold hover:underline" href="/accounts/{account.id}">
                  {#if account.code}<span class="font-mono text-xs text-muted">{account.code}</span>{/if}
                  <span>{account.name}</span>
                </a>
                <p class="mt-1 text-xs font-semibold text-ink">{formatCents(balances[account.id])}</p>
              {/if}
            </div>
            <div class="text-sm text-muted">
              {#if editingId === account.id}
                <label class="grid gap-2 text-xs font-semibold text-muted">
                  Type
                  <select
                    class="rounded-xl border border-line px-3 py-2 text-sm"
                    bind:value={editType}
                  >
                    {#each accountTypeOptions as option}
                      <option value={option.value}>{option.label}</option>
                    {/each}
                  </select>
                </label>
              {:else}
                {account.type}
              {/if}
            </div>
            <div class="text-sm text-muted">
              {#if editingId === account.id}
                <div>
                  <p class="text-xs font-semibold uppercase tracking-[0.14em] text-muted">Role tags</p>
                  <div class="mt-2 flex flex-wrap gap-2">
                    {#each roleTagOptions as option}
                      <label class="flex items-center gap-2 rounded-full border border-line px-3 py-2 text-xs text-ink">
                        <input
                          type="checkbox"
                          checked={editRoleTags.includes(option.value)}
                          onclick={() => toggleEditRoleTag(option.value)}
                        />
                        <span>{option.label}</span>
                      </label>
                    {/each}
                  </div>
                </div>
              {:else if (account.role_tags ?? []).length > 0}
                <div class="flex flex-wrap gap-2">
                  {#each account.role_tags ?? [] as roleTag}
                    <span class="rounded-full border border-line px-2 py-1 text-xs text-ink">
                      {formatRoleTag(roleTag)}
                    </span>
                  {/each}
                </div>
              {:else}
                <span class="text-xs text-muted">No role tags</span>
              {/if}
            </div>
            <div class="flex gap-2">
              {#if editingId === account.id}
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
                  disabled={deletingId === account.id}
                  onclick={() => startEdit(account)}
                >
                  Edit
                </button>
                <button
                  class="rounded-full border status-error px-4 py-2 text-sm font-semibold text-error disabled:opacity-60"
                  type="button"
                  disabled={deletingId === account.id}
                  onclick={() => deleteAccount(account)}
                >
                  {deletingId === account.id ? 'Deleting…' : 'Delete'}
                </button>
              {/if}
            </div>
          </div>
          {#if editingId === account.id && editError}
            <div class="status-message-xs status-error">
              <p>{editError}</p>
              {#if errorRecoveryHint(editError)}
                <p class="mt-1">{errorRecoveryHint(editError)}</p>
              {/if}
            </div>
          {/if}
        {/each}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

</section>
