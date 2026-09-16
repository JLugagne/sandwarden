import { Events } from "@wailsio/runtime";
import type { EventEnvelope } from "@/types";

export type ConnectionStatus = "connecting" | "online" | "offline";

type EventListener = (event: EventEnvelope) => void;
type StatusListener = (status: ConnectionStatus) => void;

/**
 * Realtime client backed by Wails events. The Go side forwards every hub
 * envelope on one channel ("hub:event"); the interface mirrors the old
 * websocket client so consumers did not have to change. Bindings are
 * in-process, so there is nothing to reconnect to.
 */
export class RealtimeClient {
  private readonly listeners = new Set<EventListener>();
  private readonly statusListeners = new Set<StatusListener>();
  private off: (() => void) | null = null;
  private status: ConnectionStatus = "connecting";

  start(): void {
    if (this.off) return;
    this.off = Events.On("hub:event", (event) => {
      const envelope = event.data as unknown as EventEnvelope | undefined;
      if (!envelope || typeof envelope.topic !== "string") return;
      for (const listener of this.listeners) listener(envelope);
    });
    this.setStatus("online");
  }

  stop(): void {
    this.off?.();
    this.off = null;
    this.setStatus("offline");
  }

  subscribe(listener: EventListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  onStatus(listener: StatusListener): () => void {
    this.statusListeners.add(listener);
    listener(this.status);
    return () => this.statusListeners.delete(listener);
  }

  getStatus(): ConnectionStatus {
    return this.status;
  }

  private setStatus(status: ConnectionStatus): void {
    if (status === this.status) return;
    this.status = status;
    for (const listener of this.statusListeners) listener(status);
  }
}
