import { forwardRef, useId } from "react";
import { cn } from "@/lib/cn";

export function Field({
  label,
  hint,
  htmlFor,
  className,
  children,
}: {
  label?: React.ReactNode;
  hint?: React.ReactNode;
  htmlFor?: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label className={cn("flex min-w-0 flex-col gap-1.5", className)} htmlFor={htmlFor}>
      {label ? <span className="text-xs font-medium text-muted">{label}</span> : null}
      {children}
      {hint ? <span className="text-2xs text-faint">{hint}</span> : null}
    </label>
  );
}

const CONTROL_CLASS =
  "w-full rounded-sm border border-border bg-canvas px-2.5 py-1.5 text-sm text-fg placeholder:text-faint transition-colors focus:border-accent disabled:cursor-not-allowed disabled:opacity-50";

export const Input = forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className, ...props }, ref) {
    return <input ref={ref} className={cn(CONTROL_CLASS, "h-8", className)} {...props} />;
  },
);

export const TextArea = forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function TextArea({ className, ...props }, ref) {
    return <textarea ref={ref} className={cn(CONTROL_CLASS, "min-h-20 resize-y", className)} {...props} />;
  },
);

export const Select = forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement>>(
  function Select({ className, children, ...props }, ref) {
    return (
      <select ref={ref} className={cn(CONTROL_CLASS, "h-8 pr-7", className)} {...props}>
        {children}
      </select>
    );
  },
);

export function Checkbox({
  label,
  checked,
  onChange,
  disabled,
  className,
}: {
  label: React.ReactNode;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
}) {
  const id = useId();
  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <input
        id={id}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
        className="size-3.5 cursor-pointer accent-[var(--sbx-accent)] disabled:cursor-not-allowed"
      />
      <label htmlFor={id} className="cursor-pointer text-xs text-muted select-none">
        {label}
      </label>
    </span>
  );
}

/** Labelled checkbox on its own line, for form grids. */
export function CheckboxField({
  label,
  hint,
  checked,
  onChange,
}: {
  label: React.ReactNode;
  hint?: React.ReactNode;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex flex-col gap-1">
      <Checkbox label={label} checked={checked} onChange={onChange} />
      {hint ? <span className="pl-5.5 text-2xs text-faint">{hint}</span> : null}
    </div>
  );
}
