import { describe, expect, it } from "vitest";
import { classifyTraffic, groupBySandbox, hostMatches, trafficKey } from "@/lib/traffic";
import type { LogEntry, PolicyRule } from "@/types";

function entry(host: string, vm = "box"): LogEntry {
  return { host, vm_name: vm, proxy_type: "https", rule: "", last_seen: "", since: "", count_since: 1 };
}

function rule(decision: string, resources: string[], sandbox = ""): PolicyRule {
  return {
    id: `${decision}-${resources.join()}-${sandbox}`,
    name: "",
    policy_id: "",
    scope: sandbox ? "local" : "global",
    applies_to: "",
    sandbox_id: sandbox || undefined,
    resource_type: "domain",
    decision,
    resources,
    origin: "",
    status: "",
    editable: true,
  };
}

describe("hostMatches", () => {
  it("matches exact hosts, wildcards and ignores case and ports", () => {
    expect(hostMatches("api.example.com", "API.example.com:443")).toBe(true);
    expect(hostMatches("*.example.com", "a.b.example.com")).toBe(true);
    expect(hostMatches("*.example.com", "example.com")).toBe(false);
    expect(hostMatches("**.example.com", "cdn.example.com")).toBe(true);
    expect(hostMatches("*", "anything.test")).toBe(true);
    expect(hostMatches("example.com", "notexample.com")).toBe(false);
  });
});

describe("classifyTraffic", () => {
  it("moves blocked hosts covered by a rule out of pending", () => {
    const log = {
      blocked_hosts: [entry("new.test"), entry("allowed.test"), entry("denied.test"), entry("other.test", "box-2")],
      allowed_hosts: [entry("seen.test")],
    };
    const rules = [rule("allow", ["allowed.test"], "box"), rule("deny", ["denied.test"]), rule("allow", ["other.test"], "box")];

    const view = classifyTraffic(log, rules);

    expect(view.pending.map(trafficKey)).toEqual(["box\u0000new.test", "box-2\u0000other.test"]);
    expect(view.denied.map((e) => e.host)).toEqual(["denied.test"]);
    expect(view.allowed.map((e) => e.host).sort()).toEqual(["allowed.test", "seen.test"]);
  });

  it("lets a deny rule win over an allow rule", () => {
    const view = classifyTraffic({ blocked_hosts: [entry("x.test")], allowed_hosts: [] }, [
      rule("allow", ["x.test"]),
      rule("deny", ["*.test"]),
    ]);
    expect(view.denied).toHaveLength(1);
    expect(view.pending).toHaveLength(0);
  });

  it("does not list a host twice when it is both logged allowed and newly allowed", () => {
    const view = classifyTraffic({ blocked_hosts: [entry("x.test")], allowed_hosts: [entry("x.test")] }, [rule("allow", ["x.test"])]);
    expect(view.allowed).toHaveLength(1);
  });
});

describe("groupBySandbox", () => {
  it("batches hosts per sandbox without duplicates", () => {
    const groups = groupBySandbox([entry("a.test"), entry("b.test"), entry("a.test"), entry("c.test", "box-2")]);
    expect(groups).toEqual({ box: ["a.test", "b.test"], "box-2": ["c.test"] });
  });
});
