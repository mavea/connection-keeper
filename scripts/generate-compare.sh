#!/usr/bin/env bash
set -euo pipefail

project_dir="${1:?project_dir is required}"
plant_dir="${2:?plant_dir is required}"
plant_out_dir="${3:?plant_out_dir is required}"

git config --global --add safe.directory "$project_dir" 2>/dev/null || true

# Re-generate diagrams directly into the working tree (same as `make generate`).
for fmt in svg png; do
	find "$plant_dir" -type f -name '*.plant' | while IFS= read -r file; do
		rel_path="${file#"$plant_dir"/}"
		rel_dir="$(dirname "$rel_path")"
		mkdir -p "$plant_out_dir/$fmt/$rel_dir"
		java -jar /opt/plantuml.jar -t"$fmt" -output "$plant_out_dir/$fmt/$rel_dir" "$file"
	done
done

# Report files that differ from HEAD and exit 1 if any are found.
diff_output="$(git -C "$project_dir" diff --name-only -- "$plant_out_dir" || true)"
if [ -z "$diff_output" ]; then
	exit 0
fi

# Strip the project-relative prefix to show paths relative to plant_out_dir.
printf '%s\n' "$diff_output" | sed "s|^$plant_out_dir/||" | sort -u
exit 1
