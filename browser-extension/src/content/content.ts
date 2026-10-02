// Content script (isolated world): builds the accessibility snapshot the model
// reads, resolves the refs it hands back, and performs DOM interactions. One
// instance per tab persists the ref -> element map between a snapshot and the
// actions that follow it.
import type { ContentRequest, ContentResponse } from "../shared/protocol";

const refToElement = new Map<string, Element>();
let refCounter = 0;
const consoleLogs: string[] = [];
const MAX_LOGS = 500;

// Receive console entries forwarded by the main-world hook.
window.addEventListener("message", (e: MessageEvent) => {
  const d = e.data;
  if (e.source === window && d && d.__bmcpConsole) {
    consoleLogs.push(String(d.__bmcpConsole));
    if (consoleLogs.length > MAX_LOGS) consoleLogs.shift();
  }
});

chrome.runtime.onMessage.addListener(
  (req: ContentRequest, _sender, sendResponse: (r: ContentResponse) => void) => {
    try {
      sendResponse({ ok: true, result: handle(req) });
    } catch (err) {
      sendResponse({ ok: false, error: err instanceof Error ? err.message : String(err) });
    }
    return false; // Synchronous response.
  },
);

function handle(req: ContentRequest): unknown {
  switch (req.action) {
    case "snapshot":
      return snapshot();
    case "click":
      click(resolve(req.ref));
      return null;
    case "hover":
      hover(resolve(req.ref));
      return null;
    case "drag":
      drag(resolve(req.startRef), resolve(req.endRef));
      return null;
    case "type":
      typeInto(resolve(req.ref), req.text, req.submit);
      return null;
    case "selectOption":
      selectOption(resolve(req.ref), req.values);
      return null;
    case "pressKey":
      pressKey(req.key);
      return null;
    case "getConsoleLogs":
      return consoleLogs.slice();
  }
}

function resolve(ref: string): Element {
  const el = refToElement.get(ref);
  if (!el || !el.isConnected) {
    throw new Error(`Ref ${ref} is stale. Take a new browser_snapshot first.`);
  }
  return el;
}

// --- Accessibility snapshot ---------------------------------------------------

function snapshot(): string {
  refToElement.clear();
  refCounter = 0;
  const lines: string[] = [];
  const body = document.body;
  if (body) walk(body, 0, lines);
  return lines.join("\n");
}

function walk(node: Element, depth: number, out: string[]): void {
  for (const child of Array.from(node.children)) {
    if (!isVisible(child)) continue;
    const role = roleOf(child);
    const name = nameOf(child);
    const interesting = role !== null || name.length > 0;

    if (interesting) {
      const ref = "e" + ++refCounter;
      refToElement.set(ref, child);
      const label = role ?? "generic";
      const namePart = name ? ` ${JSON.stringify(trim(name))}` : "";
      out.push(`${"  ".repeat(depth)}- ${label}${namePart} [ref=${ref}]`);
      walk(child, depth + 1, out);
    } else {
      // Transparent container: keep its children at the same depth.
      walk(child, depth, out);
    }
  }
}

const ROLE_BY_TAG: Record<string, string> = {
  A: "link",
  BUTTON: "button",
  SELECT: "combobox",
  TEXTAREA: "textbox",
  H1: "heading",
  H2: "heading",
  H3: "heading",
  H4: "heading",
  H5: "heading",
  H6: "heading",
  IMG: "img",
  NAV: "navigation",
  MAIN: "main",
  HEADER: "banner",
  FOOTER: "contentinfo",
  UL: "list",
  OL: "list",
  LI: "listitem",
  TABLE: "table",
  FORM: "form",
  LABEL: "label",
};

function roleOf(el: Element): string | null {
  const explicit = el.getAttribute("role");
  if (explicit) return explicit;
  const tag = el.tagName;
  if (tag === "INPUT") {
    const type = (el as HTMLInputElement).type;
    switch (type) {
      case "checkbox":
        return "checkbox";
      case "radio":
        return "radio";
      case "button":
      case "submit":
      case "reset":
        return "button";
      case "range":
        return "slider";
      case "hidden":
        return null;
      default:
        return "textbox";
    }
  }
  return ROLE_BY_TAG[tag] ?? null;
}

