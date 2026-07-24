<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { apiFetch } from '$lib/api/client';
  import type { Account } from '$lib/api/types';
  import { activeEntity } from '$lib/stores/entity';
  import { formatLocalDate, todayLocalDate, formatShortDate as formatDate } from '$lib/utils/date';
  import NewJournalEntry from '$lib/components/NewJournalEntry.svelte';

  type TransactionRow = {
    transaction: {
      id: string;
      entity_id: string;
      date: string;
      memo?: string;
    };
    entries: Array<{
      id: string;
      account_id: string;
      debit_cents: number;
      credit_cents: number;
    }>;
  };

  type AccountRow = Pick<Account, 'id' | 'name' | 'code'> & { type?: string };

  type TransactionSearchRow = {
    transaction_id: string;
    entity_id: string;
    date: string;
    memo?: string;
    description?: string;
    account_ids?: string[];
    account_names?: string[];
    account_role_tags?: string[];
    amount_cents: number;
    score: number;
  };

  let transactions: TransactionRow[] = $state([]);
  let searchResults: TransactionSearchRow[] = $state([]);
  let accounts: Record<string, AccountRow> = $state({});
  let accountList = $state<AccountRow[]>([]);
  let loading = $state(false);
  let searching = $state(false);
  let error = $state('');
  let searchQuery = $state('');
  let activeSearch = $state('');
  let startDate = $state('');
  let endDate = $state('');
  let limit = $state(100);

  function initDates() {
    const now = new Date();
    const todayStr = todayLocalDate();
    const monthStart = formatLocalDate(new Date(now.getFullYear(), now.getMonth(), 1));
    if (!startDate) {
      startDate = monthStart;
    }
    if (!endDate) {
      endDate = todayStr;
    }
  }

  function totalAmount(entries: TransactionRow['entries']) {
    const debit = entries.reduce((sum, entry) => sum + entry.debit_cents, 0);
    return formatCents(debit);
  }

  async function loadTransactions() {
    if (!$activeEntity) {
      transactions = [];
      return;
    }
    if (!startDate || !endDate) {
      error = 'Select a start and end date.';
      return;
    }
    loading = true;
    error = '';
    try {
      const params = new URLSearchParams({
        entity_id: $activeEntity,
        start_date: startDate,
        end_date: endDate,
        limit: String(limit)
      });
      const response = await apiFetch<{ rows: TransactionRow[] }>(
        `/transactions?${params.toString()}`
      );
      transactions = response.rows ?? [];
      activeSearch = '';
      searchResults = [];
      // The accounts endpoint returns a bare array, not { rows }.
      const accountResp = await apiFetch<AccountRow[]>(
        `/entities/${encodeURIComponent($activeEntity)}/accounts`
      );
      accountList = accountResp ?? [];
      accounts = {};
      for (const row of accountList) {
        accounts[row.id] = row;
      }
    } catch (err) {
      error = errorMessage(err, 'Unable to load transactions.');
    } finally {
      loading = false;
    }
  }

  async function searchTransactions() {
    if (!$activeEntity) {
      searchResults = [];
      return;
    }
    const query = searchQuery.trim();
    if (!query) {
      activeSearch = '';
      searchResults = [];
      return;
    }
    searching = true;
    error = '';
    try {
      const params = new URLSearchParams({
        entity_id: $activeEntity,
        q: query,
        limit: '20'
      });
      const response = await apiFetch<{ rows: TransactionSearchRow[] }>(
        `/search/transactions?${params.toString()}`
      );
      activeSearch = query;
      searchResults = response.rows ?? [];
    } catch (err) {
      error = errorMessage(err, 'Unable to search transactions.');
    } finally {
      searching = false;
    }
  }

  function clearSearch() {
    searchQuery = '';
    activeSearch = '';
    searchResults = [];
  }

  $effect(() => {
    if ($activeEntity) {
      initDates();
      loadTransactions();
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Transactions</h1>
      <p class="mt-2 text-sm text-muted">Recent journal entries and status.</p>
    </div>
  </div>

  {#if $activeEntity}
    <NewJournalEntry entityId={$activeEntity} accounts={accountList} oncreated={loadTransactions} />
  {/if}

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Latest activity</h2>
      <div class="flex gap-2 text-xs text-muted">
        <span class="rounded-full border border-line px-3 py-1">Posted</span>
      </div>
    </div>
    <div class="mt-4 grid gap-3 text-sm text-muted md:grid-cols-4">
      <label class="grid gap-2 text-sm font-medium text-ink md:col-span-4">
        Search
        <input
          class="rounded-xl border border-line px-3 py-2 text-sm"
          type="search"
          placeholder="Vendor, memo, account, or tag"
          bind:value={searchQuery}
          onkeydown={(event) => {
            if (event.key === 'Enter') {
              searchTransactions();
            }
          }}
        />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Start date
        <input class="rounded-xl border border-line px-3 py-2 text-sm" type="date" bind:value={startDate} />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        End date
        <input class="rounded-xl border border-line px-3 py-2 text-sm" type="date" bind:value={endDate} />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Limit
        <input class="rounded-xl border border-line px-3 py-2 text-sm" type="number" min="1" max="1000" bind:value={limit} />
      </label>
    </div>
    <div class="mt-4 flex gap-3">
      <button class="rounded-full border border-line px-4 py-2 text-sm font-semibold" type="button" onclick={searchTransactions}>
        {searching ? 'Searching...' : 'Search'}
      </button>
      {#if activeSearch}
        <button class="rounded-full border border-line px-4 py-2 text-sm font-semibold" type="button" onclick={clearSearch}>
          Clear search
        </button>
      {/if}
      <button class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper" type="button" onclick={loadTransactions}>
        Refresh
      </button>
    </div>
    {#if error}
      <p class="mt-4 status-message-sm status-error">
        {error}
      </p>
    {:else if loading || searching}
      <p class="mt-4 text-sm text-muted">Loading transactions…</p>
    {:else if activeSearch}
      <div class="mt-4 grid gap-3">
        {#if searchResults.length === 0}
          <div class="rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
            <p class="font-semibold text-ink">No indexed matches.</p>
            <p class="mt-2">Try another vendor, memo, account name, or role tag.</p>
          </div>
        {:else}
          <p class="text-sm text-muted">
            Search results for <span class="font-semibold text-ink">{activeSearch}</span>
          </p>
          {#each searchResults as row}
            <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.4fr_1fr_1fr_0.8fr] md:items-center">
              <div>
                <p class="text-sm font-semibold">
                  {row.memo ?? row.description ?? 'Posted entry'}
                </p>
                <p class="text-xs text-muted">
                  {row.account_names?.join(', ') || row.account_ids?.join(', ') || '-'}
                </p>
              </div>
              <div class="text-sm text-muted">{formatDate(row.date)}</div>
              <div class="text-sm">
                {row.account_role_tags?.join(', ') || 'Posted'}
              </div>
              <div class="text-right text-sm font-semibold">
                {formatCents(row.amount_cents)}
              </div>
            </div>
          {/each}
        {/if}
      </div>
    {:else if transactions.length === 0}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        {#if !$activeEntity}
          <p class="font-semibold text-ink">Select an entity to view transactions.</p>
          <p class="mt-2">Transactions are always scoped to the active entity before any books activity can be reviewed.</p>
        {:else}
          <p class="font-semibold text-ink">No transactions in this date range.</p>
          <p class="mt-2">Try expanding the dates or post a reviewed draft to create the first journal entry.</p>
        {/if}
      </div>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each transactions as tx}
          <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.4fr_1fr_1fr_0.8fr] md:items-center">
            <div>
              <p class="text-sm font-semibold">
                {tx.transaction.memo ?? 'Posted entry'}
              </p>
              <p class="text-xs text-muted">
                {tx.entries[0]?.account_id
                  ? accounts[tx.entries[0].account_id]?.name ?? tx.entries[0].account_id
                  : '—'}
              </p>
            </div>
            <div class="text-sm text-muted">{formatDate(tx.transaction.date)}</div>
            <div class="text-sm">Posted</div>
            <div class="text-right text-sm font-semibold">
              {totalAmount(tx.entries)}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="grid gap-4 md:grid-cols-2">
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Debits in range</p>
      <p class="mt-2 text-2xl font-semibold">
        {formatCents(
          transactions.reduce(
            (sum, tx) => sum + tx.entries.reduce((acc, entry) => acc + entry.debit_cents, 0),
            0
          )
        )}
      </p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Posted entries</p>
      <p class="mt-2 text-2xl font-semibold">{transactions.length}</p>
    </div>
  </div>
</section>

