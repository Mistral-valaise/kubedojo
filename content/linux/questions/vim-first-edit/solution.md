Vim starts in **normal mode**: keys are commands. Text is typed in **insert mode**, and Esc always
leads back to normal mode.

```bash
ssh {{host}}
vim /opt/course/vim-first-edit/db.conf
```

| Keys | What happens |
|---|---|
| `/max_connections` Enter | search forward; the cursor lands on the comment first, `n` jumps to the next match, the setting |
| `w` `w` | two words forward: the cursor is on `100` |
| `cw` | change word: `100` is removed and you are in insert mode |
| `250` Esc | type the new value, back to normal mode |
| `:wq` Enter | write the file and quit |

If the file looks wrong at any point: `u` undoes the last change (repeat it as often as needed),
Ctrl-r redoes, and `:q!` quits without saving.

The same change as a command, typed after `:`:

```vim
:/^max_connections/s/100/250/
:wq
```

That is "on the next line that starts with max_connections, substitute 100 by 250". Without the line
address, `:%s/100/250/g` would change four lines, among them `shared_buffers = 100MB`.

Check the result from the shell:

```bash
grep -n 100 /opt/course/vim-first-edit/db.conf     # three lines are left, max_connections is not one of them
```

**Takeaway:** Esc, then `:wq` to save or `:q!` to bail out. Everything else in Vim is optional; these
two are not.
