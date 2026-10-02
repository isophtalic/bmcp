// Runs in the page's main world (manifest content_scripts "world": "MAIN").
// Wraps the console so entries can be read back via browser_get_console_logs,
// forwarding each one to the isolated content script through window.postMessage.
(() => {
  const methods = ["log", "info", "warn", "error", "debug"] as const;

  for (const name of methods) {
    const original = console[name].bind(console) as (...a: unknown[]) => void;
    console[name] = (...args: unknown[]) => {
      try {
        const text = `[${name}] ${args.map(stringify).join(" ")}`;
        window.postMessage({ __bmcpConsole: text }, "*");
      } catch {
        // Never let logging break the page.
      }
      original(...args);
    };
  }

  window.addEventListener("error", (e) => {
    window.postMessage({ __bmcpConsole: `[error] ${e.message}` }, "*");
  });
  window.addEventListener("unhandledrejection", (e) => {
    window.postMessage({ __bmcpConsole: `[error] Unhandled rejection: ${String(e.reason)}` }, "*");
  });

  function stringify(v: unknown): string {
    if (typeof v === "string") return v;
    try {
      return JSON.stringify(v);
    } catch {
      return String(v);
    }
  }
})();
