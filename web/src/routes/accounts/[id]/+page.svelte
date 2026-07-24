<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { page } from '$app/stores';
  import { apiFetch } from '$lib/api/client';

  type LedgerRow = {
    transaction_id: string;
    date: string;
    memo: string;
    debit_cents: number;
    credit_cents: number;
  };

  type AccountTransactions = {
    account: { id: string; entity_id: string; name: string; type: string; code?: string };
    balance_cents: number;
    rows: LedgerRow[];
    has_more: boolean;
  };

  let accountId = $derived($page.params.id);
  let data = $state<AccountTransactions | null>(null);
  let loading = $state(false);
  let loadingMore = $state(false);
  let error = $state('');
  let loadedFor = '';

  // Running balance per row. Rows come newest-first and `balance_cents` is the current (all-time) balance,
  // so the newest row's "balance after" is the current balance; walking older we subtract each row's effect
  // on this account's natural side (assets/expenses rise on debit; everything else rises on credit).
  let ledgerRows = $derived.by(() => {
    if (!data) return [];
    const debitNatural = data.account.type === 'asset' || data.account.type === 'expense';
    let running = data.balance_cents;
    return data.rows.map((row) => {
      const balanceAfter = running;
      const delta = debitNatural
        ? row.debit_cents - row.credit_cents
        : row.credit_cents - row.debit_cents;
      running -= delta;
      return { ...row, balanceAfter };
    });
  });

  async function load(id: string) {
    loading = true;
    error = '';
    try {
      data = await apiFetch<AccountTransactions>(`/accounts/${encodeURIComponent(id)}/transactions`);
    } catch (err) {
      error = errorMessage(err, 'Unable to load account.');
      data = null;
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (!data || !accountId) return;
    loadingMore = true;
    try {
      const next = await apiFetch<AccountTransactions>(
        `/accounts/${encodeURIComponent(accountId)}/transactions?offset=${data.rows.length}`
      );
      // Append the older page; the running-balance derived recomputes over the full contiguous set.
      data = { ...data, rows: [...data.rows, ...next.rows], has_more: next.has_more };
    } catch (err) {
      error = errorMessage(err, 'Unable to load more.');
    } finally {
      loadingMore = false;
    }
  }

  $effect(() => {
    if (accountId && accountId !== loadedFor) {
      loadedFor = accountId;
      void load(accountId);
    }
  });
</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/accounts">← Back to accounts</a>

  {#if error}
    <p class="status-message-sm status-error">{error}</p>
  {:else if loading || !data}
    <p class="text-sm text-muted">Loading account…</p>
  {:else}
    <div class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Account</p>
        <h1 class="mt-2 flex items-baseline gap-2 text-2xl font-semibold tracking-tight">
          {#if data.account.code}<span class="font-mono text-base text-muted">{data.account.code}</span>{/if}
          <span>{data.account.name}</span>
        </h1>
        <p class="mt-1 text-sm capitalize text-muted">{data.account.type}</p>
      </div>
      <div class="rounded-2xl border border-line bg-surface px-5 py-4 text-right">
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Current balance</p>
        <p class="mt-1 text-2xl font-semibold text-ink">{formatCents(data.balance_cents)}</p>
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Transactions</h2>
        <span class="text-sm text-muted">{data.rows.length} entr{data.rows.length === 1 ? 'y' : 'ies'}</span>
      </div>
      {#if data.rows.length === 0}
        <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
          No posted transactions touch this account yet.
        </div>
      {:else}
        <div class="mt-4 overflow-x-auto">
          <table class="w-full min-w-[42rem] text-sm">
            <thead>
              <tr class="border-b border-line text-left text-xs uppercase tracking-[0.14em] text-muted">
                <th class="py-2 pr-4 font-semibold">Date</th>
                <th class="py-2 pr-4 font-semibold">Memo</th>
                <th class="py-2 pl-4 text-right font-semibold">Debit</th>
                <th class="py-2 pl-4 text-right font-semibold">Credit</th>
                <th class="py-2 pl-4 text-right font-semibold">Balance</th>
              </tr>
            </thead>
            <tbody>
              {#each ledgerRows as row}
                <tr class="border-b border-line/60">
                  <td class="py-2 pr-4 align-top whitespace-nowrap text-muted">{row.date}</td>
                  <td class="py-2 pr-4 align-top">
                    <a class="hover:underline" href="/transactions?focus={row.transaction_id}">
                      {row.memo || '—'}
                    </a>
                  </td>
                  <td class="py-2 pl-4 text-right align-top font-medium">
                    {row.debit_cents ? formatCents(row.debit_cents) : ''}
                  </td>
                  <td class="py-2 pl-4 text-right align-top font-medium">
                    {row.credit_cents ? formatCents(row.credit_cents) : ''}
                  </td>
                  <td class="py-2 pl-4 text-right align-top font-semibold text-ink whitespace-nowrap">
                    {formatCents(row.balanceAfter)}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if data.has_more}
          <div class="mt-4 flex justify-center">
            <button
              class="rounded-full border border-line px-5 py-2 text-sm font-semibold disabled:opacity-60"
              type="button"
              disabled={loadingMore}
              onclick={loadMore}
            >
              {loadingMore ? 'Loading…' : 'Load more'}
            </button>
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</section>
