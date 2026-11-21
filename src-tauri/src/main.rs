// Prevents an extra console window on Windows in release builds.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::io;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Mutex;

use base64::engine::general_purpose::STANDARD;
use base64::Engine as _;

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
    path: Option<PathBuf>,
    saved: String,
    dirty: bool,
}

struct AppState(Mutex<OpenDocument>);

#[derive(Serialize)]
struct DocumentPayload {
    name: String,
    path: Option<String>,
    text: String,
}

#[tauri::command]
fn get_document(state: State<AppState>) -> DocumentPayload {
    let doc = state.0.lock().expect("state lock");
    DocumentPayload {
        name: path_name(doc.path.as_deref()),
        path: doc.path.as_ref().map(|path| path.display().to_string()),
        text: doc.saved.clone(),
    }
}

#[tauri::command]
fn save_document(
    app: AppHandle,
    text: String,
    path: Option<String>,
    state: State<AppState>,
) -> Result<(), String> {
    let mut doc = state.0.lock().expect("state lock");
    let chosen = path.is_some();
    let target = target_path(&mut doc, path)?;
    if !chosen && !doc.dirty {
        // Nothing was edited, so the round-tripped text must not rewrite the
        // file (Milkdown normalises markdown on load).
        return Ok(());
    }
    write_atomically(&target, &text).map_err(|err| format!("save failed: {err}"))?;
    doc.saved = text;
    doc.dirty = false;
    let name = path_name(doc.path.as_deref());
    drop(doc);
    retitle(&app, &name);
    Ok(())
}

#[tauri::command]
fn set_dirty(dirty: bool, state: State<AppState>) {
    state.0.lock().expect("state lock").dirty = dirty;
}

#[tauri::command]
fn save_and_exit(
    app: AppHandle,
    text: String,
    path: Option<String>,
    state: State<AppState>,
) -> Result<(), String> {
    let mut doc = state.0.lock().expect("state lock");
    let chosen = path.is_some();
    let target = target_path(&mut doc, path)?;
    if doc.dirty || chosen {
        write_atomically(&target, &text).map_err(|err| format!("save failed: {err}"))?;
    }
    doc.saved = text;
    doc.dirty = false;
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

/// Pasted and uploaded images are written here: a scratch area that survives
/// until the system clears the temp directory.
fn images_dir() -> PathBuf {
    std::env::temp_dir().join("nib").join("images")
}

static NEXT_IMAGE: AtomicU64 = AtomicU64::new(0);

/// Names an image so two pastes never collide and the extension still tells a
/// browser which decoder to use.
fn image_name(name: &str) -> String {
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|elapsed| elapsed.as_millis())
        .unwrap_or(0);
    let seq = NEXT_IMAGE.fetch_add(1, Ordering::Relaxed);
    let ext = Path::new(name)
        .extension()
        .map(|ext| ext.to_string_lossy().to_lowercase())
        .filter(|ext| !ext.is_empty() && ext.chars().all(|c| c.is_ascii_alphanumeric()))
        .unwrap_or_else(|| "png".to_string());
    format!("paste-{stamp}-{seq}.{ext}")
}

fn write_image(dir: &Path, name: &str, bytes: &[u8]) -> io::Result<PathBuf> {
    std::fs::create_dir_all(dir)?;
    let path = dir.join(image_name(name));
    std::fs::write(&path, bytes)?;
    Ok(path)
}

#[tauri::command]
fn open_images_dir() -> Result<String, String> {
    let dir = images_dir();
    std::fs::create_dir_all(&dir).map_err(|err| format!("create {}: {err}", dir.display()))?;
    Ok(dir.display().to_string())
}

