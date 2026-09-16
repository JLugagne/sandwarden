import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { HashRouter } from "react-router-dom";
import { App } from "@/App";
import { RealtimeProvider } from "@/app/RealtimeProvider";
import { ToastProvider } from "@/components/Toaster";
import { installLinkGuard } from "@/lib/links";
import "@/index.css";

installLinkGuard();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 5_000,
      refetchOnWindowFocus: false,
    },
  },
});

const root = document.getElementById("root");
if (!root) throw new Error("missing #root element");

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <RealtimeProvider>
          <HashRouter>
            <App />
          </HashRouter>
        </RealtimeProvider>
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>,
);
