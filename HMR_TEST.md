# Hot Module Replacement (HMR) Test Guide

## 🔥 Testing Vite HMR in Container

The development environment now supports proper Hot Module Replacement for both frontend and backend:

### Frontend HMR Test
1. Start the development environment:
   ```bash
   just run-dev
   ```

2. Open http://localhost:5173 in your browser

3. Edit any file in `src/` (e.g., `src/App.tsx`)

4. **Expected Result**: Browser should update automatically without page refresh
   - Look for `[vite] hot updated` messages in browser console
   - Changes should appear within 1-2 seconds

### Backend Hot Reload Test  
1. With development environment running
   
2. Edit any Go file in `container/` (e.g., `container/cmd/server/main.go`)

3. **Expected Result**: Go server should restart automatically
   - Look for Air reload messages in terminal
   - API should be available without manual restart

### Configuration Details

**Frontend (Vite)**:
- ✅ File polling enabled (`usePolling: true`)
- ✅ WebSocket protocol explicitly set (`protocol: 'ws'`)
- ✅ Container-aware configuration via `CATNIP_DEV=true`

**Backend (Air)**:
- ✅ Watches Go files for changes
- ✅ Auto-restarts server process
- ✅ Preserves development state

### Troubleshooting

If HMR isn't working:

1. **Check WebSocket Connection**: Open browser dev tools → Network → WS tab
   - Should see WebSocket connection to `ws://localhost:5173`

2. **Verify Environment**: In container, `echo $CATNIP_DEV` should output `true`

3. **Rebuild Container**: If you made config changes:
   ```bash
   just rebuild-dev
   just run-dev
   ```

4. **Check File Permissions**: Ensure files are writable in the mounted volume

### Performance Notes

- **Polling Interval**: 1 second (configurable in `vite.config.ts`)
- **File Watching**: Uses polling for Docker compatibility
- **Memory Usage**: Slightly higher due to polling, but negligible for development