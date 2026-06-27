#!/usr/bin/env bash
# radii5 installer - short URL entry point
# Usage: curl -fsSL https://ohcass.github.io/radii5/install.sh | sh
set -euo pipefail
exec bash <(curl -fsSL https://raw.githubusercontent.com/ohcass/radii5/main/scripts/install.sh)
