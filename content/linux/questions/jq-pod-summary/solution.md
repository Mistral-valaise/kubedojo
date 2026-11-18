`jq` is `awk` for JSON: a filter describes a path into the document, `|` passes results on, and `-r`
prints strings raw, so that `sort`, `uniq` and `head` can take over.

**Explore**

```bash
ssh {{host}}
cd /opt/course/jq-pod-summary
jq '.items | length' pods.json                             # 12 Pods
jq '.items[0]' pods.json | less                            # the shape of one Pod
jq -r '.items[].status.phase' pods.json | sort | uniq -c   # which phases occur
```

**1. Pods that are neither Running nor Succeeded**

```bash
jq -r '.items[]
  | select(.status.phase != "Running" and .status.phase != "Succeeded")
  | "\(.metadata.namespace)/\(.metadata.name)"' pods.json > not-running.txt
```

`.items[]` emits the Pods one by one, `select()` drops those for which the condition is false, and
`"\(...)"` inserts values into a string.

**2. The three Pods with the most restarts**

```bash
jq -r '.items[]
  | "\([.status.containerStatuses[]?.restartCount] | add // 0) \(.metadata.namespace)/\(.metadata.name)"' pods.json \
  | sort -rn | head -3 > restarts.txt
```

| Part | Meaning |
|---|---|
| `[ ... ]` | collects the restart counts of all containers of the Pod into an array |
| `[]?` | no error when `containerStatuses` is missing (the Pending Pod has none) |
| `add // 0` | the sum; `add` of an empty array is `null`, and `//` supplies a default |
| `sort -rn \| head -3` | numeric, highest first, three lines |

**3. Distinct images**

```bash
jq -r '.items[].spec.containers[].image' pods.json | sort -u > images.txt
```

The path names `spec.containers` exactly, so `spec.initContainers` and the copies of the image names
in `status.containerStatuses` stay out.

Everything can be done inside jq as well (`sort_by`, `unique`, `.[:3]`), but a short filter plus the
tools you already know is faster to write and easier to check step by step.

**Takeaway:** let `jq -r` turn JSON into lines, then finish with the usual pipeline.
The same filters work on live output: `kubectl get pods -A -o json | jq -r '...'`.
