The three typos are three different numbers. Do not search for a typo: list the lines that lack the
correct text.

```bash
ssh {{host}}
vim /opt/course/vim-search-fix/backend.cfg
```

```vim
:v/check port 9100/#
```

`:v/pattern/command` runs the command on every line that does **not** match (`:g` is the opposite),
and `#` prints the line with its number. The output shows the four header lines and the culprits:

```
 51     server web-047 10.20.1.56:8080 check port 9010 weight 20
116     server web-112 10.20.2.61:8080 check port 9001 weight 15
167     server web-163 10.20.3.52:8080 check port 910 weight 15
```

Fix them one by one:

| Keys | What happens |
|---|---|
| `:51` Enter (or `51G`) | jump to line 51 |
| `/port` Enter, `w` | cursor on the word `port`, then on the number behind it |
| `cw9100` Esc | change the word to `9100` |
| `:116` Enter, `/port` Enter, `w`, `.` | next line; `.` repeats the last change |
| `:167` Enter, `/port` Enter, `w`, `.` | and the third |
| `:wq` Enter | save and quit |

Other ways to move: `n` repeats a search forward and `N` backward, `gg` and `G` jump to the first
and last line, `0` and `$` to the start and end of a line, `fx` to the next `x` on the line.

When the rule is as regular as here, one command does it all:

```vim
:g/^\s*server /s/check port \d\+/check port 9100/
```

"On every server line, substitute `check port` plus any digits by `check port 9100`." The words
around the number keep the substitute away from addresses and weights.

Verify before leaving, or from the shell afterwards:

```bash
grep -c 'check port 9100 ' /opt/course/vim-search-fix/backend.cfg     # 180
```

**Takeaway:** search for the deviation (`:v/good/#`), jump by line number, and let `.` repeat the fix.
