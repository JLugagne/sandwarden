import { useState } from "react";
import { api } from "@/api/client";
import { useJob } from "@/hooks/useJob";
import { errorMessage } from "@/hooks/useApiMutation";
import { jobHub, newJobId } from "@/store/jobs";
import { useStickToBottom, useToasts } from "@/components/Toaster";
import { Button, Input, LogView, Panel } from "@/components/ui";

export function TerminalTab({ name, running }: { name: string; running: boolean }) {
  const toast = useToasts();
  const [command, setCommand] = useState("");
  const [jobId, setJobId] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const job = useJob(jobId);
  const logRef = useStickToBottom<HTMLPreElement>(job.output);

  async function run() {
    const trimmed = command.trim();
    if (!trimmed) return;
    const id = newJobId();
    jobHub.reset(id);
    setJobId(id);
    setStarting(true);
    try {
      await api.exec(name, trimmed, id);
    } catch (error) {
      toast.push({ tone: "danger", title: "Command failed to start", body: errorMessage(error) });
    } finally {
      setStarting(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Run a command"
        description={
          running
            ? "Executed with `sbx exec <sandbox> sh -lc <command>`; output streams over the desktop event channel."
            : "The sandbox is stopped; sbx starts it automatically before running the command."
        }
      >
        <div className="flex flex-wrap items-center gap-2">
          <Input
            value={command}
            onChange={(event) => setCommand(event.target.value)}
            placeholder="ls -la"
            className="max-w-xl flex-1 font-mono text-xs"
            onKeyDown={(event) => {
              if (event.key === "Enter") void run();
            }}
          />
          <Button variant="primary" loading={starting} disabled={!command.trim()} onClick={() => void run()}>
            Run
          </Button>
          {job.status === "running" && jobId ? (
            <Button variant="ghost" onClick={() => void api.cancelJob(jobId)}>
              Cancel
            </Button>
          ) : null}
        </div>
        {jobId ? (
          <div className="mt-3 flex flex-col gap-1.5">
            <div className="flex items-center gap-2 text-xs text-faint">
              <span>job {jobId.slice(0, 8)}</span>
              <span
                className={
                  job.status === "error"
                    ? "text-danger"
                    : job.status === "done"
                      ? "text-success"
                      : "text-warning"
                }
              >
                {job.status}
                {job.error ? `: ${job.error}` : ""}
              </span>
            </div>
            <LogView text={job.output} empty="Waiting for output…" autoScrollRef={logRef} className="max-h-96" />
          </div>
        ) : null}
      </Panel>
    </div>
  );
}
