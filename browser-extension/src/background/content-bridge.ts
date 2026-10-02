// Sends an action to the content script running in the connected tab, injecting
// it first if the page was open before the extension loaded.
import type { ContentRequest, ContentResponse } from "../shared/protocol";
import { requireTabId } from "./tabs";

export async function callContent<T = unknown>(
  req: ContentRequest,
): Promise<T> {
  const tabId = await requireTabId();
  let resp: ContentResponse<T>;
  try {
    resp = await chrome.tabs.sendMessage<ContentRequest, ContentResponse<T>>(
      tabId,
      req,
    );
  } catch {
    await inject(tabId);
    resp = await chrome.tabs.sendMessage<ContentRequest, ContentResponse<T>>(
      tabId,
      req,
    );
  }
  if (!resp || !resp.ok) {
    throw new Error(resp?.error ?? "Content script error");
  }
  return resp.result as T;
}

async function inject(tabId: number): Promise<void> {
  await chrome.scripting.executeScript({
    target: { tabId, allFrames: false },
    files: ["content.js"],
  });
}
