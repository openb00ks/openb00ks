import { browser } from "$app/environment";
import { writable } from "svelte/store";

export type NotificationKind = "success" | "error" | "info";

export type Notification = {
  id: number;
  kind: NotificationKind;
  message: string;
};

const notificationsStore = writable<Notification[]>([]);
let nextNotificationID = 1;

export const notifications = {
  subscribe: notificationsStore.subscribe,
};

export function dismissNotification(id: number) {
  notificationsStore.update((items) => items.filter((item) => item.id !== id));
}

export function pushNotification(
  message: string,
  kind: NotificationKind = "info",
  durationMs = 4000,
) {
  const id = nextNotificationID++;
  notificationsStore.update((items) => [...items, { id, kind, message }]);
  if (browser && durationMs > 0) {
    window.setTimeout(() => {
      dismissNotification(id);
    }, durationMs);
  }
  return id;
}
