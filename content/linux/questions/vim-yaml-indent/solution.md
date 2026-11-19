In YAML the indentation **is** the structure, and it must consist of spaces. Two things are wrong
here: tab characters, and blocks at the wrong depth.

```bash
ssh {{host}}
vim /opt/course/vim-yaml-indent/pod.yaml
```

**1. See the tabs.**

```vim
:set list
```

Tabs show as `^I`, line ends as `$`. Lines 14 to 24 start with tabs. `:set nolist` switches the
display off again.

**2. Convert them.** The lab host's `~/.vimrc` already sets `tabstop=2 shiftwidth=2 expandtab`; on
another machine set them first.

```vim
:set ts=2 sw=2 et
:retab
```

Each tab becomes two spaces. The file may parse now, but `env` stands in the column of
`containers`: it would be a field of the Pod, not of the container.

**3. Shift the blocks.** `image` and `ports` start in column 7 (six spaces), so `env` and
`resources` must move from two to six spaces: two levels to the right.

| Keys | What happens |
|---|---|
| `:14` Enter | cursor on the line `env:` |
| `V` | visual line mode |
| `/memory` Enter (or `10j`) | extend the selection down to `memory: 128Mi` |
| `>` | shift the selection one `shiftwidth` to the right |
| `.` | repeat: second level |
| `/volumes:` Enter, `V`, `G` | select from `volumes:` to the last line |
| `<` then `.` | two levels to the left: `volumes` is in the column of `containers` |
| `:wq` Enter | save and quit |

Without visual mode: `>>` and `<<` shift the current line, `5>>` five lines, and
`:14,24>>` shifts lines 14 to 24 by two levels at once.

The result:

```yaml
spec:
  containers:
    - name: api
      image: registry.example.com/shop/api:2.4.1
      ports:
        - containerPort: 8080
      env:
        - name: LOG_LEVEL
          value: info
      # ...
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
      volumeMounts:
        - name: config
          mountPath: /etc/api
  volumes:
    - name: config
      configMap:
        name: api-config
```

To avoid the problem when you paste into Vim yourself: `:set paste` before pasting (it stops
autoindent from adding a level per line), `:set nopaste` afterwards.

**Takeaway:** `:set list` to see tabs, `:retab` to remove them, `V` plus `>` / `<` and `.` to move a
whole block to the right depth.
