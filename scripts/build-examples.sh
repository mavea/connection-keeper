#!/usr/bin/env bash
set -u

project_dir="${1:?project_dir is required}"

cd "$project_dir"

# Disable VCS stamping to avoid git dubious ownership issues in containers
export GOFLAGS=-buildvcs=false

echo "Building examples..."
failed=0

# Use process substitution to avoid subshell issues with variable scoping
while IFS= read -r -d '' main_file; do
	example_dir=$(dirname "$main_file")
	echo "Building $example_dir..."
	if (cd "$example_dir" && go build -o /tmp/example-build-test .); then
		echo "  ✓ $example_dir compiled successfully"
	else
		echo "  ✗ $example_dir FAILED"
		failed=1
	fi
done < <(find example -name "main.go" -type f -print0)

if [ $failed -eq 0 ]; then
	echo ""
	echo "All examples compiled successfully!"
	exit 0
else
	echo ""
	echo "Some examples failed to compile"
	exit 1
fi






