#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/port-hog

# Who listens on the port? Without sudo: the process is our own.
pid=$(ss -ltnpH 'sport = :22110' | grep -o 'pid=[0-9]*' | cut -d= -f2)

# Record first: afterwards the process is gone.
{
  echo "pid=$pid"
  echo "user=$(ps -o user:32= -p "$pid" | tr -d ' ')"
  echo "command=$(ps -o args= -p "$pid")"
} > culprit.txt

# SIGTERM is the default of kill: the polite request to finish.
kill "$pid"
while kill -0 "$pid" 2>/dev/null; do sleep 0.1; done
