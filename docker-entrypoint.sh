#!/bin/sh
set -e

# Ensure the directory for the SQLite database exists
if [ -n "$SQLITE_PATH" ]; then
    mkdir -p "$(dirname "$SQLITE_PATH")"
fi

case "${1:-bot}" in
    bot)
        shift 2>/dev/null || true
        exec digikeeper-bot "$@"
        ;;
    sh|bash)
        exec "$@"
        ;;
    *)
        echo "Unknown command: $1" >&2
        echo "Usage: bot | sh | bash" >&2
        exit 1
        ;;
esac
