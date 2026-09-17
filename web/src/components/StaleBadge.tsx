import { Badge } from "@/components/ui";

/** Marks an entity whose configuration file changed on disk outside the app. */
export function StaleBadge() {
  return (
    <span title="Edited outside the app; reload from disk to pick up the change.">
      <Badge tone="warning">changed on disk</Badge>
    </span>
  );
}
