mod qa_profile;
mod commands;
mod neko;
mod tray;

pub use neko::computer::gmail as gmail_signal_adapter;
pub use neko::computer::motor as realtime_motor;
pub use neko::computer::watcher as app_event_watcher;
use neko::computer::{AppEventPump, NekoComputerService};
use neko::coworker::CoworkerRecords;
use neko::runtime::NekoRuntime;
use tauri::Manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let profile = match qa_profile::NativeProfile::parse(std::env::args_os().skip(1), cfg!(debug_assertions)) {
        Ok(value) => value,
        Err(error) => { eprintln!("{error}"); std::process::exit(2); }
    };
    if let Err(error) = profile.prepare() { eprintln!("{error}"); std::process::exit(2); }
    if profile.check_only {
        println!("{}", serde_json::json!({ "qaProfile": profile.metadata(), "stateDir": profile.resolve_state(|| Err("default_disabled".into())).ok(), "webviewDir": profile.webview_dir() }));
        return;
    }
    let qa_active = profile.active();
    let mut context = tauri::generate_context!();
    if qa_active {
        for window in &mut context.config_mut().app.windows { window.create = false; }
        context.config_mut().app.security.csp = Some(tauri::utils::config::Csp::Policy(qa_profile::QA_CSP.to_owned()));
        context.config_mut().app.security.dev_csp = Some(tauri::utils::config::Csp::Policy(qa_profile::QA_CSP.to_owned()));
    }
    let mut builder = tauri::Builder::default().manage(profile.clone());
    if !qa_active {
        builder = builder
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.unminimize();
                let _ = window.set_focus();
            }
        }));
    }
    builder
        .plugin(tauri_plugin_store::Builder::new().build())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_notification::init())
        .invoke_handler({
            let guarded_profile = profile.clone();
            let handler: fn(tauri::ipc::Invoke<tauri::Wry>) -> bool = tauri::generate_handler![
            commands::qa_profile::neko_qa_store_path,
            commands::qa_profile::neko_qa_runtime_profile,
            commands::neko_bot::neko_bot_local_status,
            commands::neko_bot::neko_wiii_service_fetch,
            commands::neko_bot::neko_wiii_service_cancel,
            commands::neko_bot::neko_bot_local_connect,
            commands::neko_bot::neko_bot_local_disconnect,
            commands::neko_bot::neko_bot_local_link,
            commands::neko_bot::neko_bot_local_request,
            commands::health::check_server_reachable,
            commands::files::pick_document,
            commands::quick_chat::neko_create_quick_chat_workspace,
            commands::files::neko_resolve_workspace,
            commands::files::neko_list_workspace_files,
            commands::files::neko_read_workspace_file,
            commands::files::neko_workspace_changes,
            commands::files::neko_workspace_diff,
            commands::splash::close_splash,
            commands::neko_agent::neko_control_provider_list,
            commands::neko_agent::neko_control_provider_profiles,
            commands::neko_agent::neko_control_provider_sessions,
            commands::neko_agent::neko_control_session_list,
            commands::neko_agent::neko_control_session_start,
            commands::neko_agent::neko_control_session_write,
            commands::neko_agent::neko_control_session_cancel,
            commands::neko_agent::neko_control_events_read,
            commands::neko_computer::neko_computer_doctor,
            commands::neko_computer::neko_computer_coworker_status,
            commands::neko_computer::neko_computer_project_grant,
            commands::neko_computer::neko_computer_project_revoke,
            commands::neko_computer::neko_computer_ensure,
            commands::neko_computer::neko_computer_package_install,
            commands::neko_computer::neko_computer_package_remove,
            commands::neko_computer::neko_computer_remove,
            commands::neko_computer::neko_computer_suspend,
            commands::neko_computer::neko_computer_resume,
            commands::neko_computer::neko_computer_reset,
            commands::neko_computer::neko_computer_seat_acquire,
            commands::neko_computer::neko_computer_seat_release,
            commands::neko_computer::neko_computer_terminal_exec,
            commands::neko_computer::neko_computer_browser_navigate,
            commands::neko_computer::neko_computer_events_read,
            commands::neko_computer::neko_computer_app_events_poll,
            commands::neko_computer::neko_computer_semantic_observe,
            commands::neko_computer::neko_computer_semantic_act,
            commands::neko_computer::neko_computer_history_status,
            commands::neko_computer::neko_computer_history_set_enabled,
            commands::neko_computer::neko_computer_history_query,
            commands::neko_computer::neko_computer_history_delete,
            commands::neko_computer::neko_signal_inbox_consult,
            commands::neko_computer::neko_signal_inbox_claim,
            commands::neko_computer::neko_signal_inbox_defer,
            commands::neko_computer::neko_signal_inbox_resolve,
            commands::neko_computer::neko_signal_inbox_revoke_account,
            commands::neko_computer::neko_computer_work_plane_describe,
            commands::neko_computer::neko_computer_work_plane_query,
            commands::neko_computer::neko_computer_work_plane_execute,
        ];
            move |invoke: tauri::ipc::Invoke| {
                if !guarded_profile.command_allowed(invoke.message.command()) {
                    invoke.resolver.reject("wiii_qa_profile:command_disabled"); true
                } else { handler(invoke) }
            }
        })
        .setup(move |app| {
            app.manage(neko::local_bot_bridge::LocalBotBridge::default());
            app.manage(neko::local_service_http::ServiceHttp::default());
            neko::bundled_provider::initialize(app.path().resource_dir()?)?;
            let data_dir = qa_profile::data_dir(app.handle()).map_err(std::io::Error::other)?;
            let runtime = NekoRuntime::open(&data_dir.join("neko-runtime-v1.sqlite3"))
                .map_err(std::io::Error::other)?;
            app.manage(runtime);
            let computer = NekoComputerService::open(&data_dir)
                .and_then(|computer| {
                    if !qa_active {
                        let pump = AppEventPump::start(computer.clone())?;
                        app.manage(pump);
                    }
                    Ok(computer)
                })
                .map_err(|error| format!("computer_unavailable: Computer chưa khả dụng. Dữ liệu đã được giữ nguyên; kiểm tra quyền truy cập hoặc khôi phục bản sao lưu rồi mở lại Wiii. Chi tiết: {error}"));
            app.manage(computer);
            let coworker_records = CoworkerRecords::open(
                &data_dir.join("neko-coworker-v1").join("records-v1.sqlite3"),
            )
            .map_err(std::io::Error::other)?;
            app.manage(coworker_records);
            if qa_active {
                let mut main = tauri::WebviewWindowBuilder::new(app, "main", tauri::WebviewUrl::App("index.html".into()))
                    .title("Wiii · isolated local QA").inner_size(1200.0, 800.0).min_inner_size(800.0, 600.0)
                    .decorations(false).data_directory(profile.webview_dir().ok_or("qa_webview_path_missing")?)
                    .on_navigation(|url| neko::local_bot_bridge::trusted_surface("main", url, false));
                if let Some(script) = profile.initialization_script().map_err(std::io::Error::other)? { main = main.initialization_script(script); }
                main.build()?;
            } else { tray::create_tray(app)?; }
            Ok(())
        })
        .on_window_event(move |window, event| {
            // Minimize to tray on close (main window only)
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                if window.label() == "main" && !qa_active {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .build(context)
        .expect("error while running Wiii desktop")
        .run(|app, event| {
            // Phase 2A is in-process: a graceful app exit cancels every owned
            // child. Hard-crash recovery is classified from the journal.
            if let tauri::RunEvent::Exit = event {
                if let Some(pump) = app.try_state::<AppEventPump>() {
                    pump.shutdown();
                }
                app.state::<NekoRuntime>().kill_all(app);
            }
        });
}
