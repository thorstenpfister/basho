#!/bin/sh
# Publishes the release of the tag at HEAD (SPEC §14.3): GoReleaser builds
# the archives, creates the GitHub release with the tag's CHANGELOG entry as
# its notes, and commits Casks/basho.rb to main. Run by `make release`.
set -eu

tag=$(git describe --tags --exact-match HEAD 2>/dev/null) || {
	echo "release: HEAD carries no tag; tag it first (docs/releasing.md)" >&2
	exit 1
}
if [ -n "$(git status --porcelain)" ]; then
	echo "release: the working tree is not clean" >&2
	exit 1
fi
if ! git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null; then
	echo "release: $tag is not on origin; git push origin main $tag" >&2
	exit 1
fi

# The CHANGELOG entry "## v1.2.3 (date)", without its heading.
notes=$(mktemp)
trap 'rm -f "$notes"' EXIT
awk -v tag="$tag" '
	/^## / { if (found) exit; if ($2 == tag) { found = 1; next } }
	found
' CHANGELOG.md >"$notes"
if ! grep -q '[^[:space:]]' "$notes"; then
	echo "release: CHANGELOG.md has no entry \"## $tag (…)\"" >&2
	exit 1
fi

GITHUB_TOKEN=$(gh auth token) goreleaser release --clean --release-notes "$notes"
git pull --ff-only origin main # fetch the cask commit GoReleaser made
