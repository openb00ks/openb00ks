<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { apiFetch, apiFetchPublic } from '$lib/api/client';

  let tenantName = $state('');
  let adminEmail = $state('');
  let adminPassword = $state('');
  let defaultEntityName = $state('');
  let selectedTemplate = $state('basic');
  let templates: Array<{ key: string; name: string; account_count: number }> = $state([]);
  let loading = $state(false);
  let error = $state('');
  let statusChecked = $state(false);

  async function checkStatus() {
    try {
      const status = await apiFetchPublic<{ required: boolean }>('/setup/status');
      statusChecked = true;
      if (!status.required) {
        await goto('/login');
      }
    } catch (err) {
      statusChecked = true;
      error = errorMessage(err, 'Unable to check setup status.');
    }
  }

  async function submit() {
    error = '';
    loading = true;
    try {
      await apiFetch('/setup', {
        method: 'POST',
        body: {
          tenant_name: tenantName.trim(),
          admin_email: adminEmail.trim(),
          admin_password: adminPassword,
          default_entity_name: defaultEntityName.trim(),
          template: selectedTemplate
        }
      });
      await goto('/login');
    } catch (err) {
      error = errorMessage(err, 'Setup failed.');
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    checkStatus();
    loadTemplates();
  });

  async function loadTemplates() {
    try {
      const resp = await apiFetchPublic<{
        rows: Array<{ key: string; name: string; account_count: number }>;
      }>('/entity-templates');
      templates = resp.rows ?? [];
    } catch {
      templates = [];
    }
  }
</script>

<section class="mx-auto grid max-w-4xl gap-6 lg:grid-cols-[1.05fr_0.95fr]">
  <div class="rounded-3xl bg-surface p-8 shadow-sm">
    <p class="text-xs uppercase tracking-[0.2em] text-muted">Initial setup</p>
    <h1 class="mt-2 text-2xl font-semibold tracking-tight">Create the first workspace</h1>
    <p class="mt-2 text-sm text-muted">
      This is a one-time setup step that creates the initial tenant and the first admin login.
    </p>

    <div class="mt-4 rounded-2xl border border-line bg-paper px-4 py-4 text-sm text-muted">
      <p class="font-semibold text-ink">What a tenant is</p>
      <p class="mt-2">
        A tenant is one isolated bookkeeping workspace. It keeps one business's users, entities,
        transactions, and reports separate from any other business or personal books.
      </p>
    </div>

    <div class="mt-6 grid gap-3 text-sm text-muted">
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">What this creates</p>
        <p class="mt-2">A tenant, the first admin user, and an optional default entity for day-one bookkeeping.</p>
      </div>
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">What happens next</p>
        <p class="mt-2">After setup, you will sign in, choose an entity, then capture receipts for manual review and posting.</p>
      </div>
    </div>

    {#if error}
      <p class="mt-4 status-message-sm status-error">
        {error}
      </p>
    {/if}

    {#if statusChecked}
      <form class="mt-6 grid gap-4" onsubmit={(event) => { event.preventDefault(); void submit(); }}>
        <label class="grid gap-2 text-sm font-medium">
          Tenant name
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            placeholder="Default Tenant"
            bind:value={tenantName}
          />
        </label>

        <label class="grid gap-2 text-sm font-medium">
          Admin email
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="email"
            autocomplete="email"
            bind:value={adminEmail}
            required
          />
        </label>

        <label class="grid gap-2 text-sm font-medium">
          Admin password
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="password"
            autocomplete="new-password"
            bind:value={adminPassword}
            required
          />
        </label>

        <label class="grid gap-2 text-sm font-medium">
          Default entity name (optional)
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            placeholder="Acme LLC"
            bind:value={defaultEntityName}
          />
        </label>

        {#if templates.length > 0}
          <label class="grid gap-2 text-sm font-medium">
            Starting chart of accounts
            <select
              class="rounded-xl border border-line px-3 py-2 text-base"
              bind:value={selectedTemplate}
            >
              {#each templates as tmpl}
                <option value={tmpl.key}>{tmpl.name} ({tmpl.account_count} accounts)</option>
              {/each}
            </select>
            <span class="text-xs font-normal text-muted">
              Applied only if you name a default entity above. You can edit accounts later.
            </span>
          </label>
        {/if}

        <button
          class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
          type="submit"
          disabled={loading}
        >
          {loading ? 'Setting up…' : 'Create workspace'}
        </button>
      </form>
    {:else}
      <p class="mt-6 text-sm text-muted">Checking setup status…</p>
    {/if}
  </div>

  <div class="rounded-3xl border border-line bg-surface p-8">
    <h2 class="text-lg font-semibold">How work moves after setup</h2>
    <div class="mt-4 grid gap-3 text-sm text-muted">
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">1. Create or select an entity</p>
        <p class="mt-2">All operational work is scoped to a single entity before uploads and review are available.</p>
      </div>
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">2. Capture receipts or imports</p>
        <p class="mt-2">Bring in source documents first, then let the app prepare drafts and suggestions.</p>
      </div>
      <div class="rounded-2xl border border-line px-4 py-4">
        <p class="font-semibold text-ink">3. Review and post manually</p>
        <p class="mt-2">Nothing posts automatically. You confirm the draft, then choose when to post the transaction.</p>
      </div>
    </div>
  </div>
</section>

