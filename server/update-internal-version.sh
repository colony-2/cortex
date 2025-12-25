#!/bin/bash
set -e

# Configuration
ORG_PATH="github.com/colony-2/colony2"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Detect OS for sed compatibility
case "$OSTYPE" in
  darwin*)  SED_CMD=(sed -i '') ;;
  *)        SED_CMD=(sed -i) ;;
esac

echo "🔍 Querying moon for Go projects in topological order..."

# Robust JQ query:
# 1. Access graph nodes/edges safely with // []
# 2. Topologically sort (dependencies first)
# 3. Filter for Go projects
PROJECT_PATHS=$(
  cd "$ROOT_DIR" && moon project-graph --json | jq -r '
    def topo($nodes; $edges):
      ($edges | map({from: .[1], to: .[0]})) as $rev
      | ($nodes | length) as $n
      | (reduce range(0;$n) as $i ({}; . + {($i|tostring): []})) as $adj0
      | (reduce $rev[] as $e ($adj0; .[$e.from|tostring] += [$e.to])) as $adj
      | (reduce range(0;$n) as $i ({}; . + {($i|tostring): 0})) as $indeg0
      | (reduce $rev[] as $e ($indeg0; .[$e.to|tostring] += 1)) as $indeg
      | [range(0;$n) | select($indeg[(.|tostring)] == 0)] as $queue
      | def step($queue; $indeg; $adj; $out):
          if ($queue|length) == 0 then $out
          else
            ($queue[0]) as $u
            | ($queue[1:]) as $rest
            | ($adj[$u|tostring]) as $nbrs
            | (reduce $nbrs[] as $v ($indeg; .[$v|tostring] -= 1)) as $indeg2
            | (reduce $nbrs[] as $v ($rest; if $indeg2[$v|tostring] == 0 then . + [$v] else . end)) as $queue2
            | step($queue2; $indeg2; $adj; ($out + [$u]))
          end;
        step($queue; $indeg; $adj; []);
    ( .graph.nodes // [] ) as $n
    | ( .graph.edges // [] ) as $e
    | topo($n; $e)
    | .[] as $i
    | $n[$i]
    | select(.language == "go")
    | .source
  '
)

if [ -z "$PROJECT_PATHS" ]; then
    echo "❌ No Go projects found or moon project-graph returned no Go nodes."
    exit 1
fi

NEW_VERSION=""

# Process modules starting from the "leaf" (dependencies) up to the "root" (apps)
for DIR in $PROJECT_PATHS; do
    MODULE_DIR="$ROOT_DIR/$DIR"
    if [ ! -f "$MODULE_DIR/go.mod" ]; then continue; fi
    
    echo "--- Checking $DIR ---"
    pushd "$MODULE_DIR" > /dev/null

    # 1. Check for internal colony-2 dependencies
    INTERNAL_DEPS=$(
        awk -v org="$ORG_PATH" '
          $0 ~ org && $0 !~ "^module[[:space:]]+"org {
            for (i=1;i<=NF;i++) if ($i ~ org) print $i
          }
        ' go.mod | sort -u
    )

    if [ -n "$INTERNAL_DEPS" ]; then
        # If we haven't found the new version yet, trigger an update to get it
        if [ -z "$NEW_VERSION" ]; then
            TARGET_MOD=$(echo "$INTERNAL_DEPS" | awk '{print $1}' | head -n 1)
            echo "📦 Updating $TARGET_MOD to @latest to find new version..."
            
            # Fetch the latest commit-based pseudo-version without mutating go.mod
            NEW_VERSION=$(
                go list -m -json "$TARGET_MOD@latest" | jq -r '.Version'
            )
        fi

        if [ -n "$NEW_VERSION" ]; then
            popd > /dev/null
            break
        fi
    fi
    popd > /dev/null
done

# 2. Final Global Sweep
# Ensure every single Go module in the repo is updated to the found version
if [ -n "$NEW_VERSION" ]; then
    echo "✅ Finalizing global sync to $NEW_VERSION..."
    find "$ROOT_DIR" -name go.mod -type f -print0 | while IFS= read -r -d '' MODFILE; do
        "${SED_CMD[@]}" -E "s|($ORG_PATH/[a-zA-Z0-9./_-]+)[[:space:]]+v[0-9a-z.-]+|\1 $NEW_VERSION|g" "$MODFILE"
    done
    echo "🧹 Running moon :tidy for all Go modules..."
    (cd "$ROOT_DIR" && moon run :tidy)
    echo "✨ All Go modules synchronized successfully."
else
    echo "No internal dependency updates were required."
fi
