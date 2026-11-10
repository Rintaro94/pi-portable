#!/bin/sh
# Download and install the latest Pi Portable release into this folder.
# Your login, sessions and settings are kept.
exec "$(dirname "$0")/pi" update "$@"
