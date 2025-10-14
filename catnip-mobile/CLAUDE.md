# CLAUDE.md - Catnip Mobile

Supplemental guidance for Anthropic Claude when working on the catnip-mobile Expo app.

## Development Environment

- The dev server (`pnpm start`) is typically already running in a separate terminal
- **DO NOT** start or restart the dev server unless explicitly requested
- Hot reload will pick up most changes automatically

## Design Philosophy

- Target a modern **liquid glass aesthetic** for iOS
  - Translucent backgrounds with blur effects
  - Subtle depth and layering
  - Smooth animations and transitions
  - System-native feel

- **Prefer Expo native functionality** over custom native modules
  - Use Expo SDK packages whenever possible
  - Avoid requiring custom native code that would necessitate building a custom development client
  - Stay within the Expo Go compatibility bounds when feasible

## Technical Stack

- **Framework**: Expo (React Native)
- **Routing**: Expo Router (file-based routing)
- **Package Manager**: pnpm (already configured at workspace root)

## Development Workflow

1. Make changes to TypeScript/React components
2. Hot reload will automatically update the app
3. Test on iOS simulator or physical device via Expo Go
4. Only suggest server restarts if there are configuration or dependency changes

## Key Conventions

- Follow the existing project structure in `app/` for routing
- Keep components modular in `components/`
- Store shared utilities in `lib/`
- Use TypeScript throughout
- Match the visual design patterns established in existing screens

## When In Doubt

- Refer to the root `AGENTS.md` and `CLAUDE.md` for general repository conventions
- Ask the user before making architectural changes or adding new dependencies
