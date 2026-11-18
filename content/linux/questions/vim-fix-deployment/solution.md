Four findings, and each is a different kind of edit. With `:set nu` you see line numbers; `:set
list` would show a tab if one slipped in.

```bash
ssh {{host}}
vim /opt/course/vim-fix-deployment/deployment.yaml
```

**1. replicas.** One character changes.

| Keys | What happens |
|---|---|
| `/replicas` Enter | cursor on the line |
| `$` | last character of the line: the `1` |
| `r3` | replace it by `3`, no insert mode needed |

**2. The image.** `/ngnix` Enter puts the cursor on the typo; `cw` `nginx` Esc retypes the word.
(`:%s/ngnix/nginx/` does the same from the command line.)

**3. The port.** New lines, and the column matters: `ports` is a key of the container, so it starts
where `image` starts.

| Keys | What happens |
|---|---|
| `o` (cursor on the image line) | open a line below; autoindent puts the cursor under the `i` of `image` |
| `ports:` Enter | the key; the next line starts in the same column |
| two spaces, `- containerPort: 8080` Esc | the list entry, one level deeper |

**4. The resources.** The container `log-shipper` has the block already. Copy it, do not type it.

| Keys | What happens |
|---|---|
| `/resources` Enter | the line `resources:` of log-shipper |
| `V3j` | select it and the three lines below |
| `y` | yank the four lines |
| `?containerPort` Enter | search backwards to the port line of web |
| `p` | paste below it; both containers are indented alike, so the block fits |
| `/10m` Enter, `cw100m` Esc | correct the CPU request (the first match after the cursor is the copy) |
| `j`, then `:s/16Mi/64Mi/` Enter | correct the memory request on the next line |
| `:wq` Enter | save and quit |

The container now reads:

```yaml
        - name: web
          image: nginx:1.27-alpine
          ports:
            - containerPort: 8080
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
          env:
```

Key order inside a mapping does not matter, comments are allowed, and
`resources: {requests: {cpu: 100m, memory: 64Mi}}` on one line means the same. What does matter is
the depth: the same two lines four columns further left would be fields of the Pod.

On a machine with a cluster, `kubectl apply --dry-run=client -f deployment.yaml` checks the syntax
and `--dry-run=server` also checks the field names.

**Takeaway:** `r` and `cw` for values, `o` for a new line at the same depth, and yank and paste for
blocks that already exist; then read the indentation once more before `:wq`.
