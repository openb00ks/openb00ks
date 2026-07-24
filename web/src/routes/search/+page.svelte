<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { apiFetch } from '$lib/api/client';
  import { activeEntity } from '$lib/stores/entity';

  type Account = {
    id: string;
    name: string;
    type: string;
    role_tags?: string[];
  };

  type SearchRow = {
    id: string;
    kind: string;
    object_id: string;
    account_id?: string;
    account_name?: string;
    title: string;
    subtitle?: string;
    body?: string;
    status?: string;
    tags?: string[];
    date?: string;
    amount_cents?: number;
    href?: string;
    score: number;
  };

  const kindOptions = [
    { value: '', label: 'All' },
    { value: 'transaction', label: 'Transactions' },
    { value: 'receipt', label: 'Receipts' },
    { value: 'import', label: 'Imports' },
    { value: 'account', label: 'Accounts' },
    { value: 'statement', label: 'Statements' },
    { value: 'mileage', label: 'Mileage' },
    { value: 'vendor', label: 'Vendors' },
  ];

  let query = $state('');
  let kind = $state('');
  let status = $state('');
  let accountID = $state('');
  let tags = $state('');
  let startDate = $state('');
  let endDate = $state('');
  let searching = $state(false);
  let error = $state('');
  let searchedFor = $state('');
  let rows: SearchRow[] = $state([]);
  let accounts = $state<Account[]>([]);
  let loadedAccountsFor = $state('');

  $effect(() => {
    const entityID = $activeEntity;
    if (!entityID) {
      accounts = [];
      loadedAccountsFor = '';
      return;
    }
    if (entityID !== loadedAccountsFor) {
      loadedAccountsFor = entityID;
      void loadAccounts(entityID);
    }
  });

  function kindLabel(value: string) {
    switch (value) {
      case 'transaction':
        return 'Transaction';
      case 'receipt':
        return 'Receipt';
      case 'import':
        return 'Import';
      case 'account':
        return 'Account';
      case 'statement':
        return 'Statement';
      case 'mileage':
        return 'Mileage';
      case 'vendor':
        return 'Vendor';
      default:
        return value;
    }
  }

  async function loadAccounts(entityID: string) {
    try {
      accounts = await apiFetch<Account[]>(`/entities/${encodeURIComponent(entityID)}/accounts?limit=1000`);
    } catch {
      accounts = [];
    }
  }

  function hasActiveFilters() {
    return Boolean(kind || status.trim() || accountID || tags.trim() || startDate || endDate);
  }

  async function runSearch() {
    const trimmed = query.trim();
    if (!$activeEntity) {
      error = 'Select an entity before searching.';
      return;
    }
    if (!trimmed && !hasActiveFilters()) {
      rows = [];
      searchedFor = '';
      return;
    }
    searching = true;
    error = '';
    try {
      const params = new URLSearchParams({
        entity_id: $activeEntity,
        q: trimmed,
        limit: '25',
      });
      if (kind) {
        params.set('kinds', kind);
      }
      if (status.trim()) {
        params.set('statuses', status.trim());
      }
      if (accountID) {
        params.set('account_ids', accountID);
      }
      if (tags.trim()) {
        params.set('tags', tags.trim());
      }
      if (startDate) {
        params.set('start_date', startDate);
      }
      if (endDate) {
        params.set('end_date', endDate);
      }
      const response = await apiFetch<{ rows: SearchRow[] }>(`/search?${params.toString()}`);
      rows = response.rows ?? [];
      searchedFor = trimmed || 'filtered results';
    } catch (err) {
      error = errorMessage(err, 'Unable to search.');
    } finally {
      searching = false;
    }
  }
</script>

<section class="grid gap-6">
  <div>
    <h1 class="text-2xl font-semibold tracking-tight">Search</h1>
    <p class="mt-2 text-sm text-muted">Search indexed transactions, receipts, imports, accounts, statements, mileage, and vendors for the selected entity.</p>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="grid gap-3 lg:grid-cols-[1fr_220px_180px] lg:items-end">
      <label class="grid gap-2 text-sm font-medium text-ink">
        Query
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          type="search"
          placeholder="Merchant, file name, memo, account, status..."
          bind:value={query}
          onkeydown={(event) => {
            if (event.key === 'Enter') {
              runSearch();
            }
          }}
        />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Type
        <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={kind}>
          {#each kindOptions as option}
            <option value={option.value}>{option.label}</option>
          {/each}
        </select>
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Status
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          placeholder="posted, uploaded..."
          bind:value={status}
        />
      </label>
    </div>

    <div class="mt-3 grid gap-3 lg:grid-cols-[240px_1fr_160px_160px_auto] lg:items-end">
      <label class="grid gap-2 text-sm font-medium text-ink">
        Account
        <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={accountID}>
          <option value="">All accounts</option>
          {#each accounts as account}
            <option value={account.id}>{account.name}</option>
          {/each}
        </select>
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Tags
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          placeholder="utilities, tax..."
          bind:value={tags}
        />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        From
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={startDate} />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        To
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={endDate} />
      </label>
      <button
        class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
        type="button"
        disabled={searching}
        onclick={runSearch}
      >
        {searching ? 'Searching...' : 'Search'}
      </button>
    </div>
    {#if error}
      <p class="mt-4 status-message-sm status-error">{error}</p>
    {/if}
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Results</h2>
      {#if searchedFor}
        <span class="text-sm text-muted">{rows.length} result(s)</span>
      {/if}
    </div>
    {#if searching}
      <p class="mt-4 text-sm text-muted">Searching...</p>
    {:else if !searchedFor}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        Search for a merchant, memo, account, file name, or status.
      </div>
    {:else if rows.length === 0}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        No indexed results match this search.
      </div>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each rows as row}
          <a class="grid gap-2 rounded-xl border border-line px-4 py-3 text-ink hover:border-line-strong" href={row.href || '#'}>
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p class="font-semibold">{row.title}</p>
                <p class="mt-1 text-sm text-muted">
                  {kindLabel(row.kind)}{row.subtitle ? ` • ${row.subtitle}` : ''}{row.account_name ? ` • ${row.account_name}` : ''}
                </p>
              </div>
              <div class="text-right text-sm text-muted">
                {#if row.date}
                  <p>{row.date}</p>
                {/if}
                {#if row.amount_cents}
                  <p class="font-semibold text-ink">{formatCents(row.amount_cents)}</p>
                {/if}
              </div>
            </div>
            {#if row.body}
              <p class="line-clamp-2 text-sm text-muted">{row.body}</p>
            {/if}
            {#if row.tags?.length}
              <div class="flex flex-wrap gap-2">
                {#each row.tags as tag}
                  <span class="rounded-full border border-line bg-paper px-2 py-1 text-xs text-muted">{tag}</span>
                {/each}
              </div>
            {/if}
          </a>
        {/each}
      </div>
    {/if}
  </div>
</section>
