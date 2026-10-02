// Popup: a Connect/Disconnect button that attaches the current tab to the MCP
// server and shows the live connection state.
import { STORAGE_CONNECTED } from "../shared/protocol";

const button = document.getElementById("toggle") as HTMLButtonElement;
const status = document.getElementById("status") as HTMLParagraphElement;

async function refresh(): Promise<void> {
  const stored = await chrome.storage.local.get(STORAGE_CONNECTED);
  const resp = await chrome.runtime.sendMessage({ cmd: "status" }).catch(() => undefined);
  const live = resp?.connected === true;
  const wanted = stored[STORAGE_CONNECTED] === true;
  render(wanted, live);
}

function render(wanted: boolean, live: boolean): void {
  if (live) {
    status.textContent = "Connected";
    status.className = "ok";
  } else if (wanted) {
    status.textContent = "Connecting…";
    status.className = "pending";
  } else {
    status.textContent = "Not connected";
    status.className = "off";
  }
  button.textContent = wanted ? "Disconnect" : "Connect";
}

button.addEventListener("click", async () => {
  const stored = await chrome.storage.local.get(STORAGE_CONNECTED);
  const wanted = stored[STORAGE_CONNECTED] === true;
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  await chrome.runtime.sendMessage(
    wanted ? { cmd: "disconnect" } : { cmd: "connect", tabId: tab?.id },
  );
  setTimeout(refresh, 300);
});

void refresh();
setInterval(refresh, 1000);
