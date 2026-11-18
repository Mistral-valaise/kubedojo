#!/usr/bin/env bash
# expect: pass
# Another correct way without jq: the json module of python3. The counts are
# written right-aligned, as uniq -c would print them.
set -euo pipefail
cd /opt/course/jq-pod-summary
python3 - <<'PY'
import json
pods = json.load(open("pods.json"))["items"]
name = lambda p: p["metadata"]["namespace"] + "/" + p["metadata"]["name"]
with open("not-running.txt", "w") as f:
    for p in pods:
        if p["status"]["phase"] not in ("Running", "Succeeded"):
            f.write(name(p) + "\n")
ranked = sorted(pods, key=lambda p: -sum(c["restartCount"] for c in p["status"].get("containerStatuses", [])))
with open("restarts.txt", "w") as f:
    for p in ranked[:3]:
        f.write("%7d %s\n" % (sum(c["restartCount"] for c in p["status"]["containerStatuses"]), name(p)))
with open("images.txt", "w") as f:
    for image in sorted({c["image"] for p in pods for c in p["spec"]["containers"]}):
        f.write(image + "\n")
PY
