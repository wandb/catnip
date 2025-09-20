# Catnip Mobile App

A modern React Native/Expo mobile application for accessing Catnip workspaces and managing Claude sessions, built with the latest Expo SDK 52 and file-based routing.

## Features

- **Modern Expo SDK 52**: Using the latest and greatest Expo features
- **File-based Routing**: Expo Router for seamless navigation
- **OAuth Relay**: Secure authentication via worker-managed OAuth flow
- **Codespace Access**: Connect to GitHub Codespaces with real-time updates
- **Workspace Management**: View and interact with your Catnip workspaces
- **Claude Integration**: Send prompts and monitor AI task execution
- **Real-time Updates**: Live polling for workspace status changes
- **Native Components**: Uses only Expo SDK components (no custom React Native)

## Tech Stack

- **Expo SDK 52** - Latest Expo platform
- **Expo Router** - File-based routing with typed routes
- **OAuth Relay** - Server-managed authentication via deep links
- **Expo SecureStore** - Secure token storage
- **TypeScript** - Full type safety
- **React Native** - Cross-platform mobile development

## Architecture

The app uses Expo's modern architecture:

- File-based routing with `app/` directory
- No custom React Native dependencies
- All APIs handled via Expo's built-in fetch
- Server-sent events for real-time updates

### API Integration

Connects to Catnip CloudFlare worker at `catnip.run/v1/*`:

- **Authentication**: `GET /v1/auth/github/mobile` (OAuth relay)
- **Codespace Access**: `GET /v1/codespace` (SSE)
- **Workspaces**: `GET /v1/workspaces`
- **Claude Prompts**: `POST /v1/workspaces/:id/prompt`

## Setup

1. **Install dependencies**:

```bash
cd catnip-mobile
export VOLTA_HOME="$HOME/.volta" && export PATH="$VOLTA_HOME/bin:$PATH"
export PNPM_HOME="/home/vscode/.local/share/pnpm" && export PATH="$PNPM_HOME:$PATH"
pnpm install
```

2. **Optional environment variables**:
   Create `.env.local` if using custom API base URL:

```
EXPO_PUBLIC_CATNIP_BASE_URL=https://your-custom-domain.com
```

3. **Start the development server**:

```bash
pnpm start
```

4. **Run on devices**:

- **iOS**: `pnpm run ios` (requires macOS)
- **Android**: `pnpm run android` (requires Android Studio/emulator)
- **Web**: `pnpm run web` (for testing)

## App Structure

```
app/
├── _layout.tsx           # Root layout with navigation
├── index.tsx             # Splash/routing screen
├── auth.tsx              # GitHub OAuth login
├── codespace.tsx         # Codespace connection
├── workspaces.tsx        # Workspace list
└── workspace/
    └── [id].tsx          # Workspace detail/interaction

hooks/
└── useAuth.ts           # Authentication state management

lib/
└── api.ts               # API client for catnip.run
```

## Navigation Flow

1. **Splash** (`index.tsx`) - Routes to auth or codespace based on auth status
2. **Auth** (`auth.tsx`) - OAuth relay login via browser
3. **Codespace** (`codespace.tsx`) - Connect to GitHub Codespace
4. **Workspaces** (`workspaces.tsx`) - List of available workspaces
5. **Workspace Detail** (`workspace/[id].tsx`) - Interact with Claude

## Key Features

### Authentication

- Uses OAuth relay pattern for maximum security
- Mobile app never handles GitHub tokens directly
- Worker manages GitHub OAuth server-side
- Only session tokens stored on mobile device

### Real-time Updates

- Server-sent events for codespace connection
- Polling for workspace status changes
- Live todo list updates during Claude execution

### Modern UI

- Dark theme optimized for mobile
- Gradient buttons and modern styling
- Responsive layouts with SafeAreaView
- Keyboard-aware scrolling

## Development

### Adding New Screens

Create new files in `app/` directory:

- `app/new-screen.tsx` → `/new-screen`
- `app/folder/screen.tsx` → `/folder/screen`
- `app/folder/[param].tsx` → `/folder/:param`

### Environment Variables

Use `EXPO_PUBLIC_` prefix for client-side variables:

```typescript
process.env.EXPO_PUBLIC_CATNIP_BASE_URL; // Optional custom API URL
```

### Building for Production

```bash
# iOS
eas build --platform ios

# Android
eas build --platform android
```

## Future Enhancements

- Push notifications for Claude task completion
- Biometric authentication with Expo LocalAuthentication
- Offline support with Expo SQLite
- Pull request management
- Workspace search and filtering
- Theme customization
