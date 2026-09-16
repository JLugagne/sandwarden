import { useState } from "react";
import { cn } from "@/lib/cn";
import { IconCheck, IconCopy } from "@/components/ui/Icons";
import { OpenInTerminalButton } from "@/components/ui/OpenInTerminalButton";

export function CopyButton({
  value,
  label = "Copy",
  className,
}: {
  value: string;
  label?: string;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={cn(
        "inline-flex cursor-pointer items-center gap-1.5 rounded-sm border border-border bg-canvas px-2 py-1 text-2xs text-muted transition-colors hover:border-border-strong hover:text-fg",
        className,
      )}
      onClick={() => {
        if (!navigator.clipboard) return;
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1200);
        });
      }}
    >
      {copied ? <IconCheck className="size-3 text-success" /> : <IconCopy className="size-3" />}
      {copied ? "Copied" : label}
    </button>
  );
}

export function CommandLine({
  command,
  className,
  openDir,
}: {
  command: string;
  className?: string;
  /** When set, an Open button launches the command in a terminal, cd'd here. */
  openDir?: string;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-sm border border-border bg-canvas px-3 py-2",
        className,
      )}
    >
      <code className="min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-nowrap text-muted">{command}</code>
      {openDir ? <OpenInTerminalButton dir={openDir} command={command} /> : null}
      <CopyButton value={command} />
    </div>
  );
}
