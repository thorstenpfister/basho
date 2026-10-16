#!/bin/sh
# Writes the shell completions and the man page for the release archives
# (GoReleaser's before hook, SPEC §14.2). Both come from Fang/Cobra.
set -e
rm -rf completions manpages
mkdir -p completions manpages
for sh in bash zsh fish; do
	go run ./cmd/basho completion "$sh" >"completions/basho.$sh"
done
# The man page's date is the commit's, not today's (mango uses time.Now), so
# the archives stay reproducible.
day=$(git log -1 --format=%cs)
go run ./cmd/basho man | sed "1s/\"[0-9-]*\"/\"$day\"/" | gzip -c -9 -n >manpages/basho.1.gz
