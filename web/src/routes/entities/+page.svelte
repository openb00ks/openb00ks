<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api/client';
  import { entities as entitiesStore } from '$lib/stores/entity';

  type EntityRow = {
    id: string;
    name: string;
  };

  type MemberRow = {
    id: string;
    user_id: string;
    role: string;
  };

  type EntityView = {
    id: string;
    name: string;
    members: number;
  };

  type TemplateRow = {
    key: string;
    name: string;
    account_count: number;
  };

  let entities: EntityView[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let createOpen = $state(false);
  let createName = $state('');
  let createError = $state('');
  let creating = $state(false);
  let templates: TemplateRow[] = $state([]);
  let selectedTemplate = $state('basic');
  let defaultEntityId = $state('');
  let defaultError = $state('');
  let defaultSaving = $state('');
  let memberEntityId = $state('');
  let members: MemberRow[] = $state([]);
  let membersLoading = $state(false);
  let membersError = $state('');
  let newMemberEmail = $state('');
  let newMemberRole = $state('user');
  let memberSaving = $state(false);
  let memberUpdating: Record<string, boolean> = $state({});
  let memberRemoving: Record<string, boolean> = $state({});

  async function loadEntities() {
    loading = true;
    error = '';
    try {
      const rows = await apiFetch<EntityRow[]>('/entities');
      const enriched = await Promise.all(
        rows.map(async (row) => {
          try {
            const memberResp = await apiFetch<{ rows: MemberRow[] }>(
              `/entities/${row.id}/members`
            );
            const memberRows = memberResp.rows ?? [];
            return {
              id: row.id,
              name: row.name,
              members: memberRows.length
            };
          } catch {
            return {
              id: row.id,
              name: row.name,
              members: 0
            };
          }
        })
      );
      entities = enriched;
      entitiesStore.set(rows);
    } catch (err) {
      error = errorMessage(err, 'Unable to load entities.');
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadEntities();
    loadDefaultEntity();
    loadTemplates();
  });

  async function loadTemplates() {
    try {
      const resp = await apiFetch<{ rows: TemplateRow[] }>('/entity-templates');
      templates = resp.rows ?? [];
    } catch {
      templates = [];
    }
  }

  async function loadDefaultEntity() {
    try {
      const prefs = await apiFetch<{ default_entity_id?: string }>(
        '/me/preferences'
      );
      defaultEntityId = prefs.default_entity_id ?? '';
    } catch {
      defaultEntityId = '';
    }
  }

  async function setDefaultEntity(id: string) {
    defaultError = '';
    defaultSaving = id;
    try {
      await apiFetch('/me/preferences', {
        method: 'PATCH',
        body: { default_entity_id: id || '' }
      });
      defaultEntityId = id;
    } catch (err) {
      defaultError = errorMessage(err, 'Unable to update default entity.');
    } finally {
      defaultSaving = '';
    }
  }

  async function createEntity() {
    createError = '';
    if (!createName.trim()) {
      createError = 'Entity name is required.';
      return;
    }
    creating = true;
    try {
      await apiFetch('/entities', {
        method: 'POST',
        body: { name: createName.trim(), template: selectedTemplate }
      });
      createName = '';
      createOpen = false;
      await loadEntities();
    } catch (err) {
      createError = errorMessage(err, 'Unable to create entity.');
    } finally {
      creating = false;
    }
  }

  async function loadMembers(entityID: string) {
    if (!entityID) {
      members = [];
      return;
    }
    membersLoading = true;
    membersError = '';
    try {
      const response = await apiFetch<{ rows: MemberRow[] }>(
        `/entities/${entityID}/members`
      );
      members = response.rows ?? [];
    } catch (err) {
      membersError = errorMessage(err, 'Unable to load members.');
    } finally {
      membersLoading = false;
    }
  }

  async function addMember() {
    if (!memberEntityId) {
      membersError = 'Select an entity.';
      return;
    }
    if (!newMemberEmail.trim()) {
      membersError = 'Email is required.';
      return;
    }
    memberSaving = true;
    membersError = '';
    try {
      await apiFetch(`/entities/${memberEntityId}/members`, {
        method: 'POST',
        body: { email: newMemberEmail.trim(), role: newMemberRole }
      });
      newMemberEmail = '';
      newMemberRole = 'user';
      await loadMembers(memberEntityId);
    } catch (err) {
      const message = errorMessage(err, '');
      membersError =
        message === 'USER_NOT_FOUND'
          ? 'No user with that email. They need an account first.'
          : message || 'Unable to add member.';
    } finally {
      memberSaving = false;
    }
  }

  async function updateMember(memberId: string, role: string) {
    if (!memberEntityId) {
      return;
    }
    memberUpdating = { ...memberUpdating, [memberId]: true };
    membersError = '';
    try {
      await apiFetch(`/entities/${memberEntityId}/members/${memberId}`, {
        method: 'PATCH',
        body: { role }
      });
      await loadMembers(memberEntityId);
    } catch (err) {
      membersError = errorMessage(err, 'Unable to update member.');
    } finally {
      memberUpdating = { ...memberUpdating, [memberId]: false };
    }
  }

  async function removeMember(memberId: string) {
    if (!memberEntityId) {
      return;
    }
    memberRemoving = { ...memberRemoving, [memberId]: true };
    membersError = '';
    try {
      await apiFetch(`/entities/${memberEntityId}/members/${memberId}`, {
        method: 'DELETE'
      });
      await loadMembers(memberEntityId);
    } catch (err) {
      membersError = errorMessage(err, 'Unable to remove member.');
    } finally {
      memberRemoving = { ...memberRemoving, [memberId]: false };
    }
  }
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Entities</h1>
      <p class="mt-2 text-sm text-muted">Manage businesses and membership roles.</p>
    </div>
    <button
      class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper"
      type="button"
      onclick={() => (createOpen = !createOpen)}
    >
      {createOpen ? 'Close' : 'New entity'}
    </button>
  </div>

  {#if createOpen}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Create entity</h2>
      <form class="mt-4 grid gap-3" onsubmit={(event) => { event.preventDefault(); void createEntity(); }}>
        <label class="grid gap-2 text-sm font-medium">
          Name
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            bind:value={createName}
            required
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
              Seeds a starter chart of accounts for this entity — you can edit it later.
            </span>
          </label>
        {/if}
        {#if createError}
          <p class="status-message-sm status-error">
            {createError}
          </p>
        {/if}
        <div class="flex gap-3">
          <button
            class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
            type="submit"
            disabled={creating}
          >
            {creating ? 'Creating…' : 'Create'}
          </button>
          <button
            class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
            type="button"
            onclick={() => (createOpen = false)}
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  {/if}

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Your entities</h2>
      <span class="text-xs uppercase tracking-[0.2em] text-muted">Live</span>
    </div>
    {#if error}
      <p class="mt-4 status-message-sm status-error">
        {error}
      </p>
    {:else if loading}
      <p class="mt-4 text-sm text-muted">Loading entities…</p>
    {:else if entities.length === 0}
      <p class="mt-4 text-sm text-muted">No entities yet.</p>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each entities as entity}
          <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-line px-4 py-3">
            <div>
              <p class="text-sm font-semibold">{entity.name}</p>
              <p class="text-xs text-muted">{entity.members} members</p>
            </div>
            <div class="flex items-center gap-2">
              {#if defaultEntityId === entity.id}
                <span class="rounded-full border border-line px-3 py-1 text-xs font-semibold text-muted">
                  Default
                </span>
              {:else}
                <button
                  class="rounded-full border border-ink/20 px-3 py-1 text-xs font-semibold"
                  type="button"
                  disabled={defaultSaving === entity.id}
                  onclick={() => setDefaultEntity(entity.id)}
                >
                  {defaultSaving === entity.id ? 'Saving…' : 'Set default'}
                </button>
              {/if}
            </div>
          </div>
        {/each}
        {#if defaultError}
          <p class="status-message-xs status-error">
            {defaultError}
          </p>
        {/if}
      </div>
    {/if}
  </div>

  <div class="grid gap-4 md:grid-cols-2">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Roles</h2>
      <p class="mt-2 text-sm text-muted">Assign entity owner, accountant, and user roles.</p>
      <div class="mt-4 grid gap-3 text-sm">
        <div class="rounded-xl border border-line px-4 py-3">Admin: full control within an entity.</div>
        <div class="rounded-xl border border-line px-4 py-3">Accountant: manage accounts, rules, transactions.</div>
        <div class="rounded-xl border border-line px-4 py-3">User: capture receipts and post entries.</div>
      </div>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Members</h2>
    <p class="mt-2 text-sm text-muted">Add teammates by email — they need an existing account.</p>
    <div class="mt-4 grid gap-3 text-sm text-muted md:grid-cols-2">
      <label class="grid gap-2 text-sm font-medium text-ink">
        Entity
        <select
          class="rounded-xl border border-line px-3 py-2 text-base"
          bind:value={memberEntityId}
          onchange={(event) => loadMembers(event.currentTarget.value)}
        >
          <option value="">Select entity</option>
          {#each entities as entity}
            <option value={entity.id}>{entity.name}</option>
          {/each}
        </select>
      </label>
    </div>

    <div class="mt-4 grid gap-3 md:grid-cols-[1.6fr_0.6fr_0.6fr]">
      <label class="grid gap-2 text-sm font-medium text-ink md:col-span-2">
        Email
        <input
          class="rounded-xl border border-line px-3 py-2 text-base"
          type="email"
          autocomplete="off"
          bind:value={newMemberEmail}
          placeholder="teammate@example.com"
        />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Role
        <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={newMemberRole}>
          <option value="user">User</option>
          <option value="accountant">Accountant</option>
          <option value="admin">Admin</option>
        </select>
      </label>
    </div>
    <div class="mt-4 flex gap-3">
      <button
        class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
        type="button"
        onclick={addMember}
        disabled={memberSaving || !memberEntityId}
      >
        {memberSaving ? 'Adding…' : 'Add member'}
      </button>
    </div>

    {#if membersError}
      <p class="mt-4 status-message-sm status-error">
        {membersError}
      </p>
    {/if}

    <div class="mt-6">
      <h3 class="text-sm font-semibold">Current members</h3>
      {#if membersLoading}
        <p class="mt-3 text-sm text-muted">Loading members…</p>
      {:else if members.length === 0}
        <p class="mt-3 text-sm text-muted">No members found.</p>
      {:else}
        <div class="mt-3 grid gap-2">
          {#each members as member}
            <div class="grid gap-2 rounded-xl border border-line px-3 py-2 md:grid-cols-[1.6fr_0.6fr_0.6fr] md:items-center">
              <div class="text-sm text-muted">{member.user_id}</div>
              <div>
                <select
                  class="rounded-full border border-line px-3 py-1 text-xs font-semibold"
                  value={member.role}
                  onchange={(event) => updateMember(member.id, event.currentTarget.value)}
                  disabled={memberUpdating[member.id]}
                >
                  <option value="user">User</option>
                  <option value="accountant">Accountant</option>
                  <option value="admin">Admin</option>
                </select>
              </div>
              <div class="flex justify-end">
                <button
                  class="rounded-full border border-line px-3 py-1 text-xs font-semibold disabled:opacity-60"
                  type="button"
                  onclick={() => removeMember(member.id)}
                  disabled={memberRemoving[member.id]}
                >
                  {memberRemoving[member.id] ? 'Removing…' : 'Remove'}
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</section>

