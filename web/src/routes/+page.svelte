<script lang="ts">
  import { activeEntity } from '$lib/stores/entity';
  import { entities, entitiesLoaded, selectEntity } from '$lib/stores/entity';

  let query = $state('');
  let filteredEntities = $derived($entities.filter((entity) =>
    entity.name.toLowerCase().includes(query.toLowerCase())
  ));
  let hasEntities = $derived($entities.length > 0);
  let activeEntityRecord =
    $derived($entities.find((entity) => entity.id === $activeEntity) ?? null);
</script>

<section class="grid gap-8">
  <div class="rounded-3xl bg-surface p-8 shadow-sm">
    <p class="text-sm uppercase tracking-[0.2em] text-muted">
      Global dashboard
    </p>
    <h1 class="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
      {#if activeEntityRecord}
        {activeEntityRecord.name} is ready for work.
      {:else if hasEntities}
        Choose an entity before entering its books.
      {:else if !$entitiesLoaded}
        Loading your workspace…
      {:else}
        Create your first entity to start bookkeeping.
      {/if}
    </h1>
    <p class="mt-4 max-w-2xl text-muted">
      {#if activeEntityRecord}
        Start from the entity dashboard, then capture receipts, review suggestions,
        and post confirmed transactions manually.
      {:else if hasEntities}
        Select an entity to access receipt capture, the review queue, and reports.
        You can switch entities any time from the header.
      {:else if !$entitiesLoaded}
        Fetching the entities you have access to…
      {:else}
        Your first entity creates the working context for receipts, review, and
        books. Once it exists, day-to-day work becomes entity-specific.
      {/if}
    </p>
    <div class="mt-6 flex flex-wrap gap-3">
      {#if activeEntityRecord}
        <a
          class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper"
          href="/entity"
        >
          Open entity dashboard
        </a>
        <a
          class="rounded-full border border-line bg-surface px-5 py-2 text-sm font-semibold"
          href="/review"
        >
          Open review queue
        </a>
      {:else if hasEntities}
        <a
          class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper"
          href="#entity-list"
        >
          Select an entity
        </a>
        <a
          class="rounded-full border border-line bg-surface px-5 py-2 text-sm font-semibold"
          href="/entities"
        >
          Manage entities
        </a>
      {:else if !$entitiesLoaded}
        <span class="text-sm text-muted">One moment…</span>
      {:else}
        <a
          class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper"
          href="/entities"
        >
          Create your first entity
        </a>
      {/if}
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6" id="entity-list">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Your entities</h2>
      <span class="text-xs uppercase tracking-[0.2em] text-muted">
        {hasEntities ? `${filteredEntities.length} visible` : 'Get started'}
      </span>
    </div>
    {#if hasEntities}
      <div class="mt-4">
        <input
          class="w-full rounded-xl border border-line px-3 py-2 text-sm"
          type="search"
          placeholder="Search entities..."
          bind:value={query}
        />
      </div>
      {#if filteredEntities.length === 0}
        <p class="mt-4 text-sm text-muted">
          No entities match this search.
        </p>
      {:else}
        <div class="mt-4 grid gap-3 md:grid-cols-3">
          {#each filteredEntities as entity}
            <a
              class="rounded-xl border border-line px-4 py-4 text-left hover:border-line-strong"
              href="/entity"
              onclick={() => selectEntity(entity.id)}
            >
              <p class="text-sm font-semibold">{entity.name}</p>
              <p class="mt-2 text-xs text-muted">
                {entity.id === $activeEntity ? 'Current working entity' : 'Select and open books →'}
              </p>
            </a>
          {/each}
        </div>
      {/if}
    {:else if !$entitiesLoaded}
      <p class="mt-4 text-sm text-muted">Loading entities…</p>
    {:else}
      <div class="mt-4 rounded-xl border border-line px-4 py-4 text-sm text-muted">
        <p class="font-semibold text-ink">No entities yet.</p>
        <p class="mt-2">
          Create an entity first so receipts, review, transactions, and reports
          have a bookkeeping context.
        </p>
        <a class="mt-3 inline-flex rounded-full border border-line px-4 py-2 font-semibold text-ink" href="/entities">
          Go to entities
        </a>
      </div>
    {/if}
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Work guidance</h2>
    <div class="mt-4 grid gap-3 text-sm text-muted">
      {#if activeEntityRecord}
        <div class="rounded-xl border border-line px-4 py-3">
          Start in the review queue for anything waiting on confirmation.
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Use receipts and imports to capture new source documents.
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Posting remains manual. Review the draft before finalizing any transaction.
        </div>
      {:else}
        <div class="rounded-xl border border-line px-4 py-3">
          Pick one entity to enter a focused bookkeeping workspace.
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Once selected, the same entity stays active until you change it.
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          The safest next step is to choose an entity before uploading or reviewing anything.
        </div>
      {/if}
    </div>
  </div>
</section>
