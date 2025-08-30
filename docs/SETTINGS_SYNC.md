# Settings Synchronization

This document describes how Catnip manages persistent configuration files between the container and the host volume.

## Overview

The settings sync system ensures that important configuration files (Claude credentials, GitHub CLI config, etc.) are persisted across container restarts while being respectful of concurrent file access, particularly for files that Claude Code frequently modifies.

## Architecture

The settings synchronization is implemented in `container/internal/models/settings.go` and follows these principles:

1. **Boot-time Only Restore**: Files are only copied from volume to home directory during container startup
2. **Debounced Volume Sync**: Changes in home directory are synced to volume with intelligent debouncing
3. **Conflict Avoidance**: Special handling for sensitive files that may be concurrently accessed
4. **Atomic Operations**: Critical files use atomic writes to prevent corruption

## File Management

### Monitored Files

The system tracks these configuration files:

| File               | Home Location                 | Volume Location                             | Sensitive |
| ------------------ | ----------------------------- | ------------------------------------------- | --------- |
| Claude credentials | `~/.claude/.credentials.json` | `/volume/.claude/.claude/.credentials.json` | ✅        |
| Claude config      | `~/.claude.json`              | `/volume/.claude/claude.json`               | ✅        |
| GitHub CLI config  | `~/.config/gh/config.yml`     | `/volume/.github/config.yml`                | ❌        |
| GitHub CLI hosts   | `~/.config/gh/hosts.yml`      | `/volume/.github/hosts.yml`                 | ❌        |

### Boot-time Behavior

On container startup:

- Volume directories are created if they don't exist
- **Smart restore logic** for `claude.json`:
  - If home file doesn't exist: copies from volume
  - If both files exist: compares "configuration level" scores
  - Prefers the more configured file (higher score)
  - Creates timestamped backup before overwriting home file
- Other files: copied FROM volume TO home directory ONLY if home file doesn't exist
- No bidirectional comparison for non-Claude files

### Runtime Behavior

During container operation:

- Home directory files are monitored every 5 seconds for changes
- Changes trigger debounced sync operations to volume
- **Content validation**: Claude config files are validated before syncing to prevent unconfigured files from overwriting good volume data
- Regular files: 2-second debounce
- Sensitive files: 5-second debounce
- Multiple rapid changes only result in one sync operation

## Sensitive File Handling

Files marked as "sensitive" (Claude configuration files) receive special treatment:

### Lock File Detection

The system checks for common lock file patterns before syncing:

- `{filename}.lock`
- `{filename}.tmp`
- `{directory}/.lock`
- `{directory}/lock`

### Concurrent Access Protection

- Attempts to open files exclusively to detect active use
- Defers sync operations if file appears to be in use
- Reschedules sync for 10 seconds later if conflict detected

### Atomic Writes

Sensitive files use atomic write operations:

1. Write to temporary file (`{filename}.tmp.{basename}`)
2. Atomically rename temp file to final destination
3. Clean up temp file if operation fails

## Configuration

### Timing Settings

- **Monitor Frequency**: 5 seconds (file change detection)
- **Regular File Debounce**: 2 seconds
- **Sensitive File Debounce**: 5 seconds
- **Conflict Retry Delay**: 10 seconds

### Directory Structure

```
/volume/
├── .claude/
│   ├── .claude/
│   │   └── .credentials.json
│   └── claude.json
└── .github/
    ├── config.yml
    └── hosts.yml
```

## Implementation Details

### Key Components

- **Settings struct**: Main controller with debounce timers and sync mutex
- **restoreFromVolumeOnBoot()**: Smart startup restore with configuration comparison
- **checkAndSyncFiles()**: Periodic change detection with content validation
- **scheduleDebounceSync()**: Debounce timer management
- **performSafeSync()**: Safe sync with conflict detection
- **isFileBeingAccessed()**: Lock file and concurrent access detection
- **copyFileAtomic()**: Atomic write operations
- **shouldPreferVolumeClaudeConfig()**: Configuration level comparison
- **isClaudeConfigValid()**: Content validation to prevent syncing unconfigured files
- **backupHomeFile()**: Creates timestamped backups before overwriting

### Thread Safety

- `syncMutex` protects internal state (lastModTimes, debounceMap)
- All sync operations are serialized
- Proper cleanup of debounce timers on shutdown

## Troubleshooting

### Common Issues

1. **Files not syncing to volume**
   - Check container logs for sync errors
   - Verify volume directory permissions (should be writable by catnip user)
   - Look for lock file conflicts in logs

2. **Boot-time restore not working**
   - Check if volume directories exist and are accessible
   - Verify file ownership (should be catnip:catnip / 1000:1000)
   - Check if home files already exist (restore skips existing files)

3. **Sync delays or conflicts**
   - Look for lock file detection messages in logs
   - Check for concurrent Claude Code operations
   - Monitor debounce timer activity

### Logging

The system provides detailed logging with these prefixes:

- `📥 Boot-time restore`: Startup restore operations
- `📊 Claude config comparison`: Configuration level scores during comparison
- `💾 Created backup`: Timestamped backups before overwriting
- `🚫 Claude config appears unconfigured`: Content validation failures
- `🕒 Recent firstStartTime detected`: Fresh installation detection
- `📋 Synced`: Successful sync to volume
- `🔒 Lock file detected`: Concurrent access prevention
- `⚠️ File ... appears to be in use`: Conflict deferral
- `❌ Failed to sync`: Sync errors

### Manual Operations

If needed, you can manually trigger operations:

```bash
# Check current file states
ls -la ~/.claude.json /volume/.claude/claude.json

# Copy files manually (if sync is stuck)
cp ~/.claude.json /volume/.claude/claude.json

# Check for lock files
find ~/.claude -name "*.lock" -o -name "*.tmp"

# List backup files in volume
ls -la /volume/.claude/*.backup.*
```

## Future Improvements

Potential enhancements to consider:

- File system event-based monitoring (inotify) instead of polling
- Configurable debounce timers
- More sophisticated lock file detection
- Checksum-based change detection
- Volume-to-home sync for specific use cases

## Security Considerations

- All synced files contain sensitive credentials
- Volume directories should have restricted permissions
- Atomic writes prevent partial file corruption
- Lock file detection prevents concurrent access issues
- No network transmission of credentials (local volume only)
