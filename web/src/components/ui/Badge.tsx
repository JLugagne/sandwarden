import { cn } from "@/lib/cn";

export type BadgeTone = "neutral" | "accent" | "success" | "warning" | "danger";

const TONES: Record<BadgeTone, string> = {
  neutral: "border-border bg-canvas text-muted",
  accent: "border-accent/40 bg-accent-soft text-accent",
  success: "border-success/40 bg-success-soft text-success",
  warning: "border-warning/40 bg-warning-soft text-warning",
  danger: "border-danger/40 bg-danger-soft text-danger",
};

export function Badge({
  tone = "neutral",
  dot = false,
  className,
  children,
}: {
  tone?: BadgeTone;
  dot?: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-2xs font-medium whitespace-nowrap",
        TONES[tone],
        className,
      )}
    >
      {dot ? <span className="size-1.5 rounded-full bg-current" aria-hidden="true" /> : null}
      {children}
    </span>
  );
}

export function DecisionBadge({ decision }: { decision: string }) {
  const allow = decision.toLowerCase().includes("allow");
  return (
    <Badge tone={allow ? "success" : "danger"}>{decision}</Badge>
  );
}

export function StatusBadge({ running, status }: { running: boolean; status: string }) {
  return (
    <Badge tone={running ? "success" : "neutral"} dot>
      {running ? "running" : status || "stopped"}
    </Badge>
  );
}
