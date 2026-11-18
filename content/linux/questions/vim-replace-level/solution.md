`:s` is Vim's substitute command: `:[range]s/pattern/replacement/[flags]`. The work is in choosing a
pattern that matches the seven values and nothing else.

```bash
ssh {{host}}
vim /opt/course/vim-replace-level/logging.conf
```

First look at every line that contains the four letters:

```vim
:g/warn/#
```

Seventeen lines. Seven end in `= warn`; the others are comments, setting names (`warn_rate_limit`,
`payment.prewarn.level`), values that are already `warning`, and `message_prefix = "warn: "`.

**The precise pattern.** What the seven lines share is `= warn` at the end of the line:

```vim
:%s/= warn$/= warning/
:wq
```

`%` is the range "every line", and `$` anchors the match at the end of the line.

**Word boundaries plus confirmation.** `\<` and `\>` match the start and the end of a word, so
`\<warn\>` skips `warning`, `prewarn` and `warn_rate_limit`. It still matches the English word in
the comments and inside `"warn: "`, so let Vim ask:

```vim
:%s/\<warn\>/warning/gc
```

For every match Vim shows `replace with warning (y/n/a/q/l/^E/^Y)?`: `y` replaces, `n` skips, `q`
stops, `a` replaces all that remain. Answer `n` on the three comments and on `message_prefix`.

| Pattern | Result |
|---|---|
| `:%s/warn/warning/g` | wrong: `warninging`, `warning_rate_limit`, changed comments |
| `:%s/\<warn\>/warning/g` | wrong: names are safe, but comments and `"warn: "` change |
| `:%s/\<warn\>/warning/gc` | right, if you answer `n` four times |
| `:%s/= warn$/= warning/` | right |

A substitute that went wrong is one change: `u` undoes all of it.

```bash
grep -c '= warning$' /opt/course/vim-replace-level/logging.conf     # 9: seven new, two old
```

**Takeaway:** list the matches with `:g/pattern/#` first, then substitute with a pattern anchored
on its surroundings; `\<word\>` and the flag `c` are the tools when the surroundings are not enough.
