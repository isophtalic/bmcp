// Wire protocol shared with the Go MCP server (internal/bridge/hub.go).
//
//   server -> extension: { id, type, payload? }
//   extension -> server: { id, type: "messageResponse",
//                          payload: { requestId, result, error } }
//
// The server matches responses by payload.requestId === incoming id.

/** Default loopback port the server listens on (config.DefaultWsPort). */
export const DEFAULT_WS_PORT = 9009;

/** storage.local key holding a port override (set when the server uses --port). */
export const STORAGE_WS_PORT = "wsPort";
/** storage.local key holding the tab the user connected. */
export const STORAGE_TAB_ID = "selectedTabId";
/** storage.local key: whether the user asked to be connected. */
export const STORAGE_CONNECTED = "connected";

export const MESSAGE_RESPONSE = "messageResponse";

/** A request from the server. `payload` shape depends on `type`. */
export interface ServerMessage {
  id: string;
  type: string;
  payload?: unknown;
}

/** The body the server reads back (one of result/error is meaningful). */
export interface MessageResponsePayload {
  requestId: string;
  result?: unknown;
  error?: string;
}

/** Error the Go bridge recognises and rewords for the model. */
export const NO_CONNECTED_TAB = "No tab is connected";

// --- Payload shapes per message type (kept in sync with internal/tools). ---

export interface NavigatePayload {
  url: string;
}
export interface ElementRefPayload {
  element?: string;
  ref: string;
}
export interface DragPayload {
  startElement?: string;
  startRef: string;
  endElement?: string;
  endRef: string;
}
export interface TypePayload extends ElementRefPayload {
  text: string;
  submit?: boolean;
}
export interface SelectOptionPayload extends ElementRefPayload {
  values: string[];
}
export interface PressKeyPayload {
  key: string;
}
export interface WaitPayload {
  time: number;
}
export interface UploadFilePayload {
  selector: string;
  filePath: string;
}
export interface EvaluatePayload {
  expression: string;
}

// --- Messages exchanged between background and the content script. ---

export type ContentRequest =
  | { action: "snapshot" }
  | { action: "click"; ref: string }
  | { action: "hover"; ref: string }
  | { action: "drag"; startRef: string; endRef: string }
  | { action: "type"; ref: string; text: string; submit?: boolean }
  | { action: "selectOption"; ref: string; values: string[] }
  | { action: "pressKey"; key: string }
  | { action: "getConsoleLogs" };

export interface ContentResponse<T = unknown> {
  ok: boolean;
  result?: T;
  error?: string;
}
