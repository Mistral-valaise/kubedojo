The same edit in the same column of many lines is a job for **visual block mode**: Ctrl-v selects a
rectangle.

```bash
ssh {{host}}
vim /opt/course/vim-block-edit/haproxy.cfg
```

**1. Comment out the blue servers.**

| Keys | What happens |
|---|---|
| `/server blue-1` Enter | cursor on the `s` of `server` in the first blue line |
| Ctrl-v | visual block mode |
| `5j` | extend five lines down: a block one column wide, six lines high |
| `I` | insert before the block (capital I) |
| `# ` Esc | typed once; after Esc it appears on all six lines |

**2. Activate the green servers.**

| Keys | What happens |
|---|---|
| `/# server green-1` Enter | cursor on the `#` |
| Ctrl-v `5j` `l` | six lines high, two columns wide: `#` and the space |
| `x` | delete the block |

**3. Prefix the networks.**

| Keys | What happens |
|---|---|
| `:5` Enter, `^` | first network line, first non-blank character |
| Ctrl-v `5j` | six lines |
| `I` `acl admin_net src ` Esc | the prefix on every line |
| `:wq` Enter | save and quit |

Good to know:

- `gv` selects the previous block again, `o` jumps to the other corner of the selection.
- `A` appends after the block, `$A` at the end of every line whatever its length, `c` replaces the
  block, `r` plus a character overwrites it.
- Some terminals use Ctrl-v for pasting: Ctrl-q starts visual block mode as well.

The same three edits as commands with line ranges (`:set nu` shows the numbers):

```vim
:21,26s/^\s*/&# /
:28,33s/# //
:5,10s/^\s*/&acl admin_net src /
```

`&` in the replacement stands for the matched text, here the indentation. And `:21,26normal I# `
types `I# ` on every line of the range.

**Takeaway:** Ctrl-v, a motion, then `I`text Esc to insert, `x` to delete: one edit for a whole
column.
