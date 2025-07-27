use std::fs::OpenOptions;
use std::io::Write;
use std::os::unix::io::RawFd;
use std::sync::Mutex;
use libc::{c_void, size_t, ssize_t, dlsym, RTLD_NEXT};
use lazy_static::lazy_static;
use chrono::Local;
use std::env;
use std::sync::Once;

const STDOUT_FILENO: RawFd = 1;
const STDERR_FILENO: RawFd = 2;
const TITLE_LOG_FILE: &str = "/tmp/catnip_syscall_titles.log";

type WriteFn = unsafe extern "C" fn(RawFd, *const c_void, size_t) -> ssize_t;

lazy_static! {
    static ref LOG_MUTEX: Mutex<()> = Mutex::new(());
    static ref ORIGINAL_WRITE: WriteFn = unsafe { init_original_write() };
}

static INIT: Once = Once::new();

unsafe fn init_original_write() -> WriteFn {
    let write_ptr = dlsym(RTLD_NEXT, b"write\0".as_ptr() as *const libc::c_char);
    if write_ptr.is_null() {
        panic!("Failed to get original write function");
    }
    std::mem::transmute(write_ptr)
}

#[no_mangle]
pub unsafe extern "C" fn write(fd: RawFd, buf: *const c_void, count: size_t) -> ssize_t {
    // Ensure we have the original function
    let original = *ORIGINAL_WRITE;
    
    // Call the original write function first
    let result = original(fd, buf, count);
    
    // Only scan stdout and stderr for title sequences
    if result > 0 && (fd == STDOUT_FILENO || fd == STDERR_FILENO) && !buf.is_null() && count > 0 {
        if let Ok(enabled) = env::var("CATNIP_TITLE_INTERCEPT") {
            if enabled == "1" {
                let data = std::slice::from_raw_parts(buf as *const u8, result as usize);
                scan_for_title_sequences(data);
            }
        }
    }
    
    result
}

fn get_current_working_directory() -> String {
    match env::current_dir() {
        Ok(path) => path.to_string_lossy().to_string(),
        Err(_) => {
            // Fallback: try reading /proc/self/cwd
            match std::fs::read_link("/proc/self/cwd") {
                Ok(path) => path.to_string_lossy().to_string(),
                Err(_) => "/unknown".to_string(),
            }
        }
    }
}

fn scan_for_title_sequences(data: &[u8]) {
    let mut i = 0;
    while i < data.len().saturating_sub(4) {
        // Look for OSC sequences: \x1b]0; or \x1b]2;
        if data[i] == 0x1b && data[i + 1] == b']' && 
           (data[i + 2] == b'0' || data[i + 2] == b'2') && 
           data[i + 3] == b';' {
            
            // Found start of title sequence
            let title_start = i + 4;
            let mut title_end = title_start;
            
            // Find the terminator (\x07 or \x1b\\)
            while title_end < data.len() {
                if data[title_end] == 0x07 {
                    // Bell terminator found
                    break;
                } else if title_end < data.len() - 1 && 
                          data[title_end] == 0x1b && 
                          data[title_end + 1] == b'\\' {
                    // ESC backslash terminator found
                    break;
                }
                title_end += 1;
            }
            
            // Extract and log the title if we found a complete sequence
            if title_end < data.len() && title_end > title_start {
                let title_slice = &data[title_start..title_end];
                
                // Limit title length for safety
                let title_len = title_slice.len().min(200);
                let title_slice = &title_slice[..title_len];
                
                // Validate title is valid UTF-8 and not empty
                if !title_slice.is_empty() {
                    if let Ok(title) = std::str::from_utf8(title_slice) {
                        // Additional validation: ensure it's not just whitespace
                        if !title.trim().is_empty() {
                            log_title(title);
                        }
                    }
                }
            }
            
            // Move past the processed sequence
            i = title_end;
        }
        i += 1;
    }
}

fn log_title(title: &str) {
    let _guard = LOG_MUTEX.lock().unwrap();
    
    let timestamp = Local::now().format("%Y-%m-%d %H:%M:%S").to_string();
    let pid = std::process::id();
    let cwd = get_current_working_directory();
    
    let log_entry = format!("{}|{}|{}|{}\n", timestamp, pid, cwd, title);
    
    // Append to log file
    if let Ok(mut file) = OpenOptions::new()
        .create(true)
        .append(true)
        .open(TITLE_LOG_FILE) {
        let _ = file.write_all(log_entry.as_bytes());
    }
}

// Constructor function - runs when library is loaded
#[ctor::ctor]
fn init() {
    // Initialize the original write function pointer
    INIT.call_once(|| {
        let _ = *ORIGINAL_WRITE;
    });
}