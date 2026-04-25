#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

cat >"${TMPDIR}/kubectl" <<'EOF'
#!/bin/bash
set -euo pipefail

if [[ "$*" == *"get pod dbservice-4m2m7 -n default -o jsonpath={.status.phase}"* ]]; then
  printf "Running"
  exit 0
fi
if [[ "$*" == *"get pod dbservice-4m2m7 -n default -o jsonpath={.metadata.labels.etmem\\.openeuler\\.io/enable}"* ]]; then
  printf "true"
  exit 0
fi
if [[ "$*" == *"logs ds/etmem-agent --tail=500"* ]]; then
  cat <<'LOGS'
2026-04-24T13:48:31Z INFO agent Matched pod {"pod": "dbservice-7v774", "pids": 8, "policy": "etmem-auto"}
2026-04-24T14:10:01Z INFO agent Matched pod {"pod": "dbservice-4m2m7", "pids": 8, "policy": "etmem-auto"}
2026-04-24T14:10:01Z INFO agent Task started successfully {"project": "default-dbservice-7v774-dbmonitor-p1", "pod": "dbservice-7v774"}
2026-04-24T14:10:01Z INFO agent Task started successfully {"project": "default-dbservice-4m2m7-dbmonitor-p2", "pod": "dbservice-4m2m7"}
LOGS
  exit 0
fi
if [[ "$*" == *"get etmemnodestate -A -o jsonpath="* ]]; then
  printf "dbservice-4m2m7 → default-dbservice-4m2m7-dbmonitor-p2 (running)\n"
  exit 0
fi
if [[ "$*" == *"get pod dbservice-4m2m7 -n default -o jsonpath={.spec.nodeName}"* ]]; then
  printf "cp0"
  exit 0
fi
if [[ "$*" == *"get etmemnodestate cp0 -o jsonpath={.status.socketReachable} {.status.etmemdReady} {.status.metrics.totalManagedPods}"* ]]; then
  printf "true true 1"
  exit 0
fi
if [[ "$*" == *"get pod dbservice-4m2m7 -n default -o jsonpath={.metadata.uid}"* ]]; then
  printf "11111111-2222-3333-4444-555555555555"
  exit 0
fi
if [[ "$*" == *"get pod dbservice-4m2m7 -n default -o jsonpath={.status.qosClass}"* ]]; then
  printf "Guaranteed"
  exit 0
fi

echo "unexpected kubectl args: $*" >&2
exit 1
EOF
chmod +x "${TMPDIR}/kubectl"

OUTPUT="$(PATH="${TMPDIR}:$PATH" bash "${SCRIPT_DIR}/verify-existing-workload-etmem.sh" dbservice-4m2m7 default || true)"

if grep -q 'dbservice-7v774' <<<"${OUTPUT}"; then
  echo "FAIL: script output still contains old pod logs"
  exit 1
fi

if ! grep -q 'dbservice-4m2m7' <<<"${OUTPUT}"; then
  echo "FAIL: script output does not contain target pod logs"
  exit 1
fi

echo "PASS"
