import { describe, expect, it } from "vitest";
import { formatBytes, percent } from "@/lib/utils";

describe("formatBytes", () => {
  it("formats common units", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1024)).toBe("1.0 KiB");
    expect(formatBytes(8 * 1024 ** 3)).toBe("8.0 GiB");
  });
});

describe("percent", () => {
  it("clamps invalid and oversized values", () => {
    expect(percent(0, 0)).toBe(0);
    expect(percent(2, 4)).toBe(50);
    expect(percent(9, 4)).toBe(100);
  });
});
