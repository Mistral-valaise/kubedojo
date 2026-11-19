The rule for every clean-up: **list with the same pattern first, delete second.**

```bash
ssh {{host}}
cd /opt/course/cleanup-build-cache/workspace

find . -type f -name '*.tmp'             # 1. look: six files, some hidden, one with a space
find . -type f -name '*.tmp' -delete     #    then delete exactly that list

ls -l build/obj/*.o                      # 2. look: main.o and util.o, not main.o.d
rm -- build/obj/*.o

ls -la build/cache                       # 3. look: there is a symbolic link to ../../vendor
rm -r build/cache
ls vendor/lib                            #    still complete
```

| Step | Why this form |
|---|---|
| `find . -type f -name '*.tmp'` | reaches every depth and dot files; `-type f` keeps the directory `archive.tmp` out. `-delete` goes last: `find` evaluates left to right, and `-delete` in front would delete everything |
| `rm -- build/obj/*.o` | the directory in the pattern limits it to `build/obj`; `vendor/lib/*.o` cannot match. `--` protects against names that start with a dash |
| `rm -r build/cache` | removes a symbolic link as a link. The target is only in danger when the link is followed: `rm -r build/cache/vendor/` (trailing slash) or `rm -r build/cache/*/` empties `vendor` itself |

What goes wrong with the quick versions:

- `rm *.tmp` finds nothing here, `rm */*.tmp` misses `.swap.tmp` and `.config/session.tmp`.
- `rm -rf **/*.tmp` (with `globstar`) also takes the directory `archive.tmp` with its database.
- `find . -name '*.o' -delete` removes the prebuilt objects in `vendor/lib`, which nobody can
  rebuild here.

`rm` has no undo. `rm -i` asks for every file and `rm -I` once for more than three files or a
recursive delete; replacing `rm` by `ls -ld` in the command line is the cheapest dry run.

**Takeaway:** make the pattern as narrow as the task, look at what it matches, and only then turn
the `ls` into `rm`.
