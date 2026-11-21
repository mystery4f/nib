import { useCallback, useEffect, useRef, useState } from "react";
import type { ChangeEvent } from "react";
import { createRoot } from "react-dom/client";
import { Crepe } from "@milkdown/crepe";
import { commandsCtx } from "@milkdown/kit/core";
import { replaceAll } from "@milkdown/kit/utils";
import { convertFileSrc, invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { save as chooseFile } from "@tauri-apps/plugin-dialog";
import "@milkdown/crepe/theme/common/style.css";
import "@milkdown/crepe/theme/nord-dark.css";
import "./styles.css";

type Document = { name: string; path: string | null; text: string };

/// Two ways to work: the rendered editor where what you type is what you see,
/// and a split where the markdown source sits beside a read-only rendering.
type Mode = "edit" | "split";

/// How long the split preview waits after the last keystroke before it
/// re-renders; refreshing on every keystroke would make typing crawl.
const PREVIEW_DELAY = 250;

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

/// Every upload path (paste, drop of a file, the upload button) parks the bytes
/// in the temp image directory instead of a blob URL, so the document can point
/// at a file that outlives the session.
const IMAGE_CONFIG = {
  [Crepe.Feature.ImageBlock]: {
    onUpload: storeImage,
    blockOnUpload: storeImage,
    inlineOnUpload: storeImage,
    proxyDomURL: displayUrl,
  },
};

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
  const previewHolderRef = useRef<HTMLDivElement>(null);
  const crepeRef = useRef<Crepe | null>(null);
  const previewRef = useRef<Crepe | null>(null);
  const previewTimerRef = useRef<number | null>(null);
  const textRef = useRef("");
  const savedRef = useRef("");
  const pathRef = useRef<string | null>(null);
  const baselineRef = useRef(false);
  const [doc, setDoc] = useState<Document | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [mode, setMode] = useState<Mode>("edit");
  const [text, setText] = useState("");
  const [dirty, setDirty] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [notice, setNotice] = useState("");

  const publish = useCallback((next: string) => {
    textRef.current = next;
    const isDirty = next !== savedRef.current;
    setDirty(isDirty);
    void invoke("set_dirty", { dirty: isDirty });
  }, []);

  /// The editor is the truth while it is mounted; otherwise the last text the
  /// page saw stands in for it.
  const currentText = useCallback(() => {
    const crepe = crepeRef.current;
    if (!crepe) return textRef.current;
    const serialised = crepe.getMarkdown().replace(/\n$/, "");
    textRef.current = serialised;
    return serialised;
  }, []);

  // Milkdown owns its command table; this is the seam the page uses to run the
  // edits its own keymap does not cover.
  const runCommand = useCallback((key: string, payload?: unknown) => {
    const crepe = crepeRef.current;
    if (!crepe) return;
    crepe.editor.action((ctx) => ctx.get(commandsCtx).call(key, payload));
  }, []);

  // The shell hands over the file it was started with; nothing else opens a
  // document.
  useEffect(() => {
    let disposed = false;
    void (async () => {
      const loadedDoc = await invoke<Document>("get_document");
      if (disposed) return;
      setDoc(loadedDoc);
      pathRef.current = loadedDoc.path;
      textRef.current = loadedDoc.text;
      savedRef.current = loadedDoc.text;
      setText(loadedDoc.text);
      setLoaded(true);
      await invoke("set_dirty", { dirty: false });
    })();
    return () => {
      disposed = true;
    };
  }, []);

  // The rendered editor, mounted only while the page is in edit mode. Its
  // baseline is captured on the first mount: Milkdown normalises the markdown
  // when it parses, and that normalised form is what a clean document holds.
  useEffect(() => {
    if (!loaded || mode !== "edit" || !holderRef.current) return;
    let disposed = false;
    const crepe = new Crepe({
      root: holderRef.current,
      defaultValue: textRef.current,
      featureConfigs: IMAGE_CONFIG,
    });
    crepeRef.current = crepe;
    void (async () => {
      await crepe.create();
      if (disposed) return;
      if (!baselineRef.current) {
        baselineRef.current = true;
        const baseline = crepe.getMarkdown().replace(/\n$/, "");
        savedRef.current = baseline;
        textRef.current = baseline;
        setText(baseline);
        setDirty(false);
        await invoke("set_dirty", { dirty: false });
      }
      crepe.on((listener) =>
        listener.updated(() => publish(crepe.getMarkdown().replace(/\n$/, ""))),
      );
    })();
    return () => {
      disposed = true;
      // Keep what the user typed before the editor goes away, so switching
      // modes never loses an edit.
      textRef.current = crepe.getMarkdown().replace(/\n$/, "");
      crepeRef.current = null;
      void crepe.destroy();
    };
  }, [loaded, mode, publish]);

  // The preview half of the split view: a read-only rendering that follows the
  // source pane.
  useEffect(() => {
    if (!loaded || mode !== "split" || !previewHolderRef.current) return;
    let disposed = false;
    const crepe = new Crepe({
      root: previewHolderRef.current,
      defaultValue: textRef.current,
      featureConfigs: IMAGE_CONFIG,
    });
    previewRef.current = crepe;
    void (async () => {
      await crepe.create();
      if (disposed) return;
      crepe.setReadonly(true);
    })();
    return () => {
      disposed = true;
      previewRef.current = null;
      if (previewTimerRef.current !== null) {
        window.clearTimeout(previewTimerRef.current);
        previewTimerRef.current = null;
      }
      void crepe.destroy();
    };
  }, [loaded, mode]);

  const refreshPreview = useCallback(() => {
    if (previewTimerRef.current !== null)
      window.clearTimeout(previewTimerRef.current);
    previewTimerRef.current = window.setTimeout(() => {
      previewTimerRef.current = null;
      const preview = previewRef.current;
      if (!preview) return;
      preview.editor.action(replaceAll(textRef.current));
    }, PREVIEW_DELAY);
  }, []);

  const onSourceChange = useCallback(
    (event: ChangeEvent<HTMLTextAreaElement>) => {
      const next = event.target.value;
      setText(next);
      publish(next);
      refreshPreview();
    },
    [publish, refreshPreview],
  );

  const toggleMode = useCallback(() => {
    if (mode === "edit") {
      textRef.current = currentText();
      setText(textRef.current);
      setMode("split");
      return;
    }
    setMode("edit");
  }, [currentText, mode]);

  // A document that has never been saved has no path to write to, so the OS
  // dialog supplies one; `chosen` says whether that just happened, because the
  // shell must only rewrite a clean file when the name is fresh.
  const chooseTarget = useCallback(async (): Promise<{
    path: string;
    chosen: boolean;
  } | null> => {
    if (pathRef.current) return { path: pathRef.current, chosen: false };
    const picked = await chooseFile({
      title: "Save markdown",
      defaultPath: "untitled.md",
      filters: [{ name: "Markdown", extensions: ["md"] }],
    });
    if (!picked) return null;
    pathRef.current = picked;
    const name = picked.split(/[\\/]/).pop() || picked;
    setDoc((prev) => (prev ? { ...prev, name, path: picked } : prev));
    return { path: picked, chosen: true };
  }, []);

  const save = useCallback(async (): Promise<boolean> => {
    try {
      const target = await chooseTarget();
      if (!target) {
        setNotice("not saved: no file chosen");
        return false;
      }
      const text = currentText();
      await invoke("save_document", {
        text,
        path: target.chosen ? target.path : null,
      });
      savedRef.current = text;
      setDirty(false);
      setNotice("saved");
      return true;
    } catch (err) {
      setNotice(String(err));
      return false;
    }
  }, [chooseTarget, currentText]);

  const saveAndLeave = useCallback(async () => {
    try {
      const target = await chooseTarget();
      if (!target) {
        setNotice("not saved: no file chosen");
        return;
      }
      await invoke("save_and_exit", {
        text: currentText(),
        path: target.chosen ? target.path : null,
      });
    } catch (err) {
      setNotice(String(err));
    }
  }, [chooseTarget, currentText]);
  const discardAndLeave = useCallback(async () => {
    await invoke("discard_and_exit");
  }, []);

  const acceptAndLeave = useCallback(async () => {
    await invoke("accept_and_exit");
  }, []);

  // Bindings live in the page: ctrl+s saves without leaving, ctrl+enter saves
  // and leaves, ctrl+q asks on a dirty document and leaves on a clean one, f9
  // swaps the view.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "F9") {
        event.preventDefault();
        toggleMode();
        return;
      }
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
  }, [
    acceptAndLeave,
    confirming,
    dirty,
    runCommand,
    save,
    saveAndLeave,
    toggleMode,
  ]);

  // The shell cannot tell whether closing means "keep my edits"; it asks.
  useEffect(() => {
    const unlisten = listen("confirm-close", () => setConfirming(true));
    return () => {
      void unlisten.then((fn) => fn());
    };
  }, []);

  // Crepe has no paste handler for image files, so the page stores the bytes in
  // the temp image directory and inserts a reference to that file. The source
  // pane of the split view is a plain textarea and takes care of itself.
  useEffect(() => {
    if (mode !== "edit") return;
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
  }, [mode, runCommand]);

  return (
    <main className="nib">
      <header className="nib-header">
        <strong className="nib-name">{doc?.name ?? "nib"}</strong>
        {dirty && <span className="nib-dirty">● unsaved</span>}
        <button className="nib-mode" onClick={toggleMode} title="F9">
          {mode === "edit" ? "Split view" : "Edit view"}
        </button>
        <span className="nib-hint">
          Ctrl+K link · Ctrl+T table · Ctrl+Shift+K code · Ctrl+1-6 heading
        </span>
      </header>
      {mode === "edit" ? (
        <div ref={holderRef} className="nib-editor" />
      ) : (
        <div className="nib-split">
          <textarea
            className="nib-source"
            value={text}
            onChange={onSourceChange}
            spellCheck={false}
            aria-label="markdown source"
          />
          <div ref={previewHolderRef} className="nib-preview" />
        </div>
      )}
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
