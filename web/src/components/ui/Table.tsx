import { cn } from "@/lib/cn";

export function TableWrap({ className, children }: { className?: string; children: React.ReactNode }) {
  return (
    <div className={cn("overflow-x-auto rounded-md border border-border", className)}>
      <table className="w-full border-collapse text-sm">{children}</table>
    </div>
  );
}

export function TH({ className, children }: { className?: string; children?: React.ReactNode }) {
  return (
    <th
      className={cn(
        "border-b border-border bg-canvas px-3 py-2 text-left text-2xs font-semibold tracking-wide text-faint uppercase",
        className,
      )}
    >
      {children}
    </th>
  );
}

export function TD({ className, children, colSpan }: { className?: string; children?: React.ReactNode; colSpan?: number }) {
  return (
    <td colSpan={colSpan} className={cn("border-b border-border px-3 py-2 align-middle", className)}>
      {children}
    </td>
  );
}

export function TRow({ className, children }: { className?: string; children: React.ReactNode }) {
  return <tr className={cn("last:[&>td]:border-b-0 hover:bg-hover/40", className)}>{children}</tr>;
}