function nameOf(el: Element): string {
  const aria = el.getAttribute("aria-label");
  if (aria) return aria;
  const labelledby = el.getAttribute("aria-labelledby");
  if (labelledby) {
    const parts = labelledby
      .split(/\s+/)
      .map((id) => document.getElementById(id)?.textContent ?? "")
      .join(" ");
    if (parts.trim()) return parts;
  }
  if (el.tagName === "IMG") return (el as HTMLImageElement).alt || "";
  if (el.tagName === "INPUT") {
    const input = el as HTMLInputElement;
    return input.placeholder || input.value || labelFor(input) || "";
  }
  if (el.tagName === "TEXTAREA") {
    const ta = el as HTMLTextAreaElement;
    return ta.placeholder || ta.value || labelFor(ta) || "";
  }
  // Leaf elements: use their own text.
  if (el.children.length === 0) return el.textContent ?? "";
  // Containers: only a short direct-text label if any.
  const direct = directText(el);
  return direct;
}

function labelFor(el: HTMLElement): string {
  const id = el.id;
  if (id) {
    const lbl = document.querySelector(`label[for="${CSS.escape(id)}"]`);
    if (lbl?.textContent) return lbl.textContent;
  }
  const parentLabel = el.closest("label");
  return parentLabel?.textContent ?? "";
}

function directText(el: Element): string {
  let text = "";
  for (const n of Array.from(el.childNodes)) {
    if (n.nodeType === Node.TEXT_NODE) text += n.textContent ?? "";
  }
  return text.trim();
}

function trim(s: string): string {
  const clean = s.replace(/\s+/g, " ").trim();
  return clean.length > 120 ? clean.slice(0, 117) + "…" : clean;
}

function isVisible(el: Element): boolean {
  const style = getComputedStyle(el);
  if (style.display === "none" || style.visibility === "hidden" || style.opacity === "0") {
    return false;
  }
  const rect = el.getBoundingClientRect();
  if (rect.width === 0 && rect.height === 0 && el.children.length === 0) return false;
  return true;
}

// --- Interactions -------------------------------------------------------------

function click(el: Element): void {
  (el as HTMLElement).scrollIntoView({ block: "center", inline: "center" });
  fireMouse(el, "mouseover");
  fireMouse(el, "mousedown");
  fireMouse(el, "mouseup");
  (el as HTMLElement).click();
}

function hover(el: Element): void {
  (el as HTMLElement).scrollIntoView({ block: "center", inline: "center" });
  fireMouse(el, "mouseover");
  fireMouse(el, "mousemove");
}

function typeInto(el: Element, text: string, submit?: boolean): void {
  (el as HTMLElement).focus();
  if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
    setValue(el, text);
  } else if ((el as HTMLElement).isContentEditable) {
    el.textContent = text;
    el.dispatchEvent(new InputEvent("input", { bubbles: true }));
  }
  if (submit) {
    el.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    el.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
    const form = (el as HTMLElement).closest("form");
    if (form) form.requestSubmit();
  }
}

function setValue(el: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  const proto = el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLTextAreaElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  setter?.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
}

function selectOption(el: Element, values: string[]): void {
  if (!(el instanceof HTMLSelectElement)) {
    throw new Error("Element is not a <select>");
  }
  const wanted = new Set(values);
  for (const opt of Array.from(el.options)) {
    opt.selected = wanted.has(opt.value) || wanted.has(opt.textContent?.trim() ?? "");
  }
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
}

function pressKey(key: string): void {
  const target: EventTarget = document.activeElement ?? document.body;
  target.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
  target.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true }));
}

function drag(start: Element, end: Element): void {
  const dt = new DataTransfer();
  fireDrag(start, "dragstart", dt);
  fireDrag(end, "dragover", dt);
  fireDrag(end, "drop", dt);
  fireDrag(start, "dragend", dt);
}

function fireMouse(el: Element, type: string): void {
  el.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, view: window }));
}

function fireDrag(el: Element, type: string, dataTransfer: DataTransfer): void {
  const event = new DragEvent(type, { bubbles: true, cancelable: true });
  Object.defineProperty(event, "dataTransfer", { value: dataTransfer });
  el.dispatchEvent(event);
}
