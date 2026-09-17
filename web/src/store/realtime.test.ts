import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { applyEvent, queryKeys } from "./realtime";
import type { SandboxSummary } from "@/types";

function sandbox(name: string): SandboxSummary {
  return {
    name,
    id: name,
    status: "running",
    running: true,
    workspace: "/w",
    mount_policy_denied: false,
    profiles: [],
    run_args: "",
    connect: { run: `sbx run --name ${name}`, shell: `sbx exec -it ${name} bash` },
    cpu_percent: 0,
    memory_used_bytes: 0,
    memory_total_bytes: 0,
  };
}

function client(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

describe("applyEvent", () => {
  it("stores sandbox list snapshots", () => {
    const qc = client();
    applyEvent(qc, { topic: "sandboxes", data: [sandbox("a")], ts: "t" });
    expect(qc.getQueryData(queryKeys.sandboxes)).toEqual([sandbox("a")]);
  });

  it("stores per-sandbox detail snapshots", () => {
    const qc = client();
    const detail = { sandbox: sandbox("a") };
    applyEvent(qc, { topic: "sandbox:a", data: detail, ts: "t" });
    expect(qc.getQueryData(queryKeys.sandbox("a"))).toEqual(detail);
  });

  it("prunes detail caches for sandboxes that disappeared", () => {
    const qc = client();
    qc.setQueryData(queryKeys.sandbox("a"), { sandbox: sandbox("a") });
    qc.setQueryData(queryKeys.sandbox("b"), { sandbox: sandbox("b") });
    applyEvent(qc, { topic: "sandboxes", data: [sandbox("b")], ts: "t" });
    expect(qc.getQueryData(queryKeys.sandbox("a"))).toBeUndefined();
    expect(qc.getQueryData(queryKeys.sandbox("b"))).toBeDefined();
  });

  it("ignores a malformed sandboxes payload instead of pruning detail caches", () => {
    const qc = client();
    qc.setQueryData(queryKeys.sandbox("a"), { sandbox: sandbox("a") });
    applyEvent(qc, { topic: "sandboxes", data: null, ts: "t" });
    expect(qc.getQueryData(queryKeys.sandboxes)).toBeUndefined();
    expect(qc.getQueryData(queryKeys.sandbox("a"))).toBeDefined();
  });

  it("stores profiles, secrets and traffic snapshots", () => {
    const qc = client();
    applyEvent(qc, { topic: "profiles", data: [{ id: 1, name: "p" }], ts: "t" });
    applyEvent(qc, { topic: "secrets", data: { stored: [], custom: [] }, ts: "t" });
    applyEvent(qc, { topic: "traffic", data: { blocked_hosts: [], allowed_hosts: [] }, ts: "t" });
    applyEvent(qc, { topic: "caches", data: [{ id: 7, name: "go-mod" }], ts: "t" });
    expect(qc.getQueryData(queryKeys.profiles)).toEqual([{ id: 1, name: "p" }]);
    expect(qc.getQueryData(queryKeys.secrets)).toEqual({ stored: [], custom: [] });
    expect(qc.getQueryData(queryKeys.traffic)).toEqual({ blocked_hosts: [], allowed_hosts: [] });
    expect(qc.getQueryData(queryKeys.caches)).toEqual([{ id: 7, name: "go-mod" }]);
  });

  it("ignores blocked and job topics", () => {
    const qc = client();
    applyEvent(qc, { topic: "blocked", data: { host: "x" }, ts: "t" });
    applyEvent(qc, { topic: "jobs:1", data: { kind: "output", chunk: "hi" }, ts: "t" });
    expect(qc.getQueryData(["blocked"])).toBeUndefined();
    expect(qc.getQueryData(["jobs:1"])).toBeUndefined();
  });
});
