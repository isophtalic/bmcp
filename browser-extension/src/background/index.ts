// Service worker entry point: wires the WebSocket client to the per-type
// handlers and manages connect/disconnect requests from the popup.
import {
  type DragPayload,
  type ElementRefPayload,
  type EvaluatePayload,
  type NavigatePayload,
  type PressKeyPayload,
  type SelectOptionPayload,
  type ServerMessage,
  type TypePayload,
  type UploadFilePayload,
  type WaitPayload,
} from "../shared/protocol";
import { callContent } from "./content-bridge";
import { evaluate, screenshot, uploadFile } from "./page";
import {
  getTitle,
  getUrl,
  goBack,
  goForward,
  navigate,
  setConnectedTab,
  sleep,
} from "./tabs";
import { connect, disconnect, ensureConnected, isConnected, onServerMessage } from "./ws";

onServerMessage((msg) => dispatch(msg));

async function dispatch(msg: ServerMessage): Promise<unknown> {
  const p = (msg.payload ?? {}) as Record<string, unknown>;
  switch (msg.type) {
    case "getUrl":
      return getUrl();
    case "getTitle":
      return getTitle();
    case "browser_navigate":
      await navigate((p as unknown as NavigatePayload).url);
      return null;
    case "browser_go_back":
      await goBack();
      return null;
    case "browser_go_forward":
      await goForward();
      return null;
    case "browser_wait":
      await sleep(Math.max(0, Number((p as unknown as WaitPayload).time) || 0) * 1000);
      return null;
    case "browser_snapshot":
      return callContent<string>({ action: "snapshot" });
    case "browser_click":
      await callContent({ action: "click", ref: (p as unknown as ElementRefPayload).ref });
      return null;
    case "browser_hover":
      await callContent({ action: "hover", ref: (p as unknown as ElementRefPayload).ref });
      return null;
    case "browser_drag": {
      const d = p as unknown as DragPayload;
      await callContent({ action: "drag", startRef: d.startRef, endRef: d.endRef });
      return null;
    }
    case "browser_type": {
      const t = p as unknown as TypePayload;
      await callContent({ action: "type", ref: t.ref, text: t.text, submit: t.submit });
      return null;
    }
    case "browser_select_option": {
      const s = p as unknown as SelectOptionPayload;
      await callContent({ action: "selectOption", ref: s.ref, values: s.values });
      return null;
    }
    case "browser_press_key":
      await callContent({ action: "pressKey", key: (p as unknown as PressKeyPayload).key });
      return null;
    case "browser_get_console_logs":
      return callContent<unknown[]>({ action: "getConsoleLogs" });
    case "browser_screenshot":
      return screenshot();
    case "browser_evaluate":
      return evaluate((p as unknown as EvaluatePayload).expression);
    case "browser_upload_file": {
      const u = p as unknown as UploadFilePayload;
      await uploadFile(u.selector, u.filePath);
      return null;
    }
    default:
      throw new Error("Unknown message type: " + msg.type);
  }
}

// --- Popup / lifecycle plumbing ---

interface PopupRequest {
  cmd: "connect" | "disconnect" | "status";
  tabId?: number;
}

chrome.runtime.onMessage.addListener((req: PopupRequest, _sender, sendResponse) => {
  (async () => {
    switch (req.cmd) {
      case "connect": {
        const tabId = req.tabId ?? (await activeTabId());
        if (tabId !== undefined) await setConnectedTab(tabId);
        await connect();
        break;
      }
      case "disconnect":
        await disconnect();
        break;
      case "status":
        break;
    }
    sendResponse({ connected: isConnected() });
  })();
  return true; // Keep the message channel open for the async response.
});

async function activeTabId(): Promise<number | undefined> {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab?.id;
}

// Keep the socket alive across service-worker restarts.
const KEEPALIVE_ALARM = "bmcp-keepalive";
chrome.alarms.create(KEEPALIVE_ALARM, { periodInMinutes: 0.4 });
chrome.alarms.onAlarm.addListener((a) => {
  if (a.name === KEEPALIVE_ALARM) void ensureConnected();
});
chrome.runtime.onStartup.addListener(() => void ensureConnected());
chrome.runtime.onInstalled.addListener(() => void ensureConnected());
void ensureConnected();
