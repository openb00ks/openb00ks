<script lang="ts">
  import { formatCents } from '$lib/utils/money';
  import { browser } from "$app/environment";
  import { goto } from "$app/navigation";
  import { onMount, onDestroy, tick } from "svelte";
  import { get } from "svelte/store";
  import { apiFetch } from "$lib/api/client";
  import { activeEntity } from "$lib/stores/entity";
  import { searchOpen, closeSearch, toggleSearch } from "$lib/stores/search";

  // Global search hit — mirrors the /search endpoint's row shape (see routes/search/+page.svelte).
  type SearchRow = {
    id: string;
    kind: string;
    object_id: string;
    account_name?: string;
    title: string;
    subtitle?: string;
    status?: string;
    date?: string;
    amount_cents?: number;
    href?: string;
    score: number;
  };

  let query = $state("");
  let hits: SearchRow[] = $state([]);
  let activeIndex = $state(-1);
  let loading = $state(false);
  let inputEl: HTMLInputElement | null = $state(null);
  let debounceTimer: ReturnType<typeof setTimeout>;

  let entityID = $derived($activeEntity);

  onMount(() => {
    function onKeydown(event: KeyboardEvent) {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        toggleSearch();
      } else if (event.key === "Escape" && get(searchOpen)) {
        closeSearch();
      }
    }
    window.addEventListener("keydown", onKeydown);
    return () => window.removeEventListener("keydown", onKeydown);
  });

  onDestroy(() => clearTimeout(debounceTimer));

  // Focus on open; reset on close.
  $effect(() => {
    if ($searchOpen && browser) {
      void tick().then(() => inputEl?.focus());
    } else if (!$searchOpen) {
      query = "";
      hits = [];
      activeIndex = -1;
      loading = false;
    }
  });

  function kindLabel(kind: string) {
    switch (kind) {
      case "transaction":
        return "Transaction";
      case "receipt":
        return "Receipt";
      case "import":
        return "Import";
      case "account":
        return "Account";
      case "statement":
        return "Statement";
      case "mileage":
        return "Mileage";
      case "vendor":
        return "Vendor";
      default:
        return kind;
    }
  }

  function onInput() {
    activeIndex = -1;
    const q = query.trim();
    clearTimeout(debounceTimer);
    if (q.length < 2 || !entityID) {
      hits = [];
      loading = false;
      return;
    }
    debounceTimer = setTimeout(async () => {
      loading = true;
      try {
        const params = new URLSearchParams({ entity_id: entityID, q, limit: "8" });
        const response = await apiFetch<{ rows: SearchRow[] }>(`/search?${params.toString()}`);
        // Ignore a stale response if the query changed while the request was in flight.
        if (query.trim() === q) {
          hits = response.rows ?? [];
        }
      } catch {
        hits = [];
      } finally {
        loading = false;
      }
    }, 200);
  }

  function onFieldKeydown(event: KeyboardEvent) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      activeIndex = Math.min(activeIndex + 1, hits.length - 1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      activeIndex = Math.max(activeIndex - 1, -1);
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (activeIndex >= 0 && hits[activeIndex]) {
        selectHit(hits[activeIndex]);
      } else {
        submit();
      }
    }
  }

  function submit() {
    const q = query.trim();
    closeSearch();
    goto(q ? `/search?q=${encodeURIComponent(q)}` : "/search");
  }

  function selectHit(hit: SearchRow) {
    closeSearch();
    goto(hit.href || "/search");
  }
</script>

{#if $searchOpen}
  <button
    type="button"
    class="fixed inset-0 z-40 cursor-default bg-black/50 backdrop-blur-sm"
    aria-label="Close search"
    onclick={closeSearch}
  ></button>

  <div
    class="fixed inset-x-0 top-[12vh] z-50 mx-auto w-full max-w-2xl px-4"
    role="dialog"
    aria-modal="true"
    aria-label="Search"
  >
    <div class="overflow-hidden rounded-2xl border border-line bg-surface shadow-xl">
      <div class="flex items-center gap-3 border-b border-line px-4 py-3">
        <svg class="h-5 w-5 shrink-0 text-muted" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" d="m21 21-4.35-4.35M17 11A6 6 0 1 1 5 11a6 6 0 0 1 12 0Z" />
        </svg>
        <input
          bind:this={inputEl}
          bind:value={query}
          oninput={onInput}
          onkeydown={onFieldKeydown}
          type="search"
          placeholder={entityID ? "Search transactions, receipts, vendors, accounts…" : "Select an entity to search"}
          autocomplete="off"
          disabled={!entityID}
          class="min-w-0 flex-1 bg-transparent text-base text-ink placeholder:text-muted focus:outline-none disabled:opacity-60"
        />
        {#if loading}
          <div class="h-4 w-4 shrink-0 animate-spin rounded-full border-2 border-line border-t-ink"></div>
        {:else}
          <kbd class="hidden shrink-0 rounded border border-line px-1.5 py-0.5 text-xs text-muted sm:block">Esc</kbd>
        {/if}
      </div>

      {#if !entityID}
        <p class="px-4 py-6 text-center text-sm text-muted">
          Select an active entity from the sidebar to search its books.
        </p>
      {:else if hits.length > 0}
        <ul role="listbox" class="max-h-[50vh] overflow-y-auto">
          {#each hits as hit, i (hit.id)}
            <li role="option" aria-selected={i === activeIndex}>
              <button
                type="button"
                class={`flex w-full items-center gap-3 px-4 py-3 text-left ${i === activeIndex ? "bg-primary/10" : "hover:bg-paper"}`}
                onmousedown={(event) => {
                  event.preventDefault();
                  selectHit(hit);
                }}
                onmouseenter={() => (activeIndex = i)}
              >
                <span class="shrink-0 rounded-full border border-line bg-paper px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted">
                  {kindLabel(hit.kind)}
                </span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-semibold text-ink">{hit.title}</span>
                  {#if hit.subtitle || hit.account_name}
                    <span class="mt-0.5 block truncate text-xs text-muted">
                      {[hit.subtitle, hit.account_name].filter(Boolean).join(" • ")}
                    </span>
                  {/if}
                </span>
                {#if hit.amount_cents}
                  <span class="shrink-0 text-sm font-semibold text-ink">{formatCents(hit.amount_cents)}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
        <div class="border-t border-line px-4 py-2">
          <button
            type="button"
            class="flex w-full items-center justify-between text-sm font-semibold text-primary hover:opacity-80"
            onmousedown={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            <span>See all results{query.trim() ? ` for "${query.trim()}"` : ""}</span>
            <kbd class="rounded border border-line px-1.5 py-0.5 text-xs text-muted">↵</kbd>
          </button>
        </div>
      {:else if query.trim().length >= 2 && !loading}
        <div class="px-4 py-6 text-center text-sm text-muted">
          <p>No matches for "{query.trim()}".</p>
          <button type="button" class="mt-2 text-sm font-semibold text-primary hover:opacity-80" onmousedown={submit}>
            Open advanced search
          </button>
        </div>
      {:else}
        <p class="px-4 py-6 text-center text-sm text-muted">
          Start typing to search this entity's transactions, receipts, imports, accounts, statements, mileage, and vendors.
        </p>
      {/if}
    </div>
  </div>
{/if}
