import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/cn";
import { IconAlert, IconCheck, IconInfo, IconX } from "@/components/ui/Icons";

export type ToastTone = "info" | "success" | "warning" | "danger";

export interface Toast {
  id: number;
  title: string;
  body?: string;
  tone: ToastTone;
}

interface ToastApi {
  push: (toast: Omit<Toast, "id">) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

const TONE_ICON = {
  info: IconInfo,
  success: IconCheck,
  warning: IconAlert,
  danger: IconAlert,
} as const;

const TONE_CLASS: Record<ToastTone, string> = {
  info: "text-accent",
  success: "text-success",
  warning: "text-warning",
  danger: "text-danger",
};

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const push = useCallback((toast: Omit<Toast, "id">) => {
    const id = nextId.current++;
    setToasts((current) => [...current.slice(-4), { ...toast, id }]);
    window.setTimeout(() => {
      setToasts((current) => current.filter((item) => item.id !== id));
    }, toast.tone === "danger" ? 9000 : 6000);
  }, []);

  const api = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      {createPortal(
        <div className="pointer-events-none fixed right-4 bottom-4 z-[80] flex w-96 max-w-[calc(100vw-2rem)] flex-col gap-2">
          {toasts.map((toast) => {
            const Icon = TONE_ICON[toast.tone];
            return (
              <div
                key={toast.id}
                className="pointer-events-auto flex items-start gap-3 rounded-lg border border-border bg-raised p-3 shadow-lg"
              >
                <Icon className={cn("mt-0.5 size-4 shrink-0", TONE_CLASS[toast.tone])} />
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium break-words">{toast.title}</div>
                  {toast.body ? <div className="mt-0.5 text-xs break-words text-muted">{toast.body}</div> : null}
                </div>
                <button
                  type="button"
                  aria-label="Dismiss"
                  className="rounded p-1 text-faint transition-colors hover:text-fg"
                  onClick={() => setToasts((current) => current.filter((item) => item.id !== toast.id))}
                >
                  <IconX className="size-3.5" />
                </button>
              </div>
            );
          })}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

export function useToasts(): ToastApi {
  const api = useContext(ToastContext);
  if (!api) throw new Error("useToasts must be used inside ToastProvider");
  return api;
}

/** Auto-scroll a container to the bottom while it is pinned near the bottom. */
export function useStickToBottom<T extends HTMLElement>(dependency: unknown) {
  const ref = useRef<T | null>(null);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const nearBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 48;
    if (nearBottom) element.scrollTop = element.scrollHeight;
  }, [dependency]);
  return ref;
}
