import { describe, expect, it } from "vitest";
import {
  configVersionFilename,
  configVersionRollbackReason,
  configVersionSourceLabel,
  shortConfigVersionSHA,
} from "@/features/vms/vm-config-version-model";

describe("vm config version model", () => {
  it("labels all config change sources", () => {
    expect(configVersionSourceLabel("manual_xml")).toBe("手动编辑 XML");
    expect(configVersionSourceLabel("external")).toBe("外部工具修改");
    expect(configVersionSourceLabel("pre_rollback")).toBe("回滚前自动存档");
  });

  it("formats hashes for compact timeline display", () => {
    expect(shortConfigVersionSHA("1234567890abcdef1234567890abcdef")).toBe("1234567890ab…cdef");
    expect(shortConfigVersionSHA("abc")).toBe("abc");
  });

  it("reports why rollback is unavailable", () => {
    expect(configVersionRollbackReason({ role: "operator", readOnly: false, isCurrent: false })).toContain("管理员");
    expect(configVersionRollbackReason({ role: "admin", readOnly: true, isCurrent: false })).toContain("只读");
    expect(configVersionRollbackReason({ role: "admin", readOnly: false, isCurrent: true })).toContain("当前");
    expect(configVersionRollbackReason({ role: "admin", readOnly: false, isCurrent: false })).toBe("");
  });

  it("builds safe XML download filenames", () => {
    expect(configVersionFilename("demo db/01", 4)).toBe("demo_db_01-config-v4.xml");
    expect(configVersionFilename("../../", 2)).toBe("vm-config-v2.xml");
  });
});
