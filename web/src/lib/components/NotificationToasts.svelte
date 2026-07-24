<script lang="ts">
  import {
    dismissNotification,
    notifications,
  } from "$lib/stores/notifications";

  function toastClass(kind: "success" | "error" | "info") {
    if (kind === "success") {
      return "border-[color:var(--line)] bg-[color:var(--success-bg)] text-[color:var(--success-fg)]";
    }
    if (kind === "error") {
      return "border-[color:var(--line)] bg-[color:var(--danger-bg)] text-[color:var(--danger-fg)]";
    }
    return "border-[color:var(--line)] bg-[color:var(--info-bg)] text-[color:var(--info-fg)]";
  }
</script>

{#if $notifications.length > 0}
  <div class="pointer-events-none fixed right-4 top-4 z-50 grid w-full max-w-sm gap-3">
    {#each $notifications as notification (notification.id)}
      <div
        class={`pointer-events-auto rounded-2xl border px-4 py-3 text-sm shadow-sm ${toastClass(notification.kind)}`}
        role="status"
        aria-live="polite"
      >
        <div class="flex items-start justify-between gap-3">
          <p>{notification.message}</p>
          <button
            class="rounded-full px-2 py-0.5 text-xs font-semibold"
            type="button"
            onclick={() => dismissNotification(notification.id)}
            aria-label="Dismiss notification"
          >
            Dismiss
          </button>
        </div>
      </div>
    {/each}
  </div>
{/if}
