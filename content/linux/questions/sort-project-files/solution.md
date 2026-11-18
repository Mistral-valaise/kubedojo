Look first, create the directories, then let the shell's patterns pick the files.

```bash
ssh {{host}}
cd /opt/course/sort-project-files/project
ls -la                                  # -a: hidden files too

mkdir -p src tests docs assets/img      # -p: create missing parents, no error if it exists
mv -- test_*.py tests/                  # the tests first ...
mv -- *.py src/                         # ... then *.py is what is left
mv -- *.md .*.md docs/                  # visible and hidden Markdown files
mv docs/README.md .                     # the one exception goes back
mv -- *.png assets/img/
ls -la . docs
```

| Trap | What happens | The way out |
|---|---|---|
| `-draft.md` | `mv *.md docs/` becomes `mv -draft.md ...`, and `mv` reads `-d -r -a ...` as options | `--` ends the options: `mv -- *.md docs/`. Or give a path: `mv ./-draft.md docs/` |
| `.todo.md` | `*` does not match names that start with a dot | add the pattern `.*.md`, or name the file |
| `design notes.md` | fine as a glob result, split in two by `for f in $(ls *.md)` | let the glob produce the arguments; write `"$f"` in loops |
| `README.md` | matches `*.md` like the others | move it back, or use `mv -- [!R]*.md ...` when the names allow it |

Before a move with a pattern, look at what it expands to: `echo mv -- *.md docs/` prints the
command instead of running it. The trailing slash in `docs/` makes `mv` fail when `docs` is not a
directory, instead of renaming a file to `docs`.

**Takeaway:** globs are expanded by the shell, name by name, so spaces are safe; protect against
leading dashes with `--`, and remember that `*` skips dot files.
