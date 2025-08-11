# Tauri Desktop App Setup

This document outlines the Tauri desktop application setup for Catnip.

## Prerequisites

### System Dependencies (Linux/Ubuntu)

Before building the Tauri application, install the required system dependencies:

```bash
sudo apt update
sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev libappindicator3-dev librsvg2-dev patchelf libglib2.0-dev libgdk-pixbuf-2.0-dev libatk1.0-dev libcairo-gobject2 libpango1.0-dev libgdk-pixbuf2.0-dev libsoup-3.0-dev libjavascriptcoregtk-4.0-dev
```

### Rust

Install Rust if you haven't already:

```bash
curl --proto '=https' --tlsv1.2 https://sh.rustup.rs -sSf | sh
source $HOME/.cargo/env
```

## Available Scripts

- `pnpm tauri:dev` - Start development server with hot reload
- `pnpm tauri:build` - Build production executable
- `pnpm tauri` - Access Tauri CLI directly

## Configuration

The Tauri configuration is located in:

- `src-tauri/tauri.conf.json` - Main configuration file
- `src-tauri/capabilities/default.json` - App permissions and capabilities
- `vite.config.ts` - Updated to support Tauri development

## App Configuration

Current window settings:

- **Size**: 1400x900 (min: 800x600)
- **Title**: "Catnip - Agentic Coding Environment"
- **App ID**: com.catnip.app
- **Resizable**: Yes
- **Centered**: Yes

## Development

The Tauri app will automatically:

1. Start the Vite dev server (`pnpm dev`)
2. Build and run the Rust backend
3. Open a native window displaying your React SPA

The dev server is configured to work with both container development and Tauri mobile development via the `TAURI_DEV_HOST` environment variable.

## Building

To build the desktop application:

```bash
pnpm tauri:build
```

This will create platform-specific binaries in `src-tauri/target/release/bundle/`.

## Notes

- The frontend assets are built to `../dist` relative to the `src-tauri` directory
- Hot reload works for both frontend (React/Vite) and backend (Rust) code
- The app uses the same React SPA that runs in the browser, providing a consistent experience
