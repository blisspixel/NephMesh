# Releasing the operator image

The operator image runs on Linux amd64 and arm64. Its Go binary and the Meshtastic CLI run as UID 65532 with a read-only root filesystem. Python packages and their hashes are in `operators/meshtastic-operator/requirements.txt`. To refresh the lock from the repository root, keep the CLI version aligned with the simulated-firmware test. This command reproduced the 0.2.7 dependency list with uv 0.13.9.

```sh
printf '%s\n' 'meshtastic[cli]==2.7.11' | uv pip compile --python-version 3.14 --python-platform linux \
  --generate-hashes --no-strip-extras --no-header --no-annotate - > requirements.new
diff -u -I '^#' operators/meshtastic-operator/requirements.txt requirements.new
```

Review changes before replacing the dependency list under the existing header.

The `operator image` workflow builds one OCI archive containing both platforms. BuildKit includes an SPDX inventory from Syft and SLSA build provenance for each platform. The read-only job checks those attestations, starts both binaries, inventories the installed Python environment for pip-audit, and scans each image with Trivy. A high or critical Trivy result, a pip-audit finding, or a failed smoke test stops publication. Hadolint checks the operator Dockerfile.

## Publish from a checked main commit

Run the workflow after the candidate has landed on main and its checks have passed.

```sh
gh workflow run operator-image.yaml --ref main
```

The publish job imports the checked archive into GHCR under `sha-<full-source-commit>`, compares the remote index digest with the build output, then signs and verifies that digest with keyless Cosign. It also signs the OCI archive as a blob. The run summary gives the digest to pin in the package. Download the release files within the workflow artifact retention period of seven days.

```sh
gh run download "$RUN_ID" -n operator-oci -D release
gh run download "$RUN_ID" -n operator-signature -D release
```

Verify the archive before importing it into a local registry or transferring it to an offline site. The identity binds the signature to the publish workflow on main.

```sh
identity=https://github.com/blisspixel/NephMesh/.github/workflows/operator-image.yaml@refs/heads/main
issuer=https://token.actions.githubusercontent.com
cosign verify-blob --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" \
  --bundle release/operator.sigstore.json release/operator.tar
```

Preserve the complete index while mirroring so that both platform images, their inventories, and their provenance travel together. Check the mirror digest against the signed release before installing.

```sh
regctl image import "$MIRROR:release" release/operator.tar
regctl image digest "$MIRROR:release"
```
