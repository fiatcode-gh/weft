#!/bin/sh
# fake-editor.sh — test fixture for the editor hand-off.
# argv: $1 = file path, $2 = action
#
# Actions:
#   noop    exit 0, do nothing
#   touch   bump mtime only (no content change)
#   append  append "edited" to the file, bump mtime
#   delete  rm the file
#   fail    exit 7
set -e
case "$2" in
  noop)    : ;;
  touch)   touch "$1" ;;
  append)  echo "edited" >> "$1" ;;
  delete)  rm "$1" ;;
  fail)    exit 7 ;;
  *)       echo "fake-editor: unknown action $2" >&2; exit 2 ;;
esac
