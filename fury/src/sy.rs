pub fn username() -> String {
    std::env::var("USERNAME")
        .or_else(|_| std::env::var("USER"))
        .unwrap_or_else(|_| "Unknown".to_string())
}

pub fn hostname() -> String {
    gethostname()
}

#[cfg(windows)]
fn gethostname() -> String {
    std::env::var("COMPUTERNAME").unwrap_or_else(|_| "Unknown".to_string())
}

#[cfg(unix)]
fn gethostname() -> String {
    std::env::var("HOSTNAME").unwrap_or_else(|_| "Unknown".to_string())
}

/// Environment without shell history persistence, mirrors the Go noHistoryEnv.
pub fn no_history_env() -> Vec<(String, String)> {
    let mut envs: Vec<(String, String)> = std::env::vars().collect();
    #[cfg(windows)]
    {
        envs.retain(|(k, _)| !matches!(k.to_uppercase().as_str(), "PSREADLINE_HISTORY_PATH" | "PSHISTORY" ));
        envs.push(("PSReadLineHistoryPath".into(), String::new()));
    }
    #[cfg(unix)]
    {
        envs.retain(|(k, _)| !matches!(k.as_str(), "HISTFILE" | "HISTSIZE" | "LESSHISTFILE" | "MYSQL_HISTFILE" | "REDISCLI_HISTFILE"));
        envs.push(("HISTFILE".into(), "/dev/null".into()));
        envs.push(("HISTSIZE".into(), "0".into()));
        envs.push(("LESSHISTFILE".into(), "-".into()));
    }
    envs
}
