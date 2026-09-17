import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { stalenessKey } from "@/lib/config";

/** How often a focused window stats the loaded config files. */
export const STALENESS_POLL_MS = 4000;

/**
 * Polls the backend for configuration files changed on disk since the last
 * load. The window focus refetch covers editors that write while the app is
 * blurred; the interval covers writes while it stays focused. Every consumer
 * shares one query and one poll.
 */
export function useConfigStaleness() {
  return useQuery({
    queryKey: stalenessKey,
    queryFn: api.configStaleness,
    refetchInterval: STALENESS_POLL_MS,
    refetchOnWindowFocus: true,
    retry: 0,
  });
}
