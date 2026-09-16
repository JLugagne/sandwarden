import { api } from "@/api/client";

const STORAGE_KEY = "sandwarden.notifications";

/** Whether native desktop notifications are enabled for this installation. */
export function notificationsEnabled(): boolean {
  return localStorage.getItem(STORAGE_KEY) === "1";
}

/** Persists the desktop notification preference. */
export function setNotificationsEnabled(enabled: boolean): void {
  localStorage.setItem(STORAGE_KEY, enabled ? "1" : "0");
}

/** Sends a native desktop notification when the preference is enabled. */
export function notifyDesktop(title: string, body: string): void {
  if (!notificationsEnabled()) return;
  void api.notify(title, body).catch(() => undefined);
}
