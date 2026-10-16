use crate::neko::local_bot_bridge::{
    trusted_surface, Link, LinkInput, LocalBotBridge, LocalStatus, Request, Response,
};
use crate::neko::local_service_http::{ServiceEvent, ServiceHttp, ServiceRequest};
use tauri::{AppHandle, State, WebviewWindow};
use tauri_plugin_dialog::{DialogExt, MessageDialogButtons};

pub(crate) fn main_local(window: &WebviewWindow) -> Result<(), String> {
    let url = window
        .url()
        .map_err(|_| "neko_bot_local:untrusted_surface".to_owned())?;
    if !trusted_surface(window.label(), &url, cfg!(debug_assertions)) {
        return Err("neko_bot_local:untrusted_surface".to_owned());
    }
    Ok(())
}

#[tauri::command]
pub fn neko_bot_local_status(
    window: WebviewWindow,
    state: State<'_, LocalBotBridge>,
) -> Result<LocalStatus, String> {
    main_local(&window)?;
    state.status()
}

#[tauri::command]
pub async fn neko_bot_local_connect(
    window: WebviewWindow,
    app: AppHandle,
    state: State<'_, LocalBotBridge>,
    port: u16,
) -> Result<LocalStatus, String> {
    main_local(&window)?;
    let generation = state.begin_connect(port)?;
    let approved = tauri::async_runtime::spawn_blocking(move || {
        app.dialog().message(format!("Kết nối Wiii với Neko Bot local tại 127.0.0.1:{port}?\n\nKết nối dùng quyền chủ sở hữu local của service này để xem danh sách kênh và gửi các tin bạn chọn gửi. Cookie chỉ ở Rust trong phiên hiện tại. Đây không phải xác minh danh tính tiến trình hoặc cấp quyền cloud.\n\nChỉ tiếp tục nếu bạn tin cậy service đang chạy trên cổng này."))
            .title("Wiii · Neko Bot local")
            .buttons(MessageDialogButtons::OkCancelCustom("Kết nối".to_owned(), "Hủy".to_owned())).blocking_show()
    }).await;
    match approved {
        Ok(value) => state.finish_connect(generation, port, value).await,
        Err(_) => {
            state.abort_connect(generation)?;
            Err("neko_bot_local:confirmation_unavailable".to_owned())
        }
    }
}

#[tauri::command]
pub fn neko_bot_local_disconnect(
    window: WebviewWindow,
    state: State<'_, LocalBotBridge>,
) -> Result<(), String> {
    main_local(&window)?;
    state.disconnect()
}

#[tauri::command]
pub fn neko_bot_local_link(
    window: WebviewWindow,
    state: State<'_, LocalBotBridge>,
    input: LinkInput,
) -> Result<Link, String> {
    main_local(&window)?;
    state.link(input)
}

#[tauri::command]
pub async fn neko_bot_local_request(
    window: WebviewWindow,
    state: State<'_, LocalBotBridge>,
    input: Request,
) -> Result<Response, String> {
    main_local(&window)?;
    state.request(input).await
}

#[tauri::command]
pub async fn neko_wiii_service_fetch(
    window: WebviewWindow,
    state: State<'_, ServiceHttp>,
    profile: State<'_, crate::qa_profile::NativeProfile>,
    request: ServiceRequest,
    on_event: tauri::ipc::Channel<ServiceEvent>,
) -> Result<(), String> {
    main_local(&window)?;
    profile.service_allowed(request.port, request.remote)?;
    state
        .fetch(request, move |event| {
            on_event
                .send(event)
                .map_err(|_| "wiii_service_http:receiver_closed".to_owned())
        })
        .await
}

#[tauri::command]
pub fn neko_wiii_service_cancel(
    window: WebviewWindow,
    state: State<'_, ServiceHttp>,
    request_id: String,
) -> Result<(), String> {
    main_local(&window)?;
    state.cancel(&request_id)
}
