# Vim

**20% of the skills assessment.** In the Kubernetes exams every manifest is edited in a terminal
under time pressure, and `vim` is the editor that is always there. You need about thirty keys.

## The 20% that solves 80%

### Modes: the one idea

Vim starts in **normal mode**, where keys are commands. Esc always brings you back to it.

```text
i  a        insert before / after the cursor         I  A     at the start / end of the line
o  O        open a new line below / above            Esc      back to normal mode
v  V  Ctrl-v   select characters / lines / a column block
:           command line:  :w  save    :q  quit    :wq  both    :q!  quit and discard changes
```

Lost? Press Esc twice, then `u` to undo or `:q!` to leave without saving. Nothing is written to
disk before `:w`.

### Move

```text
h j k l     left, down, up, right (the arrow keys work too)
w  b        next / previous word                      0  ^  $   line start, first non-blank, line end
gg  G       first / last line                         42G or :42    line 42
/text       search forward, n next match, N previous  ?text         search backward
fx          jump to the next x on this line           %             matching bracket
```

### Edit: operator + motion

```text
x  r3       delete a character / replace it by 3      cw      change to the end of the word
dd  3dd     delete (cut) one / three lines            dw  D   delete a word / the rest of the line
yy  3yy     yank (copy) lines                         p  P    paste below / above the cursor line
ci"         change the text inside the quotes         dap     delete a paragraph with its blank line
>>  <<      indent / unindent the line                J       join the next line to this one
.           repeat the last change                    u  Ctrl-r    undo / redo
```

A count goes in front (`5>>`, `3dd`), and every delete is a cut: `dd`, move, `p` moves a line.
The `.` key is the fastest tool there is: make a change once, go to the next place, press `.`.

### Search and replace

```vim
:s/old/new/            " first match on the current line
:%s/old/new/g          " every match in the file
:%s/old/new/gc         " ask at each match: y yes, n no, a all, q quit
:10,20s/^/# /          " lines 10 to 20: comment out
:%s/\<warn\>/warning/g " whole words only: not "warning", not "prewarn"
:%s/= warn$/= warning/ " anchored at the end of the line
:g/pattern/#           " list matching lines with numbers;  :v/pattern/#  lists the others
```

Look before you replace: `:g/old/#` shows what a pattern hits. One `u` undoes a whole substitute.

### Blocks and columns

```text
V  then j/k        select lines;   > or < shifts them,  d cuts,  y copies,  . repeats the shift
Ctrl-v  5j  I# Esc   insert "# " in front of six lines (it appears on all lines after Esc)
Ctrl-v  5j  l  x     delete a two-column block from six lines (uncomment)
Ctrl-v  5j  $  A; Esc   append to the end of six lines
Ctrl-q             the same as Ctrl-v, for terminals in which Ctrl-v pastes
```

### YAML: the settings that matter

The lab host's `~/.vimrc` sets these already. On any other machine type the line after `:`, or put
it into `~/.vimrc` once:

```vim
set tabstop=2 shiftwidth=2 expandtab autoindent
```

`expandtab` makes the Tab key insert spaces (YAML forbids tabs), `shiftwidth` is the step of `>>`
and `<<`, and `autoindent` starts a new line at the depth of the one above.

```vim
:set nu          " line numbers (:set nonu)           :set list     " show tabs as ^I (:set nolist)
:retab           " turn existing tabs into spaces     :set paste    " before pasting from the clipboard
:14,24>          " shift lines 14-24 one level        :set nopaste  " afterwards
```

Without `:set paste`, autoindent adds indentation to every pasted line and the block walks off to
the right like a staircase. A key belongs to the block whose keys start in the same column: after
pasting, compare the first column of the new lines with their future siblings.

### Recover

```bash
vim -r file          # load unsaved work from the swap file .file.swp after a crash
rm .file.swp         # afterwards; Vim never removes it for you
```

Opening a file whose swap file exists shows `E325: ATTENTION`: `R` recovers, `D` deletes the swap
file, `Q` leaves everything alone. Recovered text is on disk only after `:w`.

## Habits that save points

- Stuck or unsure what you changed: Esc, `u` until it looks right, or `:q!` and open the file again.
- `E37: No write since last change`: Vim refuses to quit. `:wq` to keep the changes, `:q!` to drop them.
- "Permission denied" on `:w`: you needed `sudo vim`. `:w /tmp/copy`, quit, `sudo cp` it into place.
- The screen froze: you pressed Ctrl-s. Ctrl-q unfreezes it. Ctrl-z suspended Vim: `fg` brings it back.
- Copy existing blocks (`V`, `y`, `p`) and change the values instead of typing YAML from memory.
- After editing a manifest, read the left edge of the file once from top to bottom: wrong depth
  shows there, and `:set list` shows a tab that slipped in.
