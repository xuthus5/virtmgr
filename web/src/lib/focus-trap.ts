const focusableSelector = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled]):not([type='hidden'])",
  "select:not([disabled])",
  "summary",
  "textarea:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(focusableSelector)).filter((element) =>
    !element.closest("[hidden], [inert], [aria-hidden='true']") && element.getClientRects().length > 0,
  );
}

export function trapTabFocus(event: KeyboardEvent, container: HTMLElement | null): void {
  if (event.key !== "Tab" || !container) return;
  const elements = focusableElements(container);
  if (elements.length === 0) {
    event.preventDefault();
    return;
  }

  const currentIndex = elements.indexOf(document.activeElement as HTMLElement);
  if (currentIndex === -1) {
    event.preventDefault();
    elements[event.shiftKey ? elements.length - 1 : 0].focus();
    return;
  }
  if (event.shiftKey && currentIndex === 0) {
    event.preventDefault();
    elements[elements.length - 1].focus();
    return;
  }
  if (!event.shiftKey && currentIndex === elements.length - 1) {
    event.preventDefault();
    elements[0].focus();
  }
}
