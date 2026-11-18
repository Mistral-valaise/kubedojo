While a file is open, Vim keeps its unsaved state in a **swap file** next to it:
`.limits.conf.swp` for `limits.conf`. A normal exit removes it. After a crash or a dropped
connection it stays, and it is all that is left of the unsaved work.

```bash
ssh {{host}}
cd /opt/course/vim-recover-swap
ls -la                    # .limits.conf.swp and .upstream.conf.swp
vim -r                    # lists the swap files Vim finds, with owner, date and process
```

**limits.conf: recover.**

```bash
vim -r limits.conf
```

Vim reads the swap file and says `Recovery completed`. Press Enter and look at the text: the limits
of `login` and `search` changed and a section `[checkout]` is at the end. Still nothing is on disk.

```vim
:w
:q
```

```bash
rm .limits.conf.swp
```

Opening the file with plain `vim limits.conf` leads to the same place. Vim shows `E325: ATTENTION`
and asks:

```
[O]pen Read-Only, (E)dit anyway, (R)ecover, (D)elete it, (Q)uit, (A)bort:
```

| Answer | Meaning |
|---|---|
| `R` | recover: load the swap file's content, as `vim -r` does |
| `D` | delete the swap file and edit the file as it is on disk: the unsaved work is gone |
| `E` | edit the file on disk and keep the swap file: the question comes back next time |
| `O` | look at the file on disk without the risk of changing it |
| `Q` / `A` | do nothing |

To compare before deciding, recover and then run `:w /tmp/recovered` to write the recovered text
somewhere else, quit with `:q!`, and `diff /tmp/recovered limits.conf`.

**upstream.conf: discard.** The file on disk is the good one, so the swap file just goes:

```bash
rm .upstream.conf.swp
```

or `vim upstream.conf`, answer `D`, then `:q`.

```bash
ls -a                     # no .swp left
```

If the message says the process is `(STILL RUNNING)`, the file is open in another terminal or in
a Vim you suspended with Ctrl-z: get it back with `fg` and quit it properly instead.

**Takeaway:** a swap file means unsaved work. `vim -r file`, check, `:w`, then delete the `.swp`
yourself; delete it without recovering only when you are sure you do not want its content.
