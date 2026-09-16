import { Link } from "react-router-dom";
import { Badge, Panel, TableWrap, TD, TH, TRow } from "@/components/ui";
import type { SandboxDetail } from "@/types";

export function SecretsTab({ name, detail }: { name: string; detail: SandboxDetail }) {
  const stored = detail.secrets ?? [];
  const custom = detail.custom_secrets ?? [];

  return (
    <div className="flex flex-col gap-4">
      <Panel
        title="Sandbox-scoped secrets"
        description="Secrets stored for this sandbox only. Global secrets live on the Secrets page."
        actions={
          <Link to={`/secrets?sandbox=${encodeURIComponent(name)}`} className="text-xs text-accent hover:underline">
            Manage secrets
          </Link>
        }
        bodyClassName="p-0"
      >
        {stored.length === 0 && custom.length === 0 ? (
          <p className="p-4 text-sm text-muted">No sandbox-scoped secrets.</p>
        ) : (
          <>
            {stored.length > 0 ? (
              <TableWrap className="border-0 border-b border-border">
                <thead>
                  <tr>
                    <TH>Service / registry</TH>
                    <TH>Type</TH>
                    <TH>Value</TH>
                  </tr>
                </thead>
                <tbody>
                  {stored.map((secret) => (
                    <TRow key={`${secret.type}:${secret.name}`}>
                      <TD className="font-mono text-xs">{secret.name}</TD>
                      <TD>
                        <Badge>{secret.type}</Badge>
                      </TD>
                      <TD className="font-mono text-xs text-muted">{secret.masked}</TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            ) : null}
            {custom.length > 0 ? (
              <TableWrap className="border-0">
                <thead>
                  <tr>
                    <TH>Env</TH>
                    <TH>Hosts</TH>
                    <TH>Placeholder</TH>
                    <TH>Value</TH>
                  </tr>
                </thead>
                <tbody>
                  {custom.map((secret) => (
                    <TRow key={secret.placeholder || secret.env}>
                      <TD className="font-mono text-xs">{secret.env}</TD>
                      <TD className="font-mono text-xs text-muted">{(secret.targets ?? []).join(", ")}</TD>
                      <TD className="font-mono text-xs text-muted">{secret.placeholder || "—"}</TD>
                      <TD className="font-mono text-xs text-muted">{secret.masked}</TD>
                    </TRow>
                  ))}
                </tbody>
              </TableWrap>
            ) : null}
          </>
        )}
      </Panel>
    </div>
  );
}
