// Page-level operations that need extension APIs rather than the content
// script: screenshots, JavaScript evaluation in the page's main world, and
// file-input uploads via the Chrome DevTools Protocol.
import { requireTabId } from "./tabs";

/** Captures the connected tab and returns a bare base64 PNG (no data: prefix). */
export async function screenshot(): Promise<string> {
  const tabId = await requireTabId();
  const tab = await chrome.tabs.get(tabId);
  await chrome.tabs.update(tabId, { active: true });
  const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId ?? chrome.windows.WINDOW_ID_CURRENT, {
    format: "png",
  });
  return dataUrl.replace(/^data:image\/png;base64,/, "");
}

/**
 * Evaluates an expression in the page's main world, awaiting a promise result.
 * Returns the value (structured-cloned); unserialisable values come back as a
 * string description.
 */
export async function evaluate(expression: string): Promise<unknown> {
  const tabId = await requireTabId();
  const [res] = await chrome.scripting.executeScript({
    target: { tabId },
    world: "MAIN",
    args: [expression],
    func: async (expr: string) => {
      // eslint-disable-next-line no-eval
      const value = await (0, eval)(expr);
      try {
        // Force a structured-clone-safe value; throws for DOM nodes etc.
        return JSON.parse(JSON.stringify(value ?? null));
      } catch {
        return String(value);
      }
    },
  });
  return res?.result ?? "undefined";
}

/** Attaches a file at an absolute path to the input matched by `selector`. */
export async function uploadFile(selector: string, filePath: string): Promise<void> {
  const tabId = await requireTabId();
  const target: chrome.debugger.Debuggee = { tabId };
  await debuggerAttach(target);
  try {
    await send(target, "DOM.enable");
    const doc = (await send(target, "DOM.getDocument", { depth: -1 })) as {
      root: { nodeId: number };
    };
    const found = (await send(target, "DOM.querySelector", {
      nodeId: doc.root.nodeId,
      selector,
    })) as { nodeId: number };
    if (!found.nodeId) throw new Error("File input not found: " + selector);
    await send(target, "DOM.setFileInputFiles", {
      files: [filePath],
      nodeId: found.nodeId,
    });
  } finally {
    await debuggerDetach(target);
  }
}

const DEBUGGER_PROTOCOL = "1.3";

function debuggerAttach(target: chrome.debugger.Debuggee): Promise<void> {
  return new Promise((resolve, reject) => {
    chrome.debugger.attach(target, DEBUGGER_PROTOCOL, () => {
      const err = chrome.runtime.lastError;
      if (err) reject(new Error(err.message));
      else resolve();
    });
  });
}

function debuggerDetach(target: chrome.debugger.Debuggee): Promise<void> {
  return new Promise((resolve) => {
    chrome.debugger.detach(target, () => {
      void chrome.runtime.lastError; // Ignore: detach is best-effort.
      resolve();
    });
  });
}

function send(
  target: chrome.debugger.Debuggee,
  method: string,
  params?: Record<string, unknown>,
): Promise<unknown> {
  return new Promise((resolve, reject) => {
    chrome.debugger.sendCommand(target, method, params ?? {}, (result) => {
      const err = chrome.runtime.lastError;
      if (err) reject(new Error(err.message));
      else resolve(result);
    });
  });
}
