# Rust Title Interceptor

A safe, modern Rust implementation of the terminal title interceptor library that was originally written in C.

## Features

- Intercepts `write` system calls to stdout/stderr
- Detects and logs terminal title escape sequences (OSC 0 and OSC 2)
- Thread-safe logging with mutex protection
- Zero-copy scanning for better performance
- Memory-safe implementation using Rust's safety guarantees

## Building

### For Linux (in container):

```bash
docker exec catnip-test bash -c 'cd /live/catnip/container/rust-title-interceptor && cargo build --release'
```

### Using Make:

```bash
make docker-build
```

## Usage

Set the environment variable `CATNIP_TITLE_INTERCEPT=1` and preload the library:

```bash
CATNIP_TITLE_INTERCEPT=1 LD_PRELOAD=/path/to/libtitle_interceptor.so your_program
```

## Log Format

Title sequences are logged to `/tmp/catnip_syscall_titles.log` in the format:

```
timestamp|pid|working_directory|title
```

Example:

```
2025-07-27 17:57:20|234188|/live/catnip/container/rust-title-interceptor|Test Title
```

## Differences from C Implementation

1. **Memory Safety**: No manual memory management, preventing buffer overflows
2. **Simplified Dependencies**: Uses `redhook` for cleaner function interception
3. **Better Error Handling**: Rust's Result types for safer error handling
4. **Modern Date/Time**: Uses `chrono` crate instead of manual time formatting

## Testing

The library includes comprehensive TypeScript tests to verify functionality with Node.js applications:

```bash
# Run tests (requires Docker)
make test

# Or run tests manually:
cd /path/to/rust-title-interceptor
npm install
npm test
```

### Test Coverage

- Basic stdout/stderr writes
- Console.log output
- Buffer writes
- Unicode/emoji support in titles
- Terminal UI framework patterns
- Multiple titles in sequence
