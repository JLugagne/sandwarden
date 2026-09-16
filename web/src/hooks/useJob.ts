import { useEffect, useState } from "react";
import { jobHub, type JobSnapshot } from "@/store/jobs";

const IDLE: JobSnapshot = { output: "", status: "done" };

/** Subscribe to a streamed job's buffered output and status. */
export function useJob(id: string | null): JobSnapshot {
  const [snapshot, setSnapshot] = useState<JobSnapshot>(() => (id ? jobHub.snapshot(id) : IDLE));
  const [syncedId, setSyncedId] = useState<string | null>(id);

  useEffect(() => {
    if (!id) {
      setSyncedId(null);
      setSnapshot(IDLE);
      return;
    }
    setSyncedId(id);
    const update = () => setSnapshot({ ...jobHub.snapshot(id) });
    update();
    return jobHub.subscribe(id, update);
  }, [id]);

  // The effect above runs after the render that switched ids, so return the
  // fresh snapshot for that transition instead of the previous job's state.
  if (id !== syncedId) return id ? jobHub.snapshot(id) : IDLE;
  return snapshot;
}
