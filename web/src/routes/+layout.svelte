<script lang="ts">
  import { browser } from "$app/environment";
  import { goto } from "$app/navigation";
  import { page } from "$app/stores";
  import "../app.css";
  import NotificationToasts from "$lib/components/NotificationToasts.svelte";
  import SearchModal from "$lib/components/SearchModal.svelte";
  import { openSearch } from "$lib/stores/search";
  import { apiFetchPublic } from "$lib/api/client";
  import { clearSession, session } from "$lib/stores/session";
  import {
    activeEntity,
    clearActiveEntity,
    entities,
    initEntity,
    selectEntity,
  } from "$lib/stores/entity";
  import {
    currentUser,
    isAdmin,
    loadCurrentUser,
    clearCurrentUser,
  } from "$lib/stores/current-user";
  import { publicRegistrationEnabled } from "$lib/utils/auth-mode";
  import { resolveBootstrapTarget } from "$lib/utils/bootstrap";
  import { shouldRenderRouteContent } from "$lib/utils/auth-gating";
  import { isUnauthenticatedPublicRoute } from "$lib/utils/setup";
  import { initTheme } from "$lib/stores/theme";
  interface Props {
    children?: import('svelte').Snippet;
  }

  let { children }: Props = $props();

  let navOpen = $state(false);
  let userMenuOpen = $state(false);
  let bootstrapped = $state(false);
  let setupChecked = $state(false);
  const registrationEnabled = publicRegistrationEnabled();

  let hasEntities = $derived($entities.length > 0);
  let currentEntity =
    $derived($entities.find((entity) => entity.id === $activeEntity) ?? null);
  let showRouteContent = $derived(
    isPublicRoute($page.url.pathname) ||
      (setupChecked &&
        shouldRenderRouteContent(Boolean($session), $page.url.pathname, registrationEnabled)),
  );
  // App chrome (sidebar nav + header controls) renders only for an authenticated session AND not on a
  // public route — so a stale/expired session can never leave the nav + profile button showing over the
  // login screen.
  let showChrome = $derived(Boolean($session) && !isPublicRoute($page.url.pathname));

  const workflowGroups = [
    {
      label: "Capture",
      items: [
        { href: "/receipts", label: "Receipts" },
        { href: "/imports", label: "Imports" },
        { href: "/mileage", label: "Mileage" },
      ],
    },
    {
      label: "Review",
      items: [{ href: "/review", label: "Review queue" }],
    },
          {
        label: "Books",
        items: [
          { href: "/transactions", label: "Transactions" },
          { href: "/search", label: "Search" },
          { href: "/accounts", label: "Accounts" },
          { href: "/statements", label: "Statements" },
          { href: "/vendors", label: "Vendors" },
          { href: "/vendor-rules", label: "Vendor rules" },
          { href: "/tax-prep", label: "Tax prep" },
          { href: "/reports", label: "Reports" },
          { href: "/exports", label: "Exports" },
        ],
      },
  ];

  function isPublicRoute(pathname: string) {
    return isUnauthenticatedPublicRoute(pathname, registrationEnabled);
  }

  function isActiveHref(pathname: string, href: string) {
    if (href === "/") {
      return pathname === "/";
    }
    return pathname === href || pathname.startsWith(`${href}/`);
  }

  function navLinkClass(pathname: string, href: string) {
    return isActiveHref(pathname, href)
      ? "rounded-xl bg-primary/10 px-3 py-2 font-semibold text-ink"
      : "rounded-xl px-3 py-2 text-muted hover:bg-paper hover:text-ink";
  }

  function closeMobileNav() {
    navOpen = false;
  }

  async function resolveBootstrapRoute(pathname: string) {
    try {
      const status = await apiFetchPublic<{ required: boolean }>("/setup/status");
      setupChecked = true;
      if (status.required) {
        clearSession();
        entities.set([]);
        clearActiveEntity();
      }
      const target = resolveBootstrapTarget(
        status.required,
        pathname,
        Boolean($session),
        registrationEnabled,
      );
      if (target && $page.url.pathname === pathname) {
        await goto(target);
      }
    } catch {
      setupChecked = true;
      if ($page.url.pathname === pathname && !$session && !isPublicRoute(pathname)) {
        await goto("/login");
      }
    }
  }

  $effect(() => {
    if (browser && $session && setupChecked && !bootstrapped) {
      initTheme();
      initEntity();
      void loadCurrentUser();
      bootstrapped = true;
    }
  });

  $effect(() => {
    if (browser && !$session) {
      bootstrapped = false;
      clearCurrentUser();
    }
  });

  let userInitial = $derived(($currentUser?.email ?? "").trim().charAt(0).toUpperCase() || "U");

  $effect(() => {
    if (browser && !setupChecked) {
      void resolveBootstrapRoute($page.url.pathname);
    }
  });