/// Base64 in, file on disk out. The page cannot write files itself, and the
/// document keeps the plain path so the markdown stays readable.
#[tauri::command]
fn save_pasted_image(name: String, data: String) -> Result<String, String> {
    let bytes = STANDARD
        .decode(data.as_bytes())
        .map_err(|err| format!("image data: {err}"))?;
    let path =
        write_image(&images_dir(), &name, &bytes).map_err(|err| format!("save image: {err}"))?;
    Ok(path.display().to_string())
}
/// Reads the path the shell was started with. No path at all is a blank
/// document — that is how the app is opened from a shortcut — while more than
/// one path is a mistake. A failure here leaves before the window exists, so it
/// has to leave through the exit code.
fn open_initial(args: &[String]) -> Result<(Option<PathBuf>, String), String> {
    let path = match args {
        [one] => PathBuf::from(one),
        [] => return Ok((None, String::new())),
        more => {
            return Err(format!(
                "usage: nib <file.md>: got {} arguments",
                more.len()
            ))
        }
    };
    let bytes = std::fs::read(&path).map_err(|err| format!("open {}: {err}", path.display()))?;
    let text = String::from_utf8_lossy(&bytes);
    Ok((Some(path), strip_bom(&text).to_string()))
}

fn strip_bom(text: &str) -> &str {
    text.strip_prefix('\u{feff}').unwrap_or(text)
}

fn file_name(path: &Path) -> String {
    path.file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_else(|| path.display().to_string())
}

/// Untitled documents have no file name yet; the title has to say so rather than
/// show nothing.
fn path_name(path: Option<&Path>) -> String {
    path.map(file_name)
        .unwrap_or_else(|| "untitled".to_string())
}

/// Which file a save writes to: the one the page just chose, else the one the
/// document already belongs to.
fn target_path(doc: &mut OpenDocument, chosen: Option<String>) -> Result<PathBuf, String> {
    if let Some(chosen) = chosen {
        let path = PathBuf::from(chosen);
        doc.path = Some(path.clone());
        return Ok(path);
    }
    doc.path
        .clone()
        .ok_or_else(|| "this document has no file yet: choose a name to save it".to_string())
}

