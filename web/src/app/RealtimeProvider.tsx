import { createContext, useContext, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { RealtimeClient, type ConnectionStatus } from "@/api/ws";
import { applyEvent } from "@/store/realtime";
import { routeJobEvent } from "@/store/jobs";
import { useToasts } from "@/components/Toaster";
import { notifyDesktop } from "@/lib/notifications";
import type { BlockedEvent } from "@/types";

const RealtimeContext = createContext<{ status: ConnectionStatus }>({ status: "connecting" });

export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const toast = useToasts();
  const [status, setStatus] = useState<ConnectionStatus>("connecting");

  useEffect(() => {
    const client = new RealtimeClient();
    let wasOffline = false;

    const offStatus = client.onStatus((next) => {
      setStatus(next);
      if (next === "online" && wasOffline) {
        // Reconnected: snapshots may have been missed while offline.
        void queryClient.invalidateQueries();
      }
      if (next === "offline") wasOffline = true;
    });

    const offEvent = client.subscribe((event) => {
      if (routeJobEvent(event)) return;
      if (event.topic === "blocked") {
        const data = event.data as BlockedEvent;
        toast.push({
          tone: "warning",
          title: `Blocked: ${data.host}`,
          body: `sandbox ${data.sandbox} · rule ${data.rule}`,
        });
        notifyDesktop(`Traffic blocked in ${data.sandbox}`, data.host);
        return;
      }
      applyEvent(queryClient, event);
    });

    client.start();
    return () => {
      offStatus();
      offEvent();
      client.stop();
    };
  }, [queryClient, toast]);

  return <RealtimeContext.Provider value={{ status }}>{children}</RealtimeContext.Provider>;
}

export function useConnectionStatus(): ConnectionStatus {
  return useContext(RealtimeContext).status;
}
