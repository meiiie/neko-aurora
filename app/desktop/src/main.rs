#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            let url = "http://127.0.0.1:8765".parse()?;
            let mut window=tauri::WebviewWindowBuilder::new(app, "main", tauri::WebviewUrl::External(url))
                .title("Neko Bot")
                .inner_size(1220.0, 820.0)
                .min_inner_size(640.0, 560.0);
            #[cfg(debug_assertions)]
            if let (Ok(directory),Ok(port))=(std::env::var("NEKO_TEST_WEBVIEW_DIR"),std::env::var("NEKO_TEST_DEBUG_PORT")) {
                let directory=std::fs::canonicalize(directory)?;
                let port=port.parse::<u16>()?;
                if port==0 {return Err("Invalid owned test debug port".into());}
                window=window.data_directory(directory).additional_browser_args(&format!("--remote-debugging-address=127.0.0.1 --remote-debugging-port={port} --disable-background-networking --disable-sync"));
            }
            window.build()?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("Neko Bot desktop could not start");
}
