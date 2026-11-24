import { describe, expect, it } from "vitest";
import { analyzeConsoleTextInput, CONSOLE_INPUT_MAX_LENGTH, describeConsoleInputCharacter } from "@/features/console/console-input";

describe("console text input", () => {
  it("maps printable ASCII to US keyboard keystrokes", () => {
    const analysis = analyzeConsoleTextInput("Ab 1!?");
    expect(analysis.unsupported).toEqual([]);
    expect(analysis.keystrokes).toEqual([
      { keysym: 65, code: "KeyA", shift: true },
      { keysym: 98, code: "KeyB", shift: false },
      { keysym: 32, code: "Space", shift: false },
      { keysym: 49, code: "Digit1", shift: false },
      { keysym: 33, code: "Digit1", shift: true },
      { keysym: 63, code: "Slash", shift: true },
    ]);
  });

  it("normalizes line endings and maps terminal control keys", () => {
    const analysis = analyzeConsoleTextInput("one\r\ntwo\rthree\tfour\b");
    expect(analysis.normalized).toBe("one\ntwo\nthree\tfour\b");
    expect(analysis.keystrokes.filter((key) => key.code === "Enter")).toHaveLength(2);
    expect(analysis.keystrokes.at(-1)).toEqual({ keysym: 0xff08, code: "Backspace", shift: false });
  });

  it("reports characters that cannot be typed with a VNC keyboard", () => {
    const analysis = analyzeConsoleTextInput("echo 你好");
    expect(analysis.unsupported).toEqual(["你", "好"]);
    expect(analysis.keystrokes.map((key) => key.code)).not.toContain("Unidentified");
  });

  it("provides an input limit and readable unsupported character labels", () => {
    expect(CONSOLE_INPUT_MAX_LENGTH).toBe(8192);
    expect(describeConsoleInputCharacter("你")).toBe("你 (U+4F60)");
    expect(describeConsoleInputCharacter("\n")).toBe("U+000A");
  });
});
