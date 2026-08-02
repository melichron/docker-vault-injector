#!/usr/bin/env bash
set -euo pipefail

# Generate one Markdown file containing the repository layout and the complete
# contents of its source files. This is useful when the whole project needs to
# be handed to a person or an agent as a single document.
#
# Configuration can be overridden through environment variables:
#
#   OUT_FILE=tmp/project.md TITLE="Project snapshot" ./gen-repo-snapshot.sh
#
# OUT_FILE is resolved relative to the repository root unless it is absolute.
OUT_FILE="${OUT_FILE:-REPO_SNAPSHOT.md}"
TITLE="${TITLE:-Repository Snapshot}"
DATE_STR="$(date -u +'%Y-%m-%d UTC')"

have_cmd() {
    command -v "$1" >/dev/null 2>&1
}

# Work from the repository root so the result does not depend on the directory
# from which the script was invoked.
SCRIPT_ABS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
if have_cmd git && git -C "$(dirname "$SCRIPT_ABS")" rev-parse --show-toplevel >/dev/null 2>&1; then
    REPO_ROOT="$(git -C "$(dirname "$SCRIPT_ABS")" rev-parse --show-toplevel)"
else
    REPO_ROOT="$(dirname "$SCRIPT_ABS")"
fi
cd "$REPO_ROOT"

case "$OUT_FILE" in
/*) OUT_ABS="$OUT_FILE" ;;
*) OUT_ABS="$REPO_ROOT/$OUT_FILE" ;;
esac

# The temporary output lives outside the repository. Consequently it cannot
# accidentally become one of the files included in the snapshot.
TMP_OUTPUT="$(mktemp)"
CANDIDATES="$(mktemp)"
cleanup() {
    rm -f "$TMP_OUTPUT" "$CANDIDATES"
}
trap cleanup EXIT

# Capture every text-based project file outside temporary and implementation
# artefact directories, including source code, configuration, Markdown, and
# everything textual under examples/. Binary files cannot be represented safely
# inside Markdown and are skipped. The two explicit path checks prevent
# recursive snapshots and prevent this generator from embedding itself.
find "$REPO_ROOT" \
    -type d \( \
        -name .git -o \
        -name tmp -o \
        -name vendor -o \
        -name build -o \
        -name bin -o \
        -name .cache \
    \) -prune -o \
    -type f -print |
while IFS= read -r absolute_path; do
    [[ "$absolute_path" == "$SCRIPT_ABS" ]] && continue
    [[ "$absolute_path" == "$OUT_ABS" ]] && continue
    [[ "$absolute_path" == "$REPO_ROOT/REPO_SNAPSHOT.md" ]] && continue
    [[ ! -s "$absolute_path" ]] || LC_ALL=C grep -Iq . "$absolute_path" || continue
    printf '%s\n' "${absolute_path#"$REPO_ROOT/"}"
done | sort >"$CANDIDATES"

language_for() {
    case "$1" in
        *.go) echo go ;;
        *.md) echo markdown ;;
        *.yaml | *.yml) echo yaml ;;
        *.json) echo json ;;
        *.toml) echo toml ;;
        *.hcl) echo hcl ;;
        *.sh | *.bash) echo bash ;;
        *.Dockerfile | Dockerfile) echo dockerfile ;;
        Makefile | makefile | GNUmakefile) echo make ;;
        go.mod | go.sum) echo text ;;
        *) echo text ;;
    esac
}

# Pick a backtick fence which does not occur in the file. This keeps embedded
# Markdown readable even when README examples contain their own fenced blocks.
fence_for() {
    local file="$1"
    local fence='```'
    while grep -Fq -- "$fence" "$file"; do
        fence="${fence}\`"
    done
    printf '%s' "$fence"
}

{
    printf '# %s\n\n' "$TITLE"
    printf 'Generated: %s\n\n' "$DATE_STR"
    printf '## Included files\n\n'
    printf '```text\n'
    sed 's#^#./#' "$CANDIDATES"
    printf '```\n\n'
    printf '## File contents\n\n'
} >"$TMP_OUTPUT"

while IFS= read -r file; do
    [[ -f "$file" ]] || continue

    language="$(language_for "$file")"
    fence="$(fence_for "$file")"
    {
        printf '### `%s`\n\n' "$file"
        printf '%s%s\n' "$fence" "$language"
        cat "$file"

        # Ensure the closing fence starts on a new line even if the source file
        # has no trailing newline.
        if [[ -s "$file" ]] && [[ "$(tail -c 1 "$file" | wc -l | tr -d ' ')" == "0" ]]; then
            printf '\n'
        fi
        printf '%s\n\n' "$fence"
    } >>"$TMP_OUTPUT"
done <"$CANDIDATES"

mkdir -p "$(dirname "$OUT_ABS")"
mv "$TMP_OUTPUT" "$OUT_ABS"
printf 'Wrote %s\n' "$OUT_FILE"
