import { useEffect, useState } from "react";
import { api } from "@/api/client";
import { useApiMutation } from "@/hooks/useApiMutation";
import { queryKeys } from "@/store/realtime";
import { Button, Field, Input, Modal, TextArea } from "@/components/ui";

/**
 * Records the create parameters of an imported config: CPU, memory and
 * environment cannot be read back from the daemon, so the user enters them and
 * saving clears the incomplete marker.
 */
export function CompleteConfigDialog({
  name,
  open,
  onClose,
}: {
  name: string;
  open: boolean;
  onClose: () => void;
}) {
  const [cpus, setCpus] = useState("");
  const [memory, setMemory] = useState("");
  const [env, setEnv] = useState("");

  useEffect(() => {
    if (!open) return;
    setCpus("");
    setMemory("");
    setEnv("");
  }, [open]);

  const save = useApiMutation({
    mutationFn: () =>
      api.completeSandboxConfig(name, {
        cpus: Number.parseInt(cpus, 10) || 0,
        memory: memory.trim(),
        env: env
          .split("\n")
          .map((line) => line.trim())
          .filter(Boolean),
      }),
    success: "Create parameters recorded",
    invalidate: [queryKeys.sandbox(name), queryKeys.sandboxes],
    onSuccess: onClose,
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Complete ${name}'s configuration`}
      description="Record the create parameters the daemon could not report back."
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={save.isPending}>
            Cancel
          </Button>
          <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
            Save
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <p className="text-sm text-muted">
          Saving records these values and marks the config complete. Leave CPU and memory empty to use the defaults.
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="CPUs" hint="Number of CPUs the sandbox was created with.">
            <Input
              value={cpus}
              onChange={(event) => setCpus(event.target.value)}
              inputMode="numeric"
              placeholder="4"
            />
          </Field>
          <Field label="Memory" hint="sbx memory string, for example 8g.">
            <Input value={memory} onChange={(event) => setMemory(event.target.value)} placeholder="8g" />
          </Field>
        </div>
        <Field label="Environment" hint="One KEY=VALUE per line; replaces the recorded environment.">
          <TextArea
            value={env}
            onChange={(event) => setEnv(event.target.value)}
            placeholder={"LOG_LEVEL=debug\nFEATURE_X=1"}
            className="font-mono"
          />
        </Field>
      </div>
    </Modal>
  );
}
