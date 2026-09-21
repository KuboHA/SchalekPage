#!/usr/bin/env bash
#
# Build script for SchalekPage.
#
# Usage: ./build.sh [command]
#
#   build          compile the binary for the host platform (default)
#   run [args...]  build, then run it (e.g. ./build.sh run -addr :8080)
#   test           run the test suite
#   check          gofmt + go vet + tests; what CI should run
#   fmt            rewrite sources with gofmt
#   dist           cross-compile release binaries into dist/
#   clean          remove build output
#   help           show this message
#
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

BINARY=schalekpage
PKG=./cmd/schalekpage
DIST=dist

# Version is the current git tag/commit, or "dev" outside a git checkout.
version() {
	git describe --tags --always --dirty 2>/dev/null || echo dev
}

ldflags() {
	echo "-s -w -X main.version=$(version)"
}

# Platforms built by `dist`, as GOOS/GOARCH pairs.
PLATFORMS=(
	linux/amd64
	linux/arm64
	darwin/amd64
	darwin/arm64
	windows/amd64
)

require_go() {
	if ! command -v go >/dev/null 2>&1; then
		echo "error: go is not installed or not on PATH" >&2
		exit 1
	fi
}

cmd_build() {
	require_go
	echo ">> building $BINARY $(version)"
	go build -trimpath -ldflags "$(ldflags)" -o "$BINARY" "$PKG"
	echo ">> wrote ./$BINARY"
}

cmd_run() {
	cmd_build
	echo ">> running ./$BINARY $*"
	exec "./$BINARY" "$@"
}

cmd_test() {
	require_go
	echo ">> running tests"
	go test ./...
}

cmd_fmt() {
	require_go
	echo ">> formatting"
	gofmt -w .
}

cmd_check() {
	require_go

	echo ">> gofmt"
	# gofmt -l prints files needing formatting; any output is a failure.
	local unformatted
	unformatted="$(gofmt -l .)"
	if [ -n "$unformatted" ]; then
		echo "error: these files need gofmt:" >&2
		echo "$unformatted" >&2
		exit 1
	fi

	echo ">> go vet"
	go vet ./...

	echo ">> tests"
	go test ./...

	echo ">> all checks passed"
}

cmd_dist() {
	require_go
	rm -rf "$DIST"
	mkdir -p "$DIST"

	local v
	v="$(version)"

	for platform in "${PLATFORMS[@]}"; do
		local goos="${platform%/*}"
		local goarch="${platform#*/}"
		local out="$DIST/${BINARY}_${v}_${goos}_${goarch}"
		[ "$goos" = windows ] && out="$out.exe"

		echo ">> building $goos/$goarch"
		# CGO is off so each binary is fully static and needs no libc at runtime.
		CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
			go build -trimpath -ldflags "$(ldflags)" -o "$out" "$PKG"
	done

	echo ">> release binaries in $DIST/"
	ls -1 "$DIST"
}

cmd_clean() {
	echo ">> cleaning"
	rm -rf "$BINARY" "$DIST"
}

cmd_help() {
	sed -n '3,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

main() {
	local command="${1:-build}"
	[ $# -gt 0 ] && shift

	case "$command" in
	build) cmd_build "$@" ;;
	run) cmd_run "$@" ;;
	test) cmd_test "$@" ;;
	fmt) cmd_fmt "$@" ;;
	check) cmd_check "$@" ;;
	dist) cmd_dist "$@" ;;
	clean) cmd_clean "$@" ;;
	help | -h | --help) cmd_help ;;
	*)
		echo "error: unknown command '$command'" >&2
		echo >&2
		cmd_help >&2
		exit 1
		;;
	esac
}

main "$@"
