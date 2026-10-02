// Bundles the extension's TypeScript sources into dist/ with esbuild and copies
// the static files (manifest, icons, popup.html) alongside them. Run `npm run
// build`, then load dist/ as an unpacked extension in chrome://extensions.
import * as esbuild from "esbuild";
import { cp, mkdir, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const outdir = resolve(root, "dist");
const watch = process.argv.includes("--watch");

/** @type {esbuild.BuildOptions} */
const common = {
  bundle: true,
  format: "esm",
  target: "chrome114",
  logLevel: "info",
  sourcemap: watch ? "inline" : false,
  minify: !watch,
};

// Each entry becomes one self-contained file. The background service worker is
// a module; content scripts and the popup are bundled the same way (Chrome
// loads a module content script via the manifest's "type": "module").
const entries = {
  background: resolve(root, "src/background/index.ts"),
  content: resolve(root, "src/content/content.ts"),
  "console-hook": resolve(root, "src/content/console-hook.ts"),
  popup: resolve(root, "src/popup/popup.ts"),
};

async function copyStatic() {
  await cp(resolve(root, "public"), outdir, { recursive: true });
}

async function run() {
  await rm(outdir, { recursive: true, force: true });
  await mkdir(outdir, { recursive: true });
  await copyStatic();

  const opts = { ...common, entryPoints: entries, outdir };
  if (watch) {
    const ctx = await esbuild.context(opts);
    await ctx.watch();
    console.log("watching for changes…");
  } else {
    await esbuild.build(opts);
    console.log("built dist/");
  }
}

if (!existsSync(resolve(root, "public/manifest.json"))) {
  console.error("public/manifest.json is missing");
  process.exit(1);
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
