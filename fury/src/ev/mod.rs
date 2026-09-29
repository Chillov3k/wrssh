#[cfg(windows)]
pub mod pb;

/// Called once at startup: apply low-noise stealth adjustments.
pub fn init() {
    #[cfg(windows)]
    {
        pb::mask_process();
    }
}

/// Jittered sleep between reconnects; keeps long idle periods cheap.
pub async fn sleep_jitter(base_secs: u64, jitter_secs: u64) {
    let jitter = crate::rand_range(jitter_secs);
    tokio::time::sleep(std::time::Duration::from_secs(base_secs + jitter)).await;
}
