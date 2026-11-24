### Security

- **Releases are signed.** `checksums.txt`, every image and the Helm chart carry a keyless
  [Sigstore](https://www.sigstore.dev/) signature made by the release workflow, the images and the
  chart by digest. `SECURITY.md` has the `cosign` commands under "Verifying a release".
- **Releases come with SBOMs.** Every archive has an SPDX SBOM next to it
  (`<archive>.sbom.json`, listed in `checksums.txt`). The server, node, terminal and linux images
  carry an SBOM and build provenance as attestations.
- CodeQL analyses the Go and the TypeScript code on every pull request, on `main` and weekly.
- Trivy scans the dependencies, the Dockerfiles, the chart and the server image on pull requests
  that touch them, on `main` and weekly. A HIGH or CRITICAL vulnerability that has a fix fails the
  run. Exceptions are listed with a reason and an end date in `.github/trivyignore.yaml`.
- Every GitHub Action is pinned to a commit, and Dependabot keeps the pins current. A workflow's
  token can only read; the release jobs get the one or two write permissions they need.
