<script lang="ts">
  import { currentUser, isAdmin } from "$lib/stores/current-user";

  interface Props {
    children?: import("svelte").Snippet;
  }

  let { children }: Props = $props();
</script>

{#if $currentUser === null}
  <div class="rounded-2xl border border-line bg-surface px-6 py-8 text-sm text-muted">
    Checking access…
  </div>
{:else if $isAdmin}
  {@render children?.()}
{:else}
  <section class="mx-auto max-w-lg rounded-2xl border border-line bg-surface px-6 py-10 text-center">
    <h1 class="text-lg font-semibold text-ink">Admins only</h1>
    <p class="mt-2 text-sm text-muted">
      You don't have access to this page. Ask an administrator if you need it.
    </p>
    <a class="mt-6 inline-flex rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper" href="/">
      Back to home
    </a>
  </section>
{/if}
