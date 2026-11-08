import { describe, expect, it } from "vitest";
import type { SnapshotPolicy } from "@/api/types";
import {
  initialSnapshotPolicyForm,
  snapshotPolicyFormFromPolicy,
  snapshotPolicyInputFromForm,
} from "@/features/vms/vm-snapshot-policy-model";

describe("snapshot policy form model", () => {
  it("normalizes a valid form into the API input", () => {
    const result = snapshotPolicyInputFromForm({
      name: " nightly ",
      intervalMinutes: "60",
      retention: "3",
      enabled: true,
    });
    expect(result.error).toBe("");
    expect(result.input).toEqual({ name: "nightly", intervalMinutes: 60, retention: 3, enabled: true });
  });

  it("rejects unsafe names and out-of-range schedule values", () => {
    expect(snapshotPolicyInputFromForm({ ...initialSnapshotPolicyForm(), name: "../nightly" }).error).toContain("策略名称");
    expect(snapshotPolicyInputFromForm({ ...initialSnapshotPolicyForm(), name: "nightly", intervalMinutes: "4" }).error).toContain("间隔");
    expect(snapshotPolicyInputFromForm({ ...initialSnapshotPolicyForm(), name: "nightly", intervalMinutes: "60.5" }).error).toContain("间隔");
    expect(snapshotPolicyInputFromForm({ ...initialSnapshotPolicyForm(), name: "nightly", retention: "51" }).error).toContain("保留数量");
  });

  it("maps an existing policy into editable string fields", () => {
    const policy: SnapshotPolicy = {
      id: 7,
      vmUuid: "vm-a",
      name: "weekly",
      intervalMinutes: 120,
      retention: 5,
      enabled: false,
      nextRunAt: "2026-05-01T10:00:00Z",
      createdAt: "2026-05-01T08:00:00Z",
      updatedAt: "2026-05-01T08:00:00Z",
    };
    expect(snapshotPolicyFormFromPolicy(policy)).toEqual({
      name: "weekly",
      intervalMinutes: "120",
      retention: "5",
      enabled: false,
    });
  });
});
