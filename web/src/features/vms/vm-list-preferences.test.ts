import { describe, expect, it } from "vitest";
import { defaultVMListFilters, parseVMFavorites, parseVMListFilters, serializeVMListFilters, toggleVMFavorite } from "@/features/vms/vm-list-preferences";

describe("vm list preferences", () => {
  it("parses persisted filters and falls back for malformed values", () => {
    expect(parseVMListFilters(serializeVMListFilters({ state: "running", tag: "prod", owner: "ops" }))).toEqual({ state: "running", tag: "prod", owner: "ops" });
    expect(parseVMListFilters("not-json")).toEqual(defaultVMListFilters);
    expect(parseVMListFilters(JSON.stringify({ state: "invalid", tag: 42, owner: "admin" }))).toEqual({ state: "", tag: "", owner: "admin" });
  });

  it("deduplicates favorites and toggles them without losing order", () => {
    const parsed = parseVMFavorites(JSON.stringify(["vm-a", "vm-b", "vm-a", 42, ""]));
    expect(parsed).toEqual(["vm-a", "vm-b"]);
    expect(toggleVMFavorite(parsed, "vm-c")).toEqual(["vm-c", "vm-a", "vm-b"]);
    expect(toggleVMFavorite(parsed, "vm-a")).toEqual(["vm-b"]);
  });

  it("limits favorites and rejects invalid JSON", () => {
    expect(parseVMFavorites("{bad")).toEqual([]);
    expect(parseVMFavorites(JSON.stringify(Array.from({ length: 250 }, (_, index) => `vm-${index}`)))).toHaveLength(200);
  });
});
