// Prevents an extra console window on Windows in release builds.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::io;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use serde::Serialize;
use tauri::{AppHandle, Emitter, Manager, State, WindowEvent};

/// The three exit codes pi's `$EDITOR` contract is built on: accepted,
/// discarded, and could-not-start.
const EXIT_ACCEPTED: i32 = 0;
const EXIT_DISCARDED: i32 = 1;
const EXIT_FAILED: i32 = 2;

/// Every piece of state the window needs: which file, what it last held, and
/// whether the editor has moved away from that.
struct OpenDocument {
    path: PathBuf,
    saved: String,
    dirty: bool,
}

struct AppState(Mutex<OpenDocument>);

#[derive(Serialize)]
struct DocumentPayload {
    name: String,
    text: String,
}

#[tauri::command]
fn get_document(state: State<AppState>) -> DocumentPayload {
    let doc = state.0.lock().expect("state lock");
    DocumentPayload {
        name: file_name(&doc.path),
        text: doc.saved.clone(),
    }
}

#[tauri::command]
fn save_document(text: String, state: State<AppState>) -> Result<(), String> {
    let mut doc = state.0.lock().expect("state lock");
    if !doc.dirty {
        // Nothing was edited, so the round-tripped text must not rewrite the
        // file (Milkdown normalises markdown on load).
        return Ok(());
    }
    write_atomically(&doc.path, &text).map_err(|err| format!("save failed: {err}"))?;
    doc.saved = text;
    doc.dirty = false;
    Ok(())
}

#[tauri::command]
fn set_dirty(dirty: bool, state: State<AppState>) {
    state.0.lock().expect("state lock").dirty = dirty;
}

#[tauri::command]
fn save_and_exit(app: AppHandle, text: String, state: State<AppState>) -> Result<(), String> {
    let mut doc = state.0.lock().expect("state lock");
    if doc.dirty {
        write_atomically(&doc.path, &text).map_err(|err| format!("save failed: {err}"))?;
        doc.saved = text;
        doc.dirty = false;
    }
    drop(doc);
    app.exit(EXIT_ACCEPTED);
    Ok(())
}
#[tauri::command]
fn discard_and_exit(app: AppHandle) {
    app.exit(EXIT_DISCARDED);
}

#[tauri::command]
fn accept_and_exit(app: AppHandle) {
    app.exit(EXIT_ACCEPTED);
}
/// Rejects anything but exactly one path, then loads it. Once this returns the
/// window exists, so a failure here has to leave through the exit code.
fn open_initial(args: &[String]) -> Result<(PathBuf, String), String> {
    let path = match args {
        [one] => PathBuf::from(one),
        [] => return Err("usage: nib <file.md>".into()),
        more => {
            return Err(format!(
                "usage: nib <file.md>: got {} arguments",
                more.len()
            ))
        }
    };
    let bytes = std::fs::read(&path).map_err(|err| format!("open {}: {err}", path.display()))?;
    let text = String::from_utf8_lossy(&bytes);
    Ok((path, strip_bom(&text).to_string()))
}

fn strip_bom(text: &str) -> &str {
    text.strip_prefix('\u{feff}').unwrap_or(text)
}

fn file_name(path: &Path) -> String {
    path.file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_else(|| path.display().to_string())
}

/// Writes beside the target and renames over it, so an interrupted save leaves
/// the old file intact instead of a half-written one.
fn write_atomically(path: &Path, data: &str) -> io::Result<()> {
    let dir = path.parent().unwrap_or_else(|| Path::new("."));
    let tmp = dir.join(format!(".nib-{}.tmp", std::process::id()));
    let result = std::fs::write(&tmp, data)
        .and_then(|()| match std::fs::metadata(path) {
            Ok(meta) => std::fs::set_permissions(&tmp, meta.permissions()),
            Err(_) => Ok(()),
        })
        .and_then(|()| std::fs::rename(&tmp, path));
    if result.is_err() {
        let _ = std::fs::remove_file(&tmp);
    }
    result
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let (path, text) = match open_initial(&args) {
        Ok(opened) => opened,
        Err(message) => {
            eprintln!("{message}");
            std::process::exit(EXIT_FAILED);
        }
    };
    let name = file_name(&path);

    tauri::Builder::default()
        .manage(AppState(Mutex::new(OpenDocument {
            path,
            saved: text,
            dirty: false,
        })))
        .invoke_handler(tauri::generate_handler![
            get_document,
            save_document,
            set_dirty,
            save_and_exit,
            discard_and_exit,
            accept_and_exit
        ])
        .setup({
            let title = format!("nib — {name}");
            move |app| {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.set_title(&title);
                }
                Ok(())
            }
        })
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                let dirty = window
                    .state::<AppState>()
                    .0
                    .lock()
                    .expect("state lock")
                    .dirty;
                if dirty {
                    // Keep the window and let the page ask what to do; the page
                    // answers by saving and exiting, or by discarding.
                    api.prevent_close();
                    let _ = window.emit("confirm-close", ());
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
