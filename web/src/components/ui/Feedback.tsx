import { cn } from "@/lib/cn";
import { IconAlert } from "@/components/ui/Icons";

export function Spinner({ className }: { className?: string }) {
  return (
    <span
      className={cn("inline-block size-4 animate-spin rounded-full border-2 border-border-strong border-t-accent", className)}
      aria-label="Loading"
    />
  );
}

export function EmptyState({
  title,
  description,
  action,
  className,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center gap-2 rounded-md border border-dashed border-border px-6 py-10 text-center", className)}>
      <p className="text-sm text-muted">{title}</p>
      {description ? <p className="max-w-md text-xs text-faint">{description}</p> : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}

export function ErrorNote({ error, className }: { error: unknown; className?: string }) {
  if (!error) return null;
  const message = error instanceof Error ? error.message : String(error);
  return (
    <div
      className={cn(
        "flex items-start gap-2 rounded-md border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger",
        className,
      )}
      role="alert"
    >
      <IconAlert className="mt-0.5 size-4 shrink-0" />
      <span className="min-w-0 break-words">{message}</span>
    </div>
  );
}

export function LogView({
  text,
  empty = "No output yet.",
  className,
  autoScrollRef,
}: {
  text: string;
  empty?: string;
  className?: string;
  autoScrollRef?: React.Ref<HTMLPreElement>;
}) {
  return (
    <pre
      ref={autoScrollRef}
      className={cn(
        "max-h-72 min-h-16 overflow-auto rounded-sm border border-border bg-canvas p-3 font-mono text-xs whitespace-pre-wrap text-muted",
        className,
      )}
    >
      {text || empty}
    </pre>
  );
}