</script>

<NotificationToasts />
{#if showChrome}
  <SearchModal />
{/if}
<div class={showChrome ? "app-shell min-h-screen bg-paper lg:grid lg:grid-cols-[18rem_minmax(0,1fr)]" : "app-shell min-h-screen bg-paper"}>
  {#if showChrome}
    <aside class="hidden lg:flex lg:h-screen lg:flex-col lg:border-r lg:border-line lg:bg-surface">
      <div class="flex h-full flex-col">
        <div class="border-b border-line px-5 py-5">
          <a class="inline-flex items-center" href="/" aria-label="Open B00KS home">
            <img
              class="h-14 w-auto max-w-[13rem] object-contain"
              src="/logo.jpg"
              alt="Open B00KS"
            />
          </a>
          <p class="mt-4 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
            Workspace
          </p>
          <p class="mt-1 text-sm font-semibold text-ink">
            {#if $activeEntity}
              {currentEntity?.name ?? "Selected entity"}
            {:else if hasEntities}
              Choose an entity
            {:else}
              No active entity
            {/if}
          </p>
          {#if hasEntities}
            <label class="mt-4 grid gap-2 text-xs font-semibold text-muted">
              <span>Active entity</span>
              <select
                class="rounded-xl border border-line bg-surface px-3 py-2 text-sm text-ink focus:outline-none"
                value={$activeEntity ?? ""}
                onchange={(event) => selectEntity(event.currentTarget.value)}
              >
                <option class="bg-surface text-ink" value="">Select entity</option>
                {#each $entities as entity}
                  <option class="bg-surface text-ink" value={entity.id}>{entity.name}</option>
                {/each}
              </select>
            </label>
          {/if}
        </div>

        <nav class="flex-1 overflow-y-auto px-3 py-4 text-sm">
          <div class="grid gap-1">
            <a class={navLinkClass($page.url.pathname, "/")} href="/">Home</a>
            {#if $activeEntity}
              <a class={navLinkClass($page.url.pathname, "/entity")} href="/entity">
                Entity dashboard
              </a>
            {:else}
              <a class={navLinkClass($page.url.pathname, "/entities")} href="/entities">
                Manage entities
              </a>
            {/if}
          </div>

          {#if $activeEntity}
            {#each workflowGroups as group}
              <section class="mt-6">
                <p class="px-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
                  {group.label}
                </p>
                <div class="mt-2 grid gap-1">
                  {#each group.items as item}
                    <a class={navLinkClass($page.url.pathname, item.href)} href={item.href}>
                      {item.label}
                    </a>
                  {/each}
                </div>
              </section>
            {/each}
          {/if}

          <section class="mt-6">
            <p class="px-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
              Settings
            </p>
            <div class="mt-2 grid gap-1">
              <a class={navLinkClass($page.url.pathname, "/settings/user")} href="/settings/user">
                User settings
              </a>
              {#if $activeEntity}
                <a class={navLinkClass($page.url.pathname, "/settings/entity")} href="/settings/entity">
                  Entity settings
                </a>
              {/if}
              {#if $isAdmin}
                <a class={navLinkClass($page.url.pathname, "/users")} href="/users">
                  User management
                </a>
                <a class={navLinkClass($page.url.pathname, "/settings/system")} href="/settings/system">
                  System settings
                </a>
                <a class={navLinkClass($page.url.pathname, "/admin")} href="/admin">
                  Admin dashboard
                </a>
              {/if}
            </div>
          </section>

        </nav>

        <div class="border-t border-line px-5 py-4 text-[11px] text-muted">
          <p>© 2026 Spectrum Labs LLC</p>
          <p>All rights reserved.</p>
        </div>
      </div>
    </aside>
  {/if}

  <div class="flex min-w-0 flex-col">
    <header class="sticky top-0 z-20 border-b border-line bg-surface/80 backdrop-blur">
      <div class="mx-auto flex max-w-5xl items-center justify-between gap-3 px-4 py-4 lg:max-w-none lg:px-8">
        <div class="flex items-center gap-3">
          <a class="inline-flex items-center lg:hidden" href="/" aria-label="Open B00KS home">
            <img
              class="h-12 w-auto max-w-[10rem] object-contain"
              src="/logo.jpg"
              alt="Open B00KS"
            />
          </a>
          {#if showChrome && hasEntities && currentEntity}
            <span class="hidden rounded-full border border-line bg-surface px-3 py-1 text-xs font-semibold text-muted md:inline-flex">
              {currentEntity.name}
            </span>
          {/if}
        </div>

        <div class="flex items-center gap-2">
          {#if showChrome}
            <button
              class="flex items-center gap-2 rounded-full border border-line bg-surface px-3 py-1.5 text-xs font-semibold text-muted hover:text-ink sm:min-w-[13rem] sm:justify-start"
              type="button"
              aria-label="Search (Ctrl+K)"
              onclick={openSearch}
            >
              <svg class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" d="m21 21-4.35-4.35M17 11A6 6 0 1 1 5 11a6 6 0 0 1 12 0Z" />
              </svg>
              <span class="hidden sm:inline">Search…</span>
              <kbd class="ml-auto hidden rounded border border-line px-1.5 py-0.5 text-[10px] font-semibold sm:inline">Ctrl K</kbd>
            </button>
          {/if}
          {#if showChrome}
            <div class="relative hidden sm:block">
              <button
                class="flex items-center gap-2 rounded-full border border-line bg-surface px-3 py-1 text-xs font-semibold text-muted"
                type="button"
                aria-expanded={userMenuOpen}
                onclick={() => (userMenuOpen = !userMenuOpen)}
              >
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-paper text-ink">{userInitial}</span>
                <span class="max-w-[10rem] truncate">{$currentUser?.email ?? "Account"}</span>
              </button>
              {#if userMenuOpen}
                <div class="absolute right-0 mt-2 w-56 rounded-2xl border border-line bg-surface p-2 text-sm text-muted shadow-sm">
                  {#if $currentUser}
                    <p class="truncate px-3 py-2 text-xs text-muted">
                      Signed in as <span class="font-semibold text-ink">{$currentUser.email}</span>
                    </p>
                  {/if}
                  <a class="block rounded-lg px-3 py-2 hover:text-ink" href="/settings/user">User settings</a>
                  {#if $isAdmin}
                    <a class="block rounded-lg px-3 py-2 hover:text-ink" href="/settings/system">System settings</a>
                  {/if}
                  <a class="block rounded-lg px-3 py-2 hover:text-ink" href="/logout">Sign out</a>
                </div>
              {/if}
            </div>
          {/if}

          {#if showChrome}
            <button
              class="rounded-full border border-line px-3 py-1 text-xs font-semibold text-muted lg:hidden"
              type="button"
              aria-expanded={navOpen}
              onclick={() => (navOpen = !navOpen)}
            >
              Menu
            </button>
          {/if}
        </div>
      </div>

      {#if showChrome}
        <div class={`${navOpen ? "block" : "hidden"} border-t border-line bg-surface lg:hidden`}>
          <div class="mx-auto max-h-[calc(100vh-4rem)] max-w-5xl overflow-y-auto px-4 py-4">
            <div class="grid gap-4 text-sm">
              {#if $activeEntity && hasEntities}
                <div class="rounded-2xl border border-line bg-paper px-4 py-4">
                  <p class="text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
                    Active entity
                  </p>
                  <p class="mt-1 text-sm font-semibold text-ink">
                    {currentEntity?.name ?? "Selected entity"}
                  </p>
                  <label class="mt-4 grid gap-2 text-xs font-semibold text-muted">
                    <span>Switch entity</span>
                    <select
                      class="rounded-xl border border-line bg-surface px-3 py-2 text-sm text-ink focus:outline-none"
                      value={$activeEntity ?? ""}
                      onchange={(event) => selectEntity(event.currentTarget.value)}
                    >
                      <option class="bg-surface text-ink" value="">Select entity</option>
                      {#each $entities as entity}
                        <option class="bg-surface text-ink" value={entity.id}>{entity.name}</option>
                      {/each}
                    </select>
                  </label>
                </div>
              {/if}

              <div class="grid gap-1">
                <a class={navLinkClass($page.url.pathname, "/")} href="/" onclick={closeMobileNav}>Home</a>
                {#if $activeEntity}
                  <a class={navLinkClass($page.url.pathname, "/entity")} href="/entity" onclick={closeMobileNav}>
                    Entity dashboard
                  </a>
                  {#each workflowGroups as group}
                    <div class="mt-2">
                      <p class="px-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
                        {group.label}
                      </p>
                      <div class="mt-1 grid gap-1">
                        {#each group.items as item}
                          <a class={navLinkClass($page.url.pathname, item.href)} href={item.href} onclick={closeMobileNav}>
                            {item.label}
                          </a>
                        {/each}
                      </div>
                    </div>
                  {/each}
                {:else}
                  <a class={navLinkClass($page.url.pathname, "/entities")} href="/entities" onclick={closeMobileNav}>
                    Manage entities
                  </a>
                {/if}
                <div class="mt-2">
                  <p class="px-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
                    Settings
                  </p>
                  <div class="mt-1 grid gap-1">
                    <a class={navLinkClass($page.url.pathname, "/settings/user")} href="/settings/user" onclick={closeMobileNav}>
                      User settings
                    </a>
                    {#if $activeEntity}
                      <a class={navLinkClass($page.url.pathname, "/settings/entity")} href="/settings/entity" onclick={closeMobileNav}>
                        Entity settings
                      </a>
                    {/if}
                    {#if $isAdmin}
                      <a class={navLinkClass($page.url.pathname, "/users")} href="/users" onclick={closeMobileNav}>
                        User management
                      </a>
                      <a class={navLinkClass($page.url.pathname, "/settings/system")} href="/settings/system" onclick={closeMobileNav}>
                        System settings
                      </a>
                      <a class={navLinkClass($page.url.pathname, "/admin")} href="/admin" onclick={closeMobileNav}>
                        Admin dashboard
                      </a>
                    {/if}
                  </div>
                </div>
                <a class={navLinkClass($page.url.pathname, "/logout")} href="/logout" onclick={closeMobileNav}>
                  Sign out
                </a>
              </div>
            </div>
          </div>
        </div>
      {/if}
    </header>

    <main class="app-main w-full flex-1 px-4 py-8 lg:px-8">
      {#if showRouteContent}
        {@render children?.()}
      {:else}
        <div class="min-h-[24rem] rounded-3xl border border-line bg-surface px-6 py-8 text-sm text-muted">
          Checking access…
        </div>
      {/if}
    </main>
  </div>
</div>
