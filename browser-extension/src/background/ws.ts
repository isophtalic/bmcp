// WebSocket client: connects to the Go server's loopback port, reconnects while
// the user wants to stay connected, and routes each incoming server message to
// the dispatcher. One connection at a time, mirroring the server's Hub.
import {
  DEFAULT_WS_PORT,
  MESSAGE_RESPONSE,
  STORAGE_CONNECTED,
  STORAGE_WS_PORT,
  type MessageResponsePayload,
  type ServerMessage,
} from "../shared/protocol";

type Handler = (msg: ServerMessage) => Promise<unknown>;

const RECONNECT_MS = 1000;

let ws: WebSocket | null = null;
let handler: Handler | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let wantConnected = false;

export function onServerMessage(h: Handler): void {
  handler = h;
}

async function resolvePort(): Promise<number> {
  const stored = await chrome.storage.local.get(STORAGE_WS_PORT);
  const port = Number(stored[STORAGE_WS_PORT]);
  return Number.isInteger(port) && port > 0 ? port : DEFAULT_WS_PORT;
}

/** Start (or keep) the connection; remembers the intent across SW restarts. */
export async function connect(): Promise<void> {
  wantConnected = true;
  await chrome.storage.local.set({ [STORAGE_CONNECTED]: true });
  openSocket();
}

/** Stop connecting and close any live socket. */
export async function disconnect(): Promise<void> {
  wantConnected = false;
  await chrome.storage.local.set({ [STORAGE_CONNECTED]: false });
  clearReconnect();
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }
}

export function isConnected(): boolean {
  return ws !== null && ws.readyState === WebSocket.OPEN;
}

/** Re-open the socket if the user wants to be connected but we are not. */
export async function ensureConnected(): Promise<void> {
  const stored = await chrome.storage.local.get(STORAGE_CONNECTED);
  if (stored[STORAGE_CONNECTED]) {
    wantConnected = true;
    if (!ws) openSocket();
  }
}

function clearReconnect(): void {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

function scheduleReconnect(): void {
  if (!wantConnected || reconnectTimer) return;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    openSocket();
  }, RECONNECT_MS);
}

async function openSocket(): Promise<void> {
  if (ws || !wantConnected) return;
  const port = await resolvePort();
  let socket: WebSocket;
  try {
    socket = new WebSocket(`ws://127.0.0.1:${port}`);
  } catch {
    scheduleReconnect();
    return;
  }
  ws = socket;

  socket.onopen = () => {
    void chrome.action.setBadgeText({ text: "on" });
    void chrome.action.setBadgeBackgroundColor({ color: "#16a34a" });
  };

  socket.onmessage = (event) => {
    void handleIncoming(event.data);
  };

  socket.onclose = () => {
    if (ws === socket) ws = null;
    void chrome.action.setBadgeText({ text: "" });
    scheduleReconnect();
  };

  socket.onerror = () => {
    // onclose fires next and handles the retry.
    socket.close();
  };
}

async function handleIncoming(data: unknown): Promise<void> {
  let msg: ServerMessage;
  try {
    msg = JSON.parse(String(data));
  } catch {
    return;
  }
  if (!msg || typeof msg.type !== "string" || typeof msg.id !== "string") return;
  if (!handler) return;

  const payload: MessageResponsePayload = { requestId: msg.id };
  try {
    payload.result = await handler(msg);
  } catch (err) {
    payload.error = err instanceof Error ? err.message : String(err);
  }
  send({ id: crypto.randomUUID(), type: MESSAGE_RESPONSE, payload });
}

function send(obj: unknown): void {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(obj));
  }
}
