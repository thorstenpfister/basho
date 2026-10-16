#!/bin/sh
# Writes the shell completions and the man page for the release archives
# (GoReleaser's before hook, SPEC §14.2). Both come from Fang/Cobra.
set -e
rm -rf completions manpages
mkdir -p completions manpages
for sh in bash zsh fish; do
	go run ./cmd/basho completion "$sh" >"completions/basho.$sh"
done
go run ./cmd/basho man | gzip -c -9 >manpages/basho.1.gz
