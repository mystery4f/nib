import { useCallback, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Crepe } from "@milkdown/crepe";
import { commandsCtx } from "@milkdown/kit/core";
import { convertFileSrc, invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import "@milkdown/crepe/theme/common/style.css";
import "@milkdown/crepe/theme/classic.css";
import "./styles.css";

type Document = { name: string; text: string };

/// Images live in the system temp directory. The document keeps the plain path
/// so the markdown stays readable; only the view converts it for the webview.
function readBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result ?? "");
      resolve(result.slice(result.indexOf(",") + 1));
    };
    reader.onerror = () =>
      reject(reader.error ?? new Error("could not read the image"));
    reader.readAsDataURL(file);
  });
}

function extensionOf(file: File): string {
  const named = /\.([a-z0-9]+)$/i.exec(file.name)?.[1];
  if (named) return named.toLowerCase();
  const typed = file.type.replace(/^image\//, "").replace(/\+.*$/, "");
  return typed || "png";
}

async function storeImage(file: File): Promise<string> {
  const name = `image.${extensionOf(file)}`;
  const path = await invoke<string>("save_pasted_image", {
    name,
    data: await readBase64(file),
  });
  // A markdown link cannot carry a backslash; forward slashes work on Windows.
  return path.replace(/\\/g, "/");
}

function clipImageFiles(clipboard: DataTransfer | null): File[] {
  return Array.from(clipboard?.files ?? []).filter((file) =>
    file.type.startsWith("image/"),
  );
}

/// Anything the webview can load stays as it is; a local path becomes an asset
/// URL, which is what the temp directory is scoped for.
function displayUrl(url: string): string {
  if (/^(https?:|data:|blob:|asset:)/.test(url)) return url;
  return convertFileSrc(url);
}

/// Typora's editing keys that Milkdown does not bind itself. Ctrl+B, Ctrl+I and
/// Ctrl+E already work inside the editor, so they are deliberately absent.
function typoraCommand(
  event: KeyboardEvent,
  confirming: boolean,
): { command: string; payload?: unknown } | null {
  if (confirming) return null;
  const letter = event.key.toLowerCase();
  if (event.shiftKey && letter === "k") return { command: "CreateCodeBlock" };
  if (event.shiftKey && letter === "q") return { command: "WrapInBlockquote" };
  if (event.shiftKey && event.code === "Backquote")
    return { command: "ToggleInlineCode" };
  if (letter === "k") return { command: "ToggleLink", payload: { href: "" } };
  if (letter === "t")
    return { command: "InsertTable", payload: { row: 2, col: 2 } };
  if (/^[0-6]$/.test(event.key)) {
    return { command: "WrapInHeading", payload: Number(event.key) };
  }
  return null;
}

function App() {
  const holderRef = useRef<HTMLDivElement>(null);
  const crepeRef = useRef<Crepe | null>(null);
  const savedRef = useRef<string>("");
  const [doc, setDoc] = useState<Document | null>(null);
  const [dirty, setDirty] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [notice, setNotice] = useState("");

  const currentText = useCallback(() => {
    const crepe = crepeRef.current;
    if (!crepe) return savedRef.current;
    // Crepe serialises through its own editor; the trailing newline the
    // markdown writer adds is not part of what the user wrote.
    return crepe.getMarkdown().replace(/\n$/, "");
  }, []);

  // Milkdown owns its command table; this is the seam the page uses to run the
  // edits its own keymap does not cover.
  const runCommand = useCallback((key: string, payload?: unknown) => {
    const crepe = crepeRef.current;
    if (!crepe) return;
    crepe.editor.action((ctx) => ctx.get(commandsCtx).call(key, payload));
  }, []);

  const markDirty = useCallback(() => {
    const isDirty = currentText() !== savedRef.current;
    setDirty(isDirty);
    void invoke("set_dirty", { dirty: isDirty });
  }, [currentText]);

  // Load and mount the editor. The shell hands over the file it was started
  // with; nothing else opens a document.
  useEffect(() => {
    let disposed = false;
    void (async () => {
      const loaded = await invoke<Document>("get_document");
      if (disposed || !holderRef.current) return;
      setDoc(loaded);
      const crepe = new Crepe({
        root: holderRef.current,
        defaultValue: loaded.text,
        featureConfigs: {
          // Every upload path (paste, drop of a file, the upload button) parks
          // the bytes in the temp image directory instead of a blob URL, so the
          // document can point at a file that outlives the session.
          [Crepe.Feature.ImageBlock]: {
            onUpload: storeImage,
            blockOnUpload: storeImage,
            inlineOnUpload: storeImage,
            proxyDomURL: displayUrl,
          },
        },
      });
      crepeRef.current = crepe;
      await crepe.create();
      if (disposed) return;
      // Milkdown normalises the markdown on load; track the normalised form
      // so a clean document never looks dirty, and a save never rewrites a
      // byte when nothing was edited.
      savedRef.current = crepe.getMarkdown().replace(/\n$/, "");
      setDirty(false);
      await invoke("set_dirty", { dirty: false });
      crepe.on((listener) => listener.updated(() => markDirty()));
    })();
    return () => {
      disposed = true;
      const crepe = crepeRef.current;
      crepeRef.current = null;
      void crepe?.destroy();
    };
  }, [markDirty]);

  const save = useCallback(async (): Promise<boolean> => {
    try {
      const text = currentText();
      await invoke("save_document", { text });
      savedRef.current = text;
      setDirty(false);
      setNotice("saved");
      return true;
    } catch (err) {
      setNotice(String(err));
      return false;
    }
  }, [currentText]);

  const saveAndLeave = useCallback(async () => {
    try {
      await invoke("save_and_exit", { text: currentText() });
    } catch (err) {
      setNotice(String(err));
    }
  }, [currentText]);

  const discardAndLeave = useCallback(async () => {
    await invoke("discard_and_exit");
  }, []);

  const acceptAndLeave = useCallback(async () => {
    await invoke("accept_and_exit");
  }, []);

  // Bindings live in the page: ctrl+s saves without leaving, ctrl+enter saves
  // and leaves, ctrl+q asks on a dirty document and leaves on a clean one.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const mod = event.ctrlKey || event.metaKey;
      if (!mod) {
        if (event.key === "Escape" && confirming) {
          event.preventDefault();
          setConfirming(false);
        }
        return;
      }
      const binding = typoraCommand(event, confirming);
      if (binding) {
        event.preventDefault();
        runCommand(binding.command, binding.payload);
        return;
      }
      if (event.key.toLowerCase() === "s") {
        event.preventDefault();
        void save();
      } else if (event.key === "Enter") {
        event.preventDefault();
        void saveAndLeave();
      } else if (event.key.toLowerCase() === "q") {
        event.preventDefault();
        if (dirty) {
          setConfirming(true);
          return;
        }
        void acceptAndLeave();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [acceptAndLeave, confirming, dirty, runCommand, save, saveAndLeave]);

  // The shell cannot tell whether closing means "keep my edits"; it asks.
  useEffect(() => {
    const unlisten = listen("confirm-close", () => setConfirming(true));
    return () => {
      void unlisten.then((fn) => fn());
    };
  }, []);

  // Crepe has no paste handler for image files, so the page stores the bytes in
  // the temp image directory and inserts a reference to that file.
  useEffect(() => {
    const onPaste = (event: ClipboardEvent) => {
      const files = clipImageFiles(event.clipboardData);
      if (files.length === 0) return;
      event.preventDefault();
      void (async () => {
        for (const file of files) {
          try {
            const src = await storeImage(file);
            runCommand("InsertImage", { src, alt: "" });
          } catch (err) {
            setNotice(String(err));
          }
        }
      })();
    };
    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  }, [runCommand]);

  return (
    <main className="nib">
      <header className="nib-header">
        <strong className="nib-name">{doc?.name ?? "nib"}</strong>
        {dirty && <span className="nib-dirty">● unsaved</span>}
        <span className="nib-hint">
          Ctrl+K link · Ctrl+T table · Ctrl+Shift+K code · Ctrl+1-6 heading
        </span>
      </header>
      <div ref={holderRef} className="nib-editor" />
      <footer className="nib-footer">
        {notice && <span className="nib-notice">{notice}</span>}
      </footer>
      {confirming && (
        <div className="nib-confirm" role="dialog">
          <span>Save changes before closing?</span>
          <button onClick={() => void saveAndLeave()}>Save &amp; close</button>
          <button onClick={() => void discardAndLeave()}>Discard</button>
          <button onClick={() => setConfirming(false)}>Keep editing</button>
        </div>
      )}
    </main>
  );
}

createRoot(document.getElementById("root")!).render(<App />);
