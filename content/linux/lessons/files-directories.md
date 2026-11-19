# Files & Directories

**8% of the skills assessment.** Copy, move, delete and link without losing data: a handful of
commands, and three things that bite, namely globs, links and `rm`.

## The 20% that solves 80%

### Create, copy, move

```bash
mkdir -p app/conf/env                 # -p: create missing parents, no error if it exists
touch app/conf/app.yaml               # create an empty file, or set its time to now
touch -d '3 days ago' old.log         # set a time on purpose
cp app.yaml app.yaml.bak              # copy a file
cp -a app/ /backup/app                # copy a tree: recursive, keeps modes, times, owners and links
cp -r src/ dst/                       # recursive without preserving: new times, links may be followed
mv report.txt docs/                   # move into a directory (the trailing slash demands one)
mv old.txt new.txt                    # rename
mv -n a b                             # never overwrite; -i asks first
```

- `cp` and `mv` overwrite an existing destination **without asking**. With several sources, the last
  argument must be a directory.
- `cp -a src dst`: if `dst` exists, the result is `dst/src`; if not, `dst` becomes the copy. Check
  with `ls` when the layout matters.
- `mv` within one file system only renames: it is instant and keeps the inode.

### Globs: the shell picks the files

```bash
ls *.log                # every name ending in .log, in this directory
ls report-?.txt         # ? is exactly one character
ls [a-c]*.conf          # one character from a set; [!a-c] negates
ls *.{yaml,yml}         # brace expansion: two patterns
ls -d .[!.]*            # hidden entries: * alone never matches a leading dot
echo mv -- *.md docs/   # dry run: echo shows what the command line becomes
```

- The shell replaces the pattern by the matching names **before** the command starts; each name is
  one argument, so spaces in names are safe here. They break in `for f in $(ls)`: write
  `for f in *.md; do mv -- "$f" docs/; done`.
- A name that starts with a dash looks like an option. `--` ends the options (`rm -- -draft.md`),
  or give a path (`rm ./-draft.md`).
- A pattern that matches nothing stays as it is, and the command gets the literal `*.log`.
- `*` does not cross directories. For every depth use `find`, or `shopt -s globstar` and `**/*.tmp`.

### Delete on purpose

```bash
ls -ld build/obj/*.o                  # look first ...
rm -- build/obj/*.o                   # ... then turn ls into rm
rm -r build/cache                     # a directory with its content
rmdir empty-dir                       # only removes an empty directory: a safe default
find . -type f -name '*.tmp' -delete  # every depth, hidden files included, files only
rm -i *.bak                           # ask for each; -I asks once for many
```

There is no trash and no undo. `rm -r link` removes a symbolic link; `rm -r link/` works on the
directory behind it. Never build the path of a recursive delete from a variable that may be empty:
with an unset `d`, the argument `"$d/"` is the root directory.

### Links, inodes and stat

A file is an **inode** (the data and its attributes); a directory entry is a **name** for an inode.

```bash
ln data.db backup.db                  # hard link: a second name for the same inode
ln -s releases/v2 current             # symbolic link: a small file that holds a path
ln -sfn releases/v3 current           # replace a link to a directory (-n: do not follow it)
ls -li                                # first column: inode number; after the modes: link count
readlink current                      # the stored target        readlink -f   fully resolved
stat data.db                          # size, inode, links, modes, owner, the three timestamps
stat -c '%h %i %s %n' data.db         # just link count, inode, size, name
find . -samefile data.db              # all names of one inode
find . -xtype l                       # broken symbolic links
```

| | Hard link | Symbolic link |
|---|---|---|
| Is | another name for the inode | a path, stored as text |
| Target removed | the data lives until the last name is gone | the link dangles |
| Across file systems, on directories | no | yes |
| Relative target | does not apply | resolved from the link's own directory |

A relative symbolic link (`ln -s releases/v2 current`) survives moving or copying the whole tree; an
absolute one keeps pointing at the old place. `ln -s` writes the target exactly as typed and does
not check it: `ls -l` afterwards, and `ls -L` or `cat` to prove that it resolves.

## Habits that save points

- `ls -la` before and after: a plain `ls` hides dot files, and with them what you forgot.
- Use `mkdir -p` and trailing slashes on directory destinations; both turn silent surprises into
  errors or no-ops.
- Back up before a risky edit: `cp -a file file.bak` costs two seconds.
- Use the narrowest pattern that does the job, with the directory in it: `build/obj/*.o`, not `*.o`
  from wherever you happen to be.
- "Same content" is not "same file": compare inodes with `ls -li` or `[ a -ef b ]`, content with
  `cmp a b`.
