export type ClassValue = string | false | null | undefined;

/** Tiny class-name joiner; keeps components free of a clsx dependency. */
export function cn(...values: ClassValue[]): string {
  return values.filter(Boolean).join(" ");
}
