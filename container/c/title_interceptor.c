#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/types.h>
#include <time.h>
#include <fcntl.h>
#include <errno.h>
#include <pthread.h>
#include <limits.h>

// Function pointer for the original write function
static ssize_t (*original_write)(int fd, const void *buf, size_t count) = NULL;

// Thread-safe initialization
static pthread_once_t init_once = PTHREAD_ONCE_INIT;

// Log file for title sequences
static const char* TITLE_LOG_FILE = "/tmp/catnip_syscall_titles.log";

// Initialize the original write function pointer
static void init_original_write() {
    original_write = (ssize_t (*)(int, const void*, size_t))dlsym(RTLD_NEXT, "write");
    if (!original_write) {
        fprintf(stderr, "title_interceptor: Failed to get original write function\n");
        exit(1);
    }
}

// Get the current working directory for a process (this process)
static int get_current_working_directory(char *buffer, size_t buffer_size) {
    if (getcwd(buffer, buffer_size) != NULL) {
        return 1; // Success
    }
    
    // If getcwd failed, try reading from /proc/self/cwd as fallback
    ssize_t len = readlink("/proc/self/cwd", buffer, buffer_size - 1);
    if (len != -1) {
        buffer[len] = '\0';
        return 1; // Success
    }
    
    // If both failed, use a fallback
    strncpy(buffer, "/unknown", buffer_size - 1);
    buffer[buffer_size - 1] = '\0';
    return 0; // Failed but have fallback
}

// Scan buffer for terminal title escape sequences
static void scan_for_title_sequences(const void *buf, size_t count, pid_t pid) {
    const unsigned char *data = (const unsigned char *)buf;
    
    // Look for OSC sequences: \x1b]0; or \x1b]2;
    for (size_t i = 0; i < count - 4; i++) {
        if (data[i] == 0x1b && data[i + 1] == ']' && 
            (data[i + 2] == '0' || data[i + 2] == '2') && 
            data[i + 3] == ';') {
            
            // Found start of title sequence, look for terminator
            size_t title_start = i + 4;
            size_t title_end = title_start;
            
            // Find the terminator (\x07 or \x1b\\)
            while (title_end < count) {
                if (data[title_end] == 0x07) {
                    // Bell terminator found
                    break;
                } else if (title_end < count - 1 && 
                          data[title_end] == 0x1b && 
                          data[title_end + 1] == '\\') {
                    // ESC backslash terminator found
                    break;
                }
                title_end++;
            }
            
            // Extract and log the title if we found a complete sequence
            if (title_end < count && title_end > title_start) {
                size_t title_len = title_end - title_start;
                
                // Limit title length for safety
                if (title_len > 200) {
                    title_len = 200;
                }
                
                char title[256];
                memcpy(title, data + title_start, title_len);
                title[title_len] = '\0';
                
                // Validate title contains only printable characters
                int valid = 1;
                for (size_t j = 0; j < title_len; j++) {
                    if (title[j] < 32 || title[j] > 126) {
                        if (title[j] != ' ') {  // Allow spaces
                            valid = 0;
                            break;
                        }
                    }
                }
                
                if (valid && title_len > 0) {
                    // Get the current working directory
                    char cwd[PATH_MAX];
                    get_current_working_directory(cwd, sizeof(cwd));
                    
                    // Log the title with timestamp, PID, working directory, and title
                    time_t now = time(NULL);
                    struct tm *tm_info = localtime(&now);
                    char timestamp[64];
                    strftime(timestamp, sizeof(timestamp), "%Y-%m-%d %H:%M:%S", tm_info);
                    
                    // Open log file in append mode
                    int log_fd = open(TITLE_LOG_FILE, O_WRONLY | O_CREAT | O_APPEND, 0644);
                    if (log_fd >= 0) {
                        char log_entry[1024]; // Increased size for working directory
                        int log_len = snprintf(log_entry, sizeof(log_entry), 
                                             "%s|%d|%s|%s\n", timestamp, pid, cwd, title);
                        
                        if (log_len > 0 && log_len < (int)sizeof(log_entry)) {
                            // Use original_write to avoid recursion
                            original_write(log_fd, log_entry, log_len);
                        }
                        close(log_fd);
                    }
                }
            }
        }
    }
}

// Intercepted write function
ssize_t write(int fd, const void *buf, size_t count) {
    // Initialize the original function pointer once
    pthread_once(&init_once, init_original_write);
    
    // Call the original write function first
    ssize_t result = original_write(fd, buf, count);
    
    // Only scan stdout and stderr for title sequences
    if ((fd == STDOUT_FILENO || fd == STDERR_FILENO) && 
        buf != NULL && count > 0 && result > 0) {
        
        // Check if title interception is enabled
        const char *enabled = getenv("CATNIP_TITLE_INTERCEPT");
        if (enabled && strcmp(enabled, "1") == 0) {
            // Get current process ID
            pid_t pid = getpid();
            
            // Scan for title sequences (only scan the actually written bytes)
            scan_for_title_sequences(buf, (size_t)result, pid);
        }
    }
    
    return result;
}

// Constructor function to initialize when library is loaded
__attribute__((constructor))
static void init_title_interceptor() {
    // Check if title interception is enabled
    const char *enabled = getenv("CATNIP_TITLE_INTERCEPT");
    if (!enabled || strcmp(enabled, "1") != 0) {
        return;  // Not enabled, don't do anything
    }
    
    // Initialize original write function
    pthread_once(&init_once, init_original_write);
}