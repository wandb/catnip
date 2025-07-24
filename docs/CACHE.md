# Cache Directory Management

This document outlines the cache directory structure and management for package managers in the catnip container.

## Current Cache Locations

Based on analysis of the container environment, package managers currently use these cache directories:

### Go

- Build cache: `/root/.cache/go-build` (GOCACHE)
- Module cache: `/opt/catnip/go-workspace/pkg/mod` (GOMODCACHE)

### Node/NPM

- NPM cache: `/root/.npm`
- PNPM store: `/root/.local/share/pnpm/store/v10`

### Python

- UV cache: `/root/.cache/uv`
- Pip cache: `/root/.cache/pip`

### Rust

- Cargo cache: `/opt/catnip/cargo` (already under /opt/catnip)
- Rustup cache: `/opt/catnip/rustup` (already under /opt/catnip)

## Proposed Cache Directory Structure

Consolidate all package manager caches under `/opt/catnip/cache/`:

```
/opt/catnip/cache/
├── go-build/     # Go build cache (GOCACHE)
├── go-mod/       # Go module cache (GOMODCACHE)
├── npm/          # NPM cache
├── pnpm/         # PNPM store
├── uv/           # UV cache
├── pip/          # Pip cache
├── cargo/        # Cargo cache (moved from /opt/catnip/cargo)
└── rustup/       # Rustup cache (moved from /opt/catnip/rustup)
```

## Environment Variables to Update

The following environment variables need to be updated in the Dockerfile and `catnip-profile.sh`:

- `GOCACHE=/opt/catnip/cache/go-build`
- `GOMODCACHE=/opt/catnip/cache/go-mod`
- `CARGO_HOME=/opt/catnip/cache/cargo`
- `RUSTUP_HOME=/opt/catnip/cache/rustup`
- `XDG_CACHE_HOME=/opt/catnip/cache` (for uv, pip)
- `npm_config_cache=/opt/catnip/cache/npm`
- `PNPM_HOME=/opt/catnip/cache/pnpm`

## Configuration Changes Required

### NPM

Set cache directory via: `npm config set cache /opt/catnip/cache/npm`

### PNPM

Set store directory via: `pnpm config set store-dir /opt/catnip/cache/pnpm`

### UV and Pip

Will use `XDG_CACHE_HOME` environment variable automatically

## Implementation Steps

1. **Create cache directory structure** in Dockerfile:

   ```dockerfile
   RUN mkdir -p ${CATNIP_ROOT}/cache/{go-build,go-mod,npm,pnpm,uv,pip,cargo,rustup} && \
       chown -R catnip:catnip ${CATNIP_ROOT}/cache
   ```

2. **Update environment variables** in Dockerfile and setup scripts

3. **Move existing cache setups** to new locations during container build

4. **Configure package managers** to use new cache locations

5. **Update permissions** to ensure catnip user can access all cache directories

6. **Test each package manager** works correctly with new cache locations

## Benefits

- **Centralized cache management**: All caches in one location
- **Easier backup/restore**: Single directory to manage
- **Cleaner separation**: Removes caches from user home directories
- **Consistent structure**: Aligns with existing `/opt/catnip` organization
- **Better permissions**: Unified ownership under catnip user

## Migration Notes

When implementing these changes:

1. Ensure all cache directories are created with proper permissions
2. Test that package managers can read/write to new locations
3. Verify that existing functionality is not broken
4. Consider adding symbolic links for backward compatibility if needed
5. Update any documentation that references old cache locations
