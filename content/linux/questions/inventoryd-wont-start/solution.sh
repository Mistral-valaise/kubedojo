#!/usr/bin/env bash
set -euo pipefail

# See it fail, and read why.
sudo service inventoryd start || true
cat /var/log/inventoryd.log

# Fault 1: the init script is told to use a configuration file that does not exist.
sudo sed -i 's#^CONFIG=.*#CONFIG=/etc/inventoryd/inventoryd.conf#' /etc/default/inventoryd

# Fault 2: the port is taken. Find the process that holds it and ask it to stop.
pid=$(ss -ltnpH 'sport = :22210' | grep -o 'pid=[0-9]*' | cut -d= -f2)
ps -o pid,user:20,args -p "$pid"
kill "$pid"
while kill -0 "$pid" 2>/dev/null; do sleep 0.1; done

sudo service inventoryd start
sudo service inventoryd status
curl -fsS http://127.0.0.1:22210/healthz
