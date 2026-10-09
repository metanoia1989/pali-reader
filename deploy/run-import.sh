#!/usr/bin/env bash
# Runs the corpus importer on the server.
#
#   ./run-import.sh all                 # full rebuild from scratch
#   ./run-import.sh text,entries,ref    # resume from a step
#
# The DSN lives in pali-importer.env next to this script and is sourced rather
# than exported by the caller, so the password never lands in a shell history
# or in `ps` output.
set -euo pipefail
cd "$(dirname "$0")"

steps="${1:-all}"
# -drop wipes the imported tables first. It is the default for a full run and
# off for a resume, so a failed step does not cost the earlier ones.
drop=""
if [ "$steps" = "all" ]; then drop="-drop"; fi

set -a
# shellcheck disable=SC1091
. ./pali-importer.env
set +a

exec ./pali-importer -sources ./sources $drop -steps "$steps"
