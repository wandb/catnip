#!/usr/bin/env bash
# Legacy wrapper for backwards compatibility with older extensions
# This script is maintained for compatibility purposes only
# New code should use catnip-start.sh directly

exec "$(dirname "$0")/catnip-start.sh" "$@"
