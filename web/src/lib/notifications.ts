import { api } from "@/api/client";
import { cachedConfig, loadConfig, updateConfig } from "@/lib/config";

/** Whether native desktop notifications are enabled for this installation. */
export function notificationsEnabled(): boolean {
  return cachedConfig().notifications;
}

/** Loads the backend configuration and returns the notification preference. */
export async function fetchNotificationsEnabled(): Promise<boolean> {
  return (await loadConfig()).notifications;
}

/** Persists the desktop notification preference to the backend config file. */
export async function setNotificationsEnabled(enabled: boolean): Promise<void> {
  await updateConfig({ notifications: enabled });
}

/** Sends a native desktop notification when the preference is enabled. */
export function notifyDesktop(title: string, body: string): void {
  if (!notificationsEnabled()) return;
  void api.notify(title, body).catch(() => undefined);
}
