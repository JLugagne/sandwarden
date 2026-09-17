import { useState } from "react";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { Badge, Button, Modal } from "@/components/ui";
import type { ImportReport } from "@/types";

/**
 * Bulk adoption of daemon sandboxes: writes a config directory for every
 * sandbox that has none and reports which adoptions are incomplete.
 */
export function ImportSandboxesDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [report, setReport] = useState<ImportReport | null>(null);
  const runImport = useApiMutation({
    mutationFn: () => api.importSandboxes([]),
    invalidate: [queryKeys.sandboxes],
    onSuccess: (result) => setReport(result),
  });

  function close() {
    setReport(null);
    runImport.reset();
    onClose();
  }

  const results = report?.results ?? [];

  return (
    <Modal
      open={open}
      onClose={close}
      title="Import existing sandboxes"
      description="Adopt daemon sandboxes that have no config directory."
      size="sm"
      footer={
        report ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="ghost" onClick={close} disabled={runImport.isPending}>
              Cancel
            </Button>
            <Button variant="primary" loading={runImport.isPending} onClick={() => runImport.mutate()}>
              Import
            </Button>
          </>
        )
      }
    >
      <div className="flex flex-col gap-3 text-sm text-muted">
        <p>
          Every daemon sandbox without a config directory is adopted from the daemon state. Sandboxes whose original
          CPU, memory and environment settings are not recoverable are marked incomplete: a recreate uses defaults and
          will not restore them.
        </p>

        {report ? (
          <div className="flex flex-col gap-2">
            <p className="text-xs text-faint">
              {report.created} created · {report.configured} already configured · {report.failed} failed
            </p>
            {results.length > 0 ? (
              <ul className="flex flex-col gap-1.5">
                {results.map((result) => (
                  <li key={result.name} className="flex items-center justify-between gap-3">
                    <span className="truncate font-mono text-xs">{result.name}</span>
                    {result.status === "failed" ? (
                      <Badge tone="danger">{result.error || "failed"}</Badge>
                    ) : result.incomplete ? (
                      <Badge tone="warning">incomplete</Badge>
                    ) : (
                      <Badge tone="success">{result.status}</Badge>
                    )}
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        ) : null}
      </div>
    </Modal>
  );
}
