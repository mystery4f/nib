import { useCallback, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Crepe } from "@milkdown/crepe";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import "@milkdown/crepe/theme/common/style.css";
import "@milkdown/crepe/theme/classic.css";
import "./styles.css";

type Document = { name: string; text: string };

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
  }, [acceptAndLeave, confirming, dirty, save, saveAndLeave]);

  // The shell cannot tell whether closing means "keep my edits"; it asks.
  useEffect(() => {
    const unlisten = listen("confirm-close", () => setConfirming(true));
    return () => {
      void unlisten.then((fn) => fn());
    };
  }, []);

  return (
    <main className="nib">
      <header className="nib-header">
        <strong className="nib-name">{doc?.name ?? "nib"}</strong>
        {dirty && <span className="nib-dirty">● unsaved</span>}
        <span className="nib-hint">
          Ctrl+S save · Ctrl+Enter save &amp; close · Ctrl+Q close
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
