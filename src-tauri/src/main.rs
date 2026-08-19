// 发布构建时不显示宿主控制台窗口
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::fs;
use std::net::{TcpListener, TcpStream};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::thread;
use std::time::{Duration, Instant};

use tauri::{Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};

/// 编译期把 Go 服务端整体嵌入本 exe（单文件分发的关键）
const SERVER_BIN: &[u8] = include_bytes!("../binaries/server-x86_64-pc-windows-msvc.exe");

/// 保存 Go 服务子进程句柄，应用退出时负责杀掉
struct ServerProcess(Mutex<Option<Child>>);

/// 让操作系统分配一个空闲端口，避免写死端口导致冲突
fn pick_free_port() -> u16 {
    TcpListener::bind("127.0.0.1:0")
        .and_then(|listener| listener.local_addr())
        .map(|addr| addr.port())
        .unwrap_or(18080)
}

/// 轮询等待 Go 服务就绪（端口可连接即视为就绪）
fn wait_for_server(port: u16, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    let addr = format!("127.0.0.1:{port}");
    while Instant::now() < deadline {
        if TcpStream::connect(&addr).is_ok() {
            return true;
        }
        thread::sleep(Duration::from_millis(200));
    }
    false
}

/// FNV-1a 哈希，用于判断内嵌二进制是否变化（避免每次启动都重写 10MB）
fn fnv1a(data: &[u8]) -> u64 {
    let mut hash: u64 = 0xcbf29ce484222325;
    for &byte in data {
        hash ^= byte as u64;
        hash = hash.wrapping_mul(0x100000001b3);
    }
    hash
}

/// 把内嵌的 server.exe 释放到 %LOCALAPPDATA%\EDHPowerLevel\runtime\，
/// 已有同内容文件则直接复用
fn extract_server() -> std::io::Result<PathBuf> {
    let base = std::env::var_os("LOCALAPPDATA")
        .map(PathBuf::from)
        .unwrap_or_else(std::env::temp_dir);
    let dir = base.join("EDHPowerLevel").join("runtime");
    fs::create_dir_all(&dir)?;

    let exe = dir.join("server.exe");
    let stamp = dir.join("server.stamp");
    let hash = fnv1a(SERVER_BIN).to_string();

    let up_to_date = exe.exists()
        && fs::read_to_string(&stamp)
            .map(|saved| saved.trim() == hash)
            .unwrap_or(false);
    if !up_to_date {
        fs::write(&exe, SERVER_BIN)?;
        fs::write(&stamp, hash)?;
    }
    Ok(exe)
}

/// 拉起 Go 服务进程（无控制台窗口、丢弃输出、随机端口）
fn spawn_server(exe: &PathBuf, port: u16) -> std::io::Result<Child> {
    let mut cmd = Command::new(exe);
    cmd.env("APP_ADDRESS", format!("127.0.0.1:{port}"))
        // 阻止 server.exe 自己弹系统浏览器
        .env("POWERLEVEL_OPEN_BROWSER", "0")
        .env("BROWSER_HEADLESS", "true")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x08000000;
        cmd.creation_flags(CREATE_NO_WINDOW);
    }
    cmd.spawn()
}

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            let port = pick_free_port();

            let exe = extract_server().map_err(|e| format!("释放内嵌 server 失败: {e}"))?;
            let child = spawn_server(&exe, port).map_err(|e| format!("启动 server 失败: {e}"))?;
            app.manage(ServerProcess(Mutex::new(Some(child))));

            if !wait_for_server(port, Duration::from_secs(15)) {
                return Err("server 15 秒内未就绪".into());
            }

            let url: tauri::Url = format!("http://127.0.0.1:{port}").parse().unwrap();
            WebviewWindowBuilder::new(app, "main", WebviewUrl::External(url))
                .title("EDH Power Level")
                .inner_size(1280.0, 840.0)
                .min_inner_size(960.0, 640.0)
                .on_navigation(|url| {
                    // Open any link the front-end navigates to (e.g. the provider/e-drop
                    // "查看来源"/"在 EDHREC 查看" links) in the user's default browser
                    // instead of replacing the Tauri window. The window's own origin
                    // (127.0.0.1) never navigates away, so every navigation event is an
                    // external link.
                    let _ = open::that(url.to_string());
                    false
                })
                .build()?;

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while running tauri application")
        .run(|app, event| {
            // 应用退出时杀掉 Go 子进程，避免残留
            if let RunEvent::Exit = event {
                if let Some(state) = app.try_state::<ServerProcess>() {
                    if let Some(mut child) = state.0.lock().unwrap().take() {
                        let _ = child.kill();
                    }
                }
            }
        });
}
