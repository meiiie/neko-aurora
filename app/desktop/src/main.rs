#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod local_instance;
use std::sync::{
    atomic::{AtomicBool, Ordering},
    Arc,
};

fn main() {
    if let Err(error) = run() {
        let reason = error.to_string();
        let reason = if reason.starts_with("local_")
            && reason.bytes().all(|c| c.is_ascii_lowercase() || c == b'_')
        {
            reason.as_str()
        } else {
            "desktop_startup_failed"
        };
        eprintln!("verified_local_instance_required: {reason}");
        std::process::exit(1)
    }
}
fn run() -> Result<(), Box<dyn std::error::Error>> {
    let config = local_instance::parse_args(&std::env::args().skip(1).collect::<Vec<_>>())?;
    let instance = Arc::new(local_instance::verify(&config)?);
    if config.verify_only {
        println!("{}", serde_json::to_string(&instance.binding)?);
        return Ok(());
    }
    let data = local_instance::profile_directory(&config.state)?;
    let running = Arc::new(AtomicBool::new(true));
    let monitor = running.clone();
    let setup_instance = instance.clone();
    let result = tauri::Builder::default()
        .setup(move |app| {
            let navigation = setup_instance.clone();
            let window = tauri::WebviewWindowBuilder::new(
                app,
                "main",
                tauri::WebviewUrl::External(setup_instance.binding.endpoint.parse()?),
            )
            .title("Neko Bot")
            .inner_size(1220.0, 820.0)
            .min_inner_size(640.0, 560.0)
            .data_directory(data.clone())
            .incognito(true)
            .on_navigation(move |url| navigation.allows(url) && navigation.live())
            .on_new_window(|_, _| tauri::webview::NewWindowResponse::Deny)
            .build()?;
            let liveness = setup_instance.clone();
            let alive = monitor.clone();
            std::thread::spawn(move || {
                while alive.load(Ordering::Acquire) {
                    if !liveness.live() {
                        let _ = window.close();
                        break;
                    };
                    std::thread::sleep(std::time::Duration::from_millis(500))
                }
            });
            Ok(())
        })
        .run(tauri::generate_context!());
    running.store(false, Ordering::Release);
    result.map_err(Into::into)
}
