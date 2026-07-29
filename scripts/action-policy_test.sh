#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ACTION="$ROOT/action.yml"

assert_contains() {
  local needle=$1 description=$2
  if ! grep -Fq -- "$needle" "$ACTION"; then
    echo "FAIL: $description" >&2
    exit 1
  fi
  echo "PASS: $description"
}

assert_contains "policy-file:" "Action exposes policy-file input"
assert_contains 'POLICY_ARGS+=(--policy-file "$GITHUB_WORKSPACE/$CLOUDTAB_POLICY_FILE")' "Policy path is repository-relative and safely quoted"
assert_contains 'if [ "$status" -ne 0 ] && [ "$status" -ne 2 ]; then' "Operational failures stop before comment posting"
assert_contains 'echo "exit_code=$status" >> "$GITHUB_OUTPUT"' "Policy exit code is preserved"
assert_contains "if: steps.run.outputs.exit_code == '2'" "Policy failure is enforced after the comment step"

post_line=$(grep -n -- "- name: Post PR comment" "$ACTION" | cut -d: -f1)
enforce_line=$(grep -n -- "- name: Enforce cloudtab cost policy" "$ACTION" | cut -d: -f1)
if [[ -z "$post_line" || -z "$enforce_line" || "$post_line" -ge "$enforce_line" ]]; then
  echo "FAIL: PR comment must be posted before policy failure is enforced" >&2
  exit 1
fi
echo "PASS: PR comment precedes policy enforcement"

echo "All Action policy tests passed"
