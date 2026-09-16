import type { JobEvent } from "@/types";
import type { EventEnvelope } from "@/types";

export type JobStatus = "running" | "done" | "error";

export interface JobSnapshot {
  output: string;
  status: JobStatus;
  error?: string;
}

const EMPTY: JobSnapshot = { output: "", status: "running" };

interface Entry extends JobSnapshot {
  readonly listeners: Set<() => void>;
}

/**
 * In-memory registry for streamed jobs. The event dispatcher forwards
 * `jobs:<id>` envelopes here; components subscribe with `useJob`.
 */
class JobHub {
  private readonly jobs = new Map<string, Entry>();
  private static maxBuffer = 256 * 1024;

  ensure(id: string): void {
    if (!this.jobs.has(id)) {
      this.jobs.set(id, { output: "", status: "running", listeners: new Set() });
    }
  }

  reset(id: string): void {
    const entry = this.jobs.get(id);
    if (entry) {
      entry.output = "";
      entry.status = "running";
      entry.error = undefined;
      this.emit(entry);
    } else {
      this.jobs.set(id, { output: "", status: "running", listeners: new Set() });
    }
  }

  dispatch(id: string, event: JobEvent): void {
    const entry = this.jobs.get(id) ?? { output: "", status: "running" as JobStatus, listeners: new Set() };
    if (event.kind === "output" && event.chunk) {
      entry.output += event.chunk;
      if (entry.output.length > JobHub.maxBuffer) {
        entry.output = entry.output.slice(entry.output.length - JobHub.maxBuffer);
      }
    }
    if (event.kind === "done") {
      entry.status = event.error ? "error" : "done";
      entry.error = event.error;
    }
    this.jobs.set(id, entry);
    this.emit(entry);
  }

  subscribe(id: string, listener: () => void): () => void {
    const entry = this.jobs.get(id) ?? { output: "", status: "running" as JobStatus, listeners: new Set() };
    entry.listeners.add(listener);
    this.jobs.set(id, entry);
    return () => entry.listeners.delete(listener);
  }

  snapshot(id: string): JobSnapshot {
    const entry = this.jobs.get(id);
    if (!entry) return EMPTY;
    return { output: entry.output, status: entry.status, error: entry.error };
  }

  private emit(entry: Entry): void {
    for (const listener of entry.listeners) listener();
  }
}

export const jobHub = new JobHub();

/** Route a realtime envelope to the job hub when it is a job topic. */
export function routeJobEvent(envelope: EventEnvelope): boolean {
  if (!envelope.topic.startsWith("jobs:")) return false;
  const id = envelope.topic.slice("jobs:".length);
  if (id) jobHub.dispatch(id, envelope.data as JobEvent);
  return true;
}

/** A client-generated id lets the stream be tracked before the POST returns. */
export function newJobId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  return `job-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
