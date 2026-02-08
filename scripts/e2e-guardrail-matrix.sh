#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "[1/3] deterministic failure -> loop_restart blocked + final.json present"
go test ./internal/attractor/engine -run TestRun_LoopRestartBlockedForDeterministicFailureClass -count=1

echo "[2/3] transient failure -> circuit breaker + terminal finalization"
go test ./internal/attractor/engine -run 'TestRun_LoopRestartCircuitBreakerOnRepeatedSignature|TestRun_LoopRestartLimitExceeded_WritesTerminalFinalJSON' -count=1

echo "[3/3] detached launch -> run.pid + eventual final.json"
go test ./cmd/kilroy -run 'TestAttractorRun_DetachedModeSurvivesLauncherExit|TestAttractorRun_DetachedWritesPIDFile' -count=1

echo "guardrail matrix: PASS"
