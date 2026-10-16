use crate::qa_profile::{NativeProfile, QaMetadata};
use tauri::{State, WebviewWindow};

#[tauri::command]
pub fn neko_qa_store_path(window: WebviewWindow, state: State<'_, NativeProfile>, name: String) -> Result<String, String> {
    super::neko_bot::main_local(&window)?;
    state.store_path(&name)
}

#[tauri::command]
pub fn neko_qa_runtime_profile(window: WebviewWindow, state: State<'_, NativeProfile>) -> Result<QaMetadata, String> {
    super::neko_bot::main_local(&window)?;
    Ok(state.metadata())
}
