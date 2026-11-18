Run the sample and read the messages; each one names a line of the script.

```bash
ssh {{host}}
cd /opt/course/fix-deploy-script
./deploy.sh "sample/release 1.4" "out/live site"; echo "exit code: $?"
vim deploy.sh
```

There are four bugs.

**1. `[ $# < 2 ]` is not a comparison.** Inside `[ ]`, `<` is a redirection from a file named `2`.
The test fails with "No such file", `if` takes that as false, and the script continues without
arguments. Numbers are compared with `-lt`, `-le`, `-eq`, `-ne`, `-ge`, `-gt`:

```bash
if [ $# -lt 2 ]; then
  echo "usage: deploy.sh <release-dir> <target-dir>" >&2
  exit 2
fi
```

**2. Unquoted variables.** `$release` with the value `sample/release 1.4` becomes two words.
`[ ! -f sample/release 1.4/manifest.txt ]` is an error ("too many arguments"), again taken as false;
`mkdir -p $target` creates two directories. Quote every expansion:

```bash
if [ ! -f "$release/manifest.txt" ]; then ...
mkdir -p "$target"
```

In Vim, `:%s/\$release\/\S*/"&"/gc` asks for each place; checking every `$` by eye is just as fast in
a script this short.

**3. `for f in $(cat file)` splits at blanks, not at lines.** `release notes.txt` arrives as
`release` and `notes.txt`. Read lines with `read`:

```bash
while IFS= read -r f; do
  cp "$release/$f" "$target/"
  count=$((count + 1))
done < "$release/manifest.txt"
```

`IFS=` keeps leading and trailing blanks, `-r` keeps backslashes.

**4. The pipe swallows the exit code, and nothing exits.** After `healthcheck.sh | tee log`, `$?`
belongs to `tee`. And the `else` branch only printed a message: the script still ended with 0.

```bash
"$release/healthcheck.sh" "$target" | tee "$target/healthcheck.log"
rc=${PIPESTATUS[0]}
if [ "$rc" -eq 0 ]; then
  echo "deployed $count files"
else
  echo "deploy: health check failed" >&2
  exit "$rc"
fi
```

Without `tee`, `"$release/healthcheck.sh" "$target" > "$target/healthcheck.log"; rc=$?` does the same.

Test all paths, not only the one that failed:

```bash
./deploy.sh "sample/release 1.4" "out/live site"; echo $?    # 4: the health check of the sample fails
./deploy.sh; echo $?                                          # usage, 2
./deploy.sh sample out/x; echo $?                             # no manifest, 3
```

**Takeaway:** quote every `"$variable"`, compare numbers with `-lt`/`-eq`, read lines with
`while IFS= read -r`, and never trust `$?` after a pipe.
