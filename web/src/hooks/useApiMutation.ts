import { useMutation, useQueryClient, type QueryKey } from "@tanstack/react-query";
import { useToasts } from "@/components/Toaster";

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/**
 * Mutation wrapper that surfaces failures as toasts and, on success, can push a
 * toast and invalidate query keys.
 */
export function useApiMutation<TData, TVariables = void>(options: {
  mutationFn: (variables: TVariables) => Promise<TData>;
  success?: string | ((data: TData, variables: TVariables) => string | undefined);
  invalidate?: QueryKey[];
  onSuccess?: (data: TData, variables: TVariables) => void;
}) {
  const toast = useToasts();
  const queryClient = useQueryClient();
  return useMutation<TData, Error, TVariables>({
    mutationFn: options.mutationFn,
    onSuccess: (data, variables) => {
      const message =
        typeof options.success === "function" ? options.success(data, variables) : options.success;
      if (message) toast.push({ tone: "success", title: message });
      for (const key of options.invalidate ?? []) void queryClient.invalidateQueries({ queryKey: key });
      options.onSuccess?.(data, variables);
    },
    onError: (error) => {
      toast.push({ tone: "danger", title: "Request failed", body: errorMessage(error) });
    },
  });
}
