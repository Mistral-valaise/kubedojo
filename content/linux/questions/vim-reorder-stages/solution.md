Moving text in Vim is **delete, move the cursor, put**. Every delete goes into a register, and `p`
pastes the register's content.

```bash
ssh {{host}}
vim /opt/course/vim-reorder-stages/pipeline.conf
```

| Keys | What happens |
|---|---|
| `dd` / `4dd` | delete (cut) one line / four lines |
| `dap` | delete "a paragraph": the block around the cursor plus the blank line after it |
| `yy` / `yap` | yank (copy) a line / a paragraph without deleting it |
| `p` / `P` | put below / above the cursor line |
| `u` / Ctrl-r | undo / redo |

One way through the task:

1. **Remove the obsolete stage.** `/legacy` Enter, then `dap`. Nothing is pasted, so it is gone.
   The file now reads package, test, build, lint, deploy.
2. **lint first.** `/\[lint` Enter, `dap`. Then `/\[package` Enter (the search wraps around to the
   top of the file) and `P`: the block lands above `[package]`.
3. **build second.** `/\[build` Enter, `dap`, `/\[package` Enter, `P`.
4. **package after test.** `/\[package` Enter, `dap`, `/\[deploy` Enter, `P`.
5. **Copy the line.** `/on_failure` Enter, `yy`, `G` (last line, the end of `[deploy]`), `p`.
6. `:wq`

`dap` takes the blank line after the block along, and `P` on a `[name]` line pastes above it, so
the stages stay separated by one blank line. The last block of a file has no blank line after it:
there `dap` takes the blank line before it instead, and a pasted copy sticks to the next block.
That is why this walk-through never moves `[deploy]`.

Check the order without leaving Vim: `:g/^\[/#` lists the stage headers with line numbers.

The same kind of edit on the command line, handy when you can see the line numbers (`:set nu`).
Each example refers to the file as it is when you open it:

```vim
:15,19d          " delete lines 15 to 19: legacy-upload and its blank line
:25,29m3         " move lines 25 to 29 below line 3: lint becomes the first stage
:13t$            " copy line 13 to the end of the file: on_failure into deploy
```

The trap: a register holds one thing. `dap`, then another `dd` to tidy up a blank line, then `p`
pastes the blank line; the block is in the numbered register `"2` (`"2p`), or two `u` away.

**Takeaway:** `dd`/`dap` cut, `yy` copies, `p`/`P` paste; move blocks as blocks and never retype.
