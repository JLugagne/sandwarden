import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { Button } from "@/components/ui/Button";
import { CopyButton } from "@/components/ui/CopyButton";
import { ErrorNote, Spinner } from "@/components/ui/Feedback";
import { IconAlert } from "@/components/ui/Icons";
import { Modal } from "@/components/ui/Modal";
import { Tabs } from "@/components/ui/Tabs";

/** One configuration file shown by the viewer, with its per-file error. */
export interface ConfigFileView {
  name: string;
  path: string;
  content: string;
  error?: string;
}

/** The sandbox or profile whose configuration files the viewer shows. */
export interface ConfigTarget {
  kind: "sandbox" | "profile";
  slug: string;
  title?: string;
}

const KEY_LINE = /^(\s*(?:-\s+)?)([A-Za-z0-9_.-]+)(:)(.*)$/;

/**
 * Read-only viewer for the raw spec.yaml and sandwarden.yaml of a sandbox or
 * profile. A file that is missing or invalid is reported in its tab without
 * hiding the raw text, so broken hand-edited files stay inspectable.
 */
export function ConfigViewer({
  open,
  target,
  onClose,
}: {
  open: boolean;
  target: ConfigTarget;
  onClose: () => void;
}) {
  const [active, setActive] = useState("");
  const query = useQuery({
    queryKey: ["config-files", target.kind, target.slug],
    queryFn: () =>
      target.kind === "sandbox" ? api.readSandboxConfig(target.slug) : api.readProfileConfig(target.slug),
    enabled: open,
    retry: 0,
  });

  const files = query.data ?? [];
  const current = files.find((file) => file.name === active) ?? files[0];

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="xl"
      title={`${target.title ?? target.slug} configuration`}
      description="Read-only view of the files edited on disk and converged by Apply."
      footer={
        <Button variant="ghost" onClick={onClose}>
          Close
        </Button>
      }
    >
      {query.isLoading ? (
        <div className="flex justify-center py-10">
          <Spinner />
        </div>
      ) : query.isError ? (
        <ErrorNote error={query.error} />
      ) : files.length === 0 ? (
        <p className="text-sm text-muted">No configuration files found.</p>
      ) : (
        <div className="flex flex-col gap-3">
          <Tabs
            tabs={files.map((file) => ({
              id: file.name,
              label: (
                <span className="inline-flex items-center gap-1.5">
                  {file.name}
                  {file.error ? <IconAlert className="size-3 text-danger" /> : null}
                </span>
              ),
            }))}
            active={current?.name ?? ""}
            onChange={setActive}
          />
          {current ? <ConfigFilePanel file={current} /> : null}
        </div>
      )}
    </Modal>
  );
}

function ConfigFilePanel({ file }: { file: ConfigFileView }) {
  return (
    <div className="flex flex-col gap-2">
      {file.error ? <ErrorNote error={file.error} /> : null}
      <div className="flex flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 truncate font-mono text-xs text-faint" title={file.path}>
          {file.path}
        </code>
        <CopyButton value={file.content} />
      </div>
      {file.content === "" ? (
        <p className="rounded-sm border border-dashed border-border px-3 py-6 text-center text-sm text-muted">
          {file.error ? "No content to show." : "The file is empty."}
        </p>
      ) : (
        <YamlCode text={file.content} />
      )}
    </div>
  );
}

function YamlCode({ text }: { text: string }) {
  const lines = useMemo(() => text.replace(/\r\n/g, "\n").replace(/\n$/, "").split("\n"), [text]);
  return (
    <pre className="max-h-[52vh] overflow-auto rounded-sm border border-border bg-canvas py-2 font-mono text-xs leading-relaxed">
      {lines.map((line, index) => (
        <div key={index} className="flex px-3 hover:bg-hover">
          <span className="w-8 shrink-0 pr-3 text-right text-faint select-none">{index + 1}</span>
          <span className="min-w-0 flex-1 whitespace-pre">{highlightLine(line)}</span>
        </div>
      ))}
    </pre>
  );
}

/** Cheap YAML token coloring: full-line comments and mapping keys only. */
function highlightLine(line: string): React.ReactNode {
  if (line.trimStart().startsWith("#")) {
    return <span className="text-faint italic">{line}</span>;
  }
  const match = KEY_LINE.exec(line);
  if (!match) return line;
  const [, indent, key, colon, rest] = match;
  return (
    <>
      {indent}
      <span className="text-accent">{key}</span>
      <span className="text-faint">{colon}</span>
      {rest}
    </>
  );
}
