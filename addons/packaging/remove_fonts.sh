#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# remove font-face CSS statement
sed -i '/@font-face/,/}/d' "${PROJECT_ROOT}/internal/web/static/style.css"

if [ -e "${PROJECT_ROOT}/internal/web/static/game.css" ]; then
    sed -i '/@font-face/,/}/d' "${PROJECT_ROOT}"/internal/web/static/game.css
fi

# remove fonts
rm -rf "${PROJECT_ROOT}"/internal/web/static/fonts
