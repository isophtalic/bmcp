// Tracks the tab the user connected and provides navigation/load helpers.
import { NO_CONNECTED_TAB, STORAGE_TAB_ID } from "../shared/protocol";

/** Returns the connected tab id, or throws the error the Go bridge reemits. */
export async function requireTabId(): Promise<number> {
  const stored = await chrome.storage.local.get(STORAGE_TAB_ID);
  const id = Number(stored[STORAGE_TAB_ID]);
  if (!Number.isInteger(id)) throw new Error(NO_CONNECTED_TAB);
  try {
    await chrome.tabs.get(id); // Confirms the tab still exists.
  } catch {
    throw new Error("No tab with given id " + id);
  }
  return id;
}

export async function setConnectedTab(tabId: number): Promise<void> {
  await chrome.storage.local.set({ [STORAGE_TAB_ID]: tabId });
}

export async function getUrl(): Promise<string> {
  const tab = await chrome.tabs.get(await requireTabId());
  return tab.url ?? "";
}

export async function getTitle(): Promise<string> {
  const tab = await chrome.tabs.get(await requireTabId());
  return tab.title ?? "";
}

/** Navigates the connected tab and resolves once it finishes loading. */
export async function navigate(url: string): Promise<void> {
  const tabId = await requireTabId();
  await chrome.tabs.update(tabId, { url });
  await waitForComplete(tabId);
}

export async function goBack(): Promise<void> {
  const tabId = await requireTabId();
  await chrome.tabs.goBack(tabId);
  await waitForComplete(tabId);
}

export async function goForward(): Promise<void> {
  const tabId = await requireTabId();
  await chrome.tabs.goForward(tabId);
  await waitForComplete(tabId);
}

/** Resolves when the tab reaches "complete", or after a timeout. */
export function waitForComplete(tabId: number, timeoutMs = 15000): Promise<void> {
  return new Promise((resolve) => {
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      chrome.tabs.onUpdated.removeListener(listener);
      clearTimeout(timer);
      resolve();
    };
    const listener = (id: number, info: chrome.tabs.TabChangeInfo) => {
      if (id === tabId && info.status === "complete") finish();
    };
    chrome.tabs.onUpdated.addListener(listener);
    const timer = setTimeout(finish, timeoutMs);
    // The tab may already be complete (e.g. same-document navigation).
    chrome.tabs.get(tabId).then((t) => {
      if (t.status === "complete") finish();
    }).catch(finish);
  });
}

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
