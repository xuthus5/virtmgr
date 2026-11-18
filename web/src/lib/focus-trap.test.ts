import { afterEach, describe, expect, it } from "vitest";
import { trapTabFocus } from "@/lib/focus-trap";

function makeVisible(...elements: HTMLElement[]) {
  for (const element of elements) {
    Object.defineProperty(element, "getClientRects", { configurable: true, value: () => [{ width: 1 }] });
  }
}

function tabEvent(shiftKey = false) {
  return new KeyboardEvent("keydown", { key: "Tab", shiftKey, bubbles: true, cancelable: true });
}

afterEach(() => {
  document.body.replaceChildren();
});

describe("trapTabFocus", () => {
  it("wraps focus from the last element to the first", () => {
    document.body.innerHTML = '<div id="dialog"><button id="first">first</button><button id="last">last</button></div>';
    const dialog = document.getElementById("dialog")!;
    const first = document.getElementById("first") as HTMLButtonElement;
    const last = document.getElementById("last") as HTMLButtonElement;
    makeVisible(first, last);

    last.focus();
    const event = tabEvent();
    trapTabFocus(event, dialog);

    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(first);
  });

  it("wraps backwards and moves focus into the dialog when it escaped", () => {
    document.body.innerHTML = '<button id="outside">outside</button><div id="dialog"><button id="first">first</button><button id="last">last</button></div>';
    const dialog = document.getElementById("dialog")!;
    const outside = document.getElementById("outside") as HTMLButtonElement;
    const first = document.getElementById("first") as HTMLButtonElement;
    const last = document.getElementById("last") as HTMLButtonElement;
    makeVisible(first, last);

    first.focus();
    trapTabFocus(tabEvent(true), dialog);
    expect(document.activeElement).toBe(last);

    outside.focus();
    trapTabFocus(tabEvent(), dialog);
    expect(document.activeElement).toBe(first);
  });

  it("prevents tabbing when the dialog has no focusable controls", () => {
    const dialog = document.createElement("div");
    document.body.append(dialog);
    const event = tabEvent();
    trapTabFocus(event, dialog);
    expect(event.defaultPrevented).toBe(true);
  });
});
