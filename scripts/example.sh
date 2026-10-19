#!/bin/sh
# Copies an example project from testdata/examples into a scratch git
# repository with a short history and one uncommitted change, so diff
# review, History, the blast radius and the Tests panel have something to
# show. Usage: scripts/example.sh <name> [destination]
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
examples="$root/testdata/examples"
name=${1:-}
if [ -z "$name" ] || [ ! -d "$examples/$name" ]; then
	echo "usage: $0 <example> [destination]" >&2
	echo "examples:" >&2
	for d in "$examples"/*/; do echo "  $(basename "$d")" >&2; done
	exit 2
fi
dest=${2:-$(mktemp -d)/$name}
if [ -e "$dest" ] && [ -n "$(ls -A "$dest" 2>/dev/null)" ]; then
	echo "$dest already exists and isn't empty" >&2
	exit 1
fi
mkdir -p "$dest"
(cd "$examples/$name" && tar cf - --exclude=.demo .) | (cd "$dest" && tar xf -)

cd "$dest"
git init -q
commit() { git add -A && git -c user.name="CodeDiver Example" -c user.email="example@codediver.app" commit -q -m "$1"; }
# .demo/commits names the two commits, one per line.
first="Initial order service"
second="Reject orders over 1000 items"
if [ -f "$examples/$name/.demo/commits" ]; then
	first=$(sed -n 1p "$examples/$name/.demo/commits")
	second=$(sed -n 2p "$examples/$name/.demo/commits")
fi
git apply -R "$examples/$name/.demo/history.patch"
commit "$first"
git apply "$examples/$name/.demo/history.patch"
commit "$second"
git apply "$examples/$name/.demo/change.patch"

echo "Example $name is ready in $dest"
echo "  2 commits, plus an uncommitted change:"
git status --short | sed 's/^/   /'
echo "  Open it in CodeDiver:  codediver --add $dest   (or: codediver serve --add $dest)"
