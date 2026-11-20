// End-to-end check of the $EDITOR contract against the real binary.
//
// A Tauri window cannot be screenshotted and has no widget tree to click, so the
// only programmable surface is WebView2's remote debugging port: this script
// starts nib with it, drives the page over CDP, and then asserts the two things
// a caller actually judges — the process exit code and the file bytes.
//
//   node scripts/e2e.mjs --file <doc.md> --phase clean|edit|discard
//
// The fixture is copied to `<doc>.e2e` and removed afterwards; the source file
// is never touched.

import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFile, readFile, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, "..");
const binary = join(repo, "nib.exe");
const PORT = 9222;
const EDIT = "e2e edit";

const args = process.argv.slice(2);
function option(name, fallback) {
  const at = args.indexOf(`--${name}`);
  return at === -1 ? fallback : args[at + 1];
}

const source = option("file", join(repo, ".tmp", "sample.md"));
const phase = option("phase", "edit");
if (!existsSync(source)) throw new Error(`no such fixture: ${source}`);
if (!existsSync(binary)) throw new Error(`no binary at ${binary}; run "npm run tauri -- build --no-bundle" first`);

const fixture = `${source}.e2e`;
const digest = async (path) => createHash("md5").update(await readFile(path)).digest("hex");
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

await copyFile(source, fixture);
const before = await digest(fixture);

const nib = spawn(binary, [fixture], {
  env: { ...process.env, WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS: `--remote-debugging-port=${PORT}` },
  stdio: "ignore",
});
const exited = new Promise((done) => nib.on("exit", (code) => done(code ?? -1)));

async function debuggerTarget() {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const targets = await (await fetch(`http://127.0.0.1:${PORT}/json`)).json();
      const page = targets.find((t) => t.type === "page");
      if (page) return page;
    } catch {
      // not listening yet
    }
    await sleep(100);
  }
  throw new Error("WebView2 never opened its debugging port");
}

const target = await debuggerTarget();
const socket = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((open, fail) => {
  socket.onopen = open;
  socket.onerror = fail;
});

let sequence = 0;
const pending = new Map();
socket.onmessage = (event) => {
  const message = JSON.parse(event.data);
  const waiter = pending.get(message.id);
  if (!waiter) return;
  pending.delete(message.id);
  message.error ? waiter.fail(new Error(message.error.message)) : waiter.open(message.result);
};
const send = (method, params = {}) =>
  new Promise((open, fail) => {
    const id = ++sequence;
    pending.set(id, { open, fail });
    socket.send(JSON.stringify({ id, method, params }));
  });

const evaluate = async (expression) =>
  (await send("Runtime.evaluate", { expression, returnByValue: true })).result.value;

// The debugging port opens before React has mounted, and a shortcut pressed
// into a page with no listeners yet is silently lost.
async function waitForPage() {
  for (let attempt = 0; attempt < 150; attempt++) {
    const ready = await evaluate(`!!document.querySelector(".ProseMirror")`).catch(() => false);
    if (ready) return;
    await sleep(100);
  }
  throw new Error("the editor never mounted");
}

async function pressKey(key, code, keyCode) {
  const event = { key, code, windowsVirtualKeyCode: keyCode, nativeVirtualKeyCode: keyCode };
  await send("Input.dispatchKeyEvent", { type: "keyDown", modifiers: 2, ...event });
  await send("Input.dispatchKeyEvent", { type: "keyUp", modifiers: 2, ...event });
}

// Click a real paragraph and type there. A caret forced in with a DOM Range sits
// outside ProseMirror's selection model, and the Enter that follows lands as a
// hard break — which serialises into the file as `<br />`.
async function typeIntoFirstParagraph(text) {
  const box = await evaluate(`(() => {
    const p = document.querySelector(".ProseMirror p");
    p.scrollIntoView({ block: "center" });
    const r = p.getBoundingClientRect();
    return { x: r.x + Math.min(120, r.width / 2), y: r.y + r.height / 2 };
  })()`);
  for (const type of ["mousePressed", "mouseReleased"]) {
    await send("Input.dispatchMouseEvent", { type, x: box.x, y: box.y, button: "left", clickCount: 1 });
  }
  await sleep(300);
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Enter", code: "Enter", windowsVirtualKeyCode: 13, nativeVirtualKeyCode: 13 });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Enter", code: "Enter", windowsVirtualKeyCode: 13, nativeVirtualKeyCode: 13 });
  await sleep(200);
  await send("Input.insertText", { text });
  await sleep(400);
}

async function confirmDiscard() {
  await pressKey("q", "KeyQ", 81);
  await sleep(700);
  const labels = await evaluate(
    `Array.from(document.querySelectorAll(".nib-confirm button")).map((b) => b.textContent)`,
  );
  if (!Array.isArray(labels) || labels.length === 0) throw new Error("no confirmation strip appeared");
  await evaluate(`Array.from(document.querySelectorAll(".nib-confirm button")).find((b) => b.textContent === "Discard").click()`);
  await sleep(800);
}

async function run() {
  await waitForPage();
  if (phase === "clean") {
    await pressKey("Enter", "Enter", 13);
  } else {
    await typeIntoFirstParagraph(EDIT);
    if (phase === "edit") await pressKey("Enter", "Enter", 13);
    else if (phase === "discard") await confirmDiscard();
    else throw new Error(`unknown phase: ${phase}`);
  }
}

await run();
const code = await exited;
socket.close();

const after = await digest(fixture);
const text = await readFile(fixture, "utf8");
const problems = [];
if (phase === "clean" && (code !== 0 || after !== before)) {
  problems.push(`clean exit should leave the file untouched: code=${code} changed=${after !== before}`);
}
if (phase === "edit" && (code !== 0 || !text.includes(EDIT))) {
  problems.push(`edit exit should accept the edit: code=${code} written=${text.includes(EDIT)}`);
}
if (phase === "edit" && text.includes("<br")) problems.push("the save wrote a hard break (<br />)");
if (phase === "discard" && (code !== 1 || after !== before)) {
  problems.push(`discard should leave the file byte-identical: code=${code} changed=${after !== before}`);
}

console.log(`phase=${phase} exit=${code} file_changed=${after !== before}`);
await rm(fixture, { force: true });
if (problems.length) {
  for (const problem of problems) console.error(`FAIL ${problem}`);
  process.exit(1);
}
console.log("PASS");