/// The title names the file, so a save-as has to update it in place.
fn retitle(app: &AppHandle, name: &str) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.set_title(&format!("nib — {name}"));
    }
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
    let name = path_name(path.as_deref());

    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
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
            accept_and_exit,
            open_images_dir,
            save_pasted_image
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::sync::atomic::{AtomicUsize, Ordering};

    static NEXT_DIR: AtomicUsize = AtomicUsize::new(0);

    /// A fresh directory under the system temp dir, removed when the guard drops.
    struct ScratchDir(PathBuf);

    impl ScratchDir {
        fn new() -> Self {
            let n = NEXT_DIR.fetch_add(1, Ordering::Relaxed);
            let dir = std::env::temp_dir().join(format!("nib-test-{}-{n}", std::process::id()));
            fs::create_dir_all(&dir).expect("scratch dir");
            Self(dir)
        }

        fn file(&self, name: &str) -> PathBuf {
            self.0.join(name)
        }
    }

    impl Drop for ScratchDir {
        fn drop(&mut self) {
            let _ = fs::remove_dir_all(&self.0);
        }
    }

    fn args(list: &[&str]) -> Vec<String> {
        list.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn open_initial_starts_blank_without_arguments() {
        let (path, text) = open_initial(&[]).expect("no path is a blank document");
        assert_eq!(path, None);
        assert_eq!(text, "");
    }

    #[test]
    fn open_initial_rejects_extra_arguments() {
        assert_eq!(
            open_initial(&args(&["a.md", "b.md"])),
            Err("usage: nib <file.md>: got 2 arguments".into())
        );
    }

    #[test]
    fn open_initial_reports_an_unreadable_file() {
        let scratch = ScratchDir::new();
        let missing = scratch.file("absent.md");
        let error = open_initial(&[missing.display().to_string()]).expect_err("should fail");
        assert!(
            error.starts_with(&format!("open {}", missing.display())),
            "unexpected message: {error}"
        );
    }

    #[test]
    fn open_initial_strips_the_bom_and_keeps_the_bytes() {
        let scratch = ScratchDir::new();
        let path = scratch.file("bom.md");
        fs::write(&path, "\u{feff}# title\n\nbody\n").expect("write fixture");
        let (opened, text) = open_initial(&[path.display().to_string()]).expect("should open");
        assert_eq!(opened, Some(path));
        assert_eq!(text, "# title\n\nbody\n");
    }

    #[test]
    fn write_atomically_replaces_the_file_and_leaves_no_temp() {
        let scratch = ScratchDir::new();
        let path = scratch.file("doc.md");
        fs::write(&path, "old\n").expect("write fixture");
        write_atomically(&path, "new\ntext").expect("should write");
        assert_eq!(fs::read_to_string(&path).expect("read back"), "new\ntext");
        let leftovers: Vec<_> = fs::read_dir(&scratch.0)
            .expect("list dir")
            .map(|entry| {
                entry
                    .expect("entry")
                    .file_name()
                    .to_string_lossy()
                    .into_owned()
            })
            .filter(|name| name.ends_with(".tmp"))
            .collect();
        assert!(
            leftovers.is_empty(),
            "temp files left behind: {leftovers:?}"
        );
    }

    #[test]
    fn write_atomically_fails_without_touching_the_target() {
        let scratch = ScratchDir::new();
        let path = scratch.file("doc.md");
        fs::write(&path, "keep\n").expect("write fixture");
        let into_missing_dir = scratch.file("nope").join("doc.md");
        assert!(write_atomically(&into_missing_dir, "lost").is_err());
        assert_eq!(fs::read_to_string(&path).expect("read back"), "keep\n");
    }

    #[test]
    fn file_name_falls_back_to_the_whole_path() {
        assert_eq!(file_name(Path::new(r"C:\docs\note.md")), "note.md");
    }

    #[test]
    fn path_name_says_untitled_before_a_save_as() {
        assert_eq!(path_name(None), "untitled");
        assert_eq!(path_name(Some(Path::new(r"C:\docs\note.md"))), "note.md");
    }

    #[test]
    fn target_path_adopts_a_chosen_name_and_remembers_it() {
        let scratch = ScratchDir::new();
        let chosen = scratch.file("chosen.md");
        let mut doc = OpenDocument {
            path: None,
            saved: String::new(),
            dirty: true,
        };
        let target = target_path(&mut doc, Some(chosen.display().to_string())).expect("chosen");
        assert_eq!(target, chosen);
        assert_eq!(doc.path, Some(chosen.clone()));
        assert_eq!(target_path(&mut doc, None).expect("remembered"), chosen);
    }

    #[test]
    fn target_path_refuses_to_guess_without_a_choice() {
        let mut doc = OpenDocument {
            path: None,
            saved: String::new(),
            dirty: false,
        };
        let error = target_path(&mut doc, None).expect_err("should refuse");
        assert!(error.contains("no file yet"), "unexpected message: {error}");
    }
    #[test]
    fn write_image_keeps_the_extension_of_the_source_name() {
        let scratch = ScratchDir::new();
        let path = write_image(&scratch.0, "shot.JPG", b"bytes").expect("write image");
        assert!(
            path.starts_with(&scratch.0),
            "outside the images dir: {path:?}"
        );
        assert_eq!(path.extension().and_then(|ext| ext.to_str()), Some("jpg"));
        assert_eq!(fs::read(&path).expect("read back"), b"bytes");
    }

    #[test]
    fn write_image_falls_back_to_png_and_never_reuses_a_name() {
        let scratch = ScratchDir::new();
        let first = write_image(&scratch.0, "clipboard", b"one").expect("first");
        let second = write_image(&scratch.0, "clipboard", b"two").expect("second");
        assert_ne!(first, second);
        assert_eq!(first.extension().and_then(|ext| ext.to_str()), Some("png"));
    }

    #[test]
    fn image_name_rejects_a_traversing_suffix() {
        let name = image_name("../../evil.sh/../x");
        assert!(
            !name.contains('/') && !name.contains('\\'),
            "unexpected name: {name}"
        );
    }
}
