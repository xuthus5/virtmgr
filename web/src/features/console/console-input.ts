export const CONSOLE_INPUT_MAX_LENGTH = 8192;

export const CONSOLE_SHIFT_KEYSYM = 0xffe1;

export type ConsoleKeystroke = {
  keysym: number;
  code: string;
  shift: boolean;
};

export type ConsoleTextInputAnalysis = {
  keystrokes: ConsoleKeystroke[];
  normalized: string;
  unsupported: string[];
};

const symbolKeys: Record<string, { code: string; shift: boolean }> = {
  " ": { code: "Space", shift: false },
  "!": { code: "Digit1", shift: true },
  '"': { code: "Quote", shift: true },
  "#": { code: "Digit3", shift: true },
  "$": { code: "Digit4", shift: true },
  "%": { code: "Digit5", shift: true },
  "&": { code: "Digit7", shift: true },
  "'": { code: "Quote", shift: false },
  "(": { code: "Digit9", shift: true },
  ")": { code: "Digit0", shift: true },
  "*": { code: "Digit8", shift: true },
  "+": { code: "Equal", shift: true },
  ",": { code: "Comma", shift: false },
  "-": { code: "Minus", shift: false },
  ".": { code: "Period", shift: false },
  "/": { code: "Slash", shift: false },
  ":": { code: "Semicolon", shift: true },
  ";": { code: "Semicolon", shift: false },
  "<": { code: "Comma", shift: true },
  "=": { code: "Equal", shift: false },
  ">": { code: "Period", shift: true },
  "?": { code: "Slash", shift: true },
  "@": { code: "Digit2", shift: true },
  "[": { code: "BracketLeft", shift: false },
  "\\": { code: "Backslash", shift: false },
  "]": { code: "BracketRight", shift: false },
  "^": { code: "Digit6", shift: true },
  "_": { code: "Minus", shift: true },
  "`": { code: "Backquote", shift: false },
  "{": { code: "BracketLeft", shift: true },
  "|": { code: "Backslash", shift: true },
  "}": { code: "BracketRight", shift: true },
  "~": { code: "Backquote", shift: true },
};

const controlKeys: Record<string, { keysym: number; code: string }> = {
  "\n": { keysym: 0xff0d, code: "Enter" },
  "\t": { keysym: 0xff09, code: "Tab" },
  "\b": { keysym: 0xff08, code: "Backspace" },
};

function letterKey(char: string): ConsoleKeystroke | null {
  const lower = char.toLowerCase();
  if (lower < "a" || lower > "z") return null;
  return {
    keysym: char.codePointAt(0) ?? 0,
    code: `Key${lower.toUpperCase()}`,
    shift: char !== lower,
  };
}

function digitKey(char: string): ConsoleKeystroke | null {
  if (char < "0" || char > "9") return null;
  return { keysym: char.codePointAt(0) ?? 0, code: `Digit${char}`, shift: false };
}

export function analyzeConsoleTextInput(value: string): ConsoleTextInputAnalysis {
  const normalized = value.replace(/\r\n?/g, "\n");
  const keystrokes: ConsoleKeystroke[] = [];
  const unsupported = new Set<string>();

  for (const char of normalized) {
    const control = controlKeys[char];
    if (control) {
      keystrokes.push({ ...control, shift: false });
      continue;
    }
    const letter = letterKey(char);
    if (letter) {
      keystrokes.push(letter);
      continue;
    }
    const digit = digitKey(char);
    if (digit) {
      keystrokes.push(digit);
      continue;
    }
    const symbol = symbolKeys[char];
    if (symbol) {
      keystrokes.push({ keysym: char.codePointAt(0) ?? 0, ...symbol });
      continue;
    }
    unsupported.add(char);
  }

  return { keystrokes, normalized, unsupported: [...unsupported] };
}

export function describeConsoleInputCharacter(char: string): string {
  const code = char.codePointAt(0) ?? 0;
  const hex = code.toString(16).toUpperCase().padStart(4, "0");
  if (char.trim() === "") return `U+${hex}`;
  return `${char} (U+${hex})`;
}
