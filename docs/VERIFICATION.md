# Verifying Radix Releases

Every release published to [GitHub Releases](https://github.com/osuritz/radix/releases)
ships with a `checksums.txt` file containing SHA256 hashes for all release
archives. You can (and should) verify the integrity of any archive you download
before extracting it.

**What is published today:**

- ✅ **SHA256 checksums** (`checksums.txt`) — available for every release.
- ⏳ **GPG signature** (`checksums.txt.asc`) — the release pipeline supports
  detached GPG signing of the checksums file, but a project signing key has not
  yet been set up. Until it is, releases are published without a signature.
  This document will be updated with the key fingerprint once signing is live.

## Verifying checksums (available now)

Checksum verification confirms that the archive you downloaded matches the one
GoReleaser produced — it protects against corrupted or truncated downloads and
mirrors serving tampered files (but not against a compromised release pipeline;
that is what the GPG signature is for).

### Linux

```bash
VERSION="v0.7.1"  # the release you are verifying
ARCHIVE="radix_${VERSION#v}_linux_x86_64.tar.gz"

# Download the archive and the checksums file
curl -LO "https://github.com/osuritz/radix/releases/download/${VERSION}/${ARCHIVE}"
curl -LO "https://github.com/osuritz/radix/releases/download/${VERSION}/checksums.txt"

# Verify (only checks files you actually downloaded)
sha256sum -c checksums.txt --ignore-missing
```

Expected output:

```
radix_0.7.1_linux_x86_64.tar.gz: OK
```

### macOS

macOS ships `shasum` instead of `sha256sum`:

```bash
shasum -a 256 -c checksums.txt --ignore-missing
```

### Windows (PowerShell)

```powershell
$archive = "radix_0.7.1_windows_x86_64.zip"
$actual = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLower()
$expected = (Select-String -Path checksums.txt -Pattern $archive).Line.Split(" ")[0]
if ($actual -eq $expected) { "OK" } else { "CHECKSUM MISMATCH" }
```

If verification fails, delete the archive and re-download it. If it fails
repeatedly, do not run the binary — open an issue at
https://github.com/osuritz/radix/issues.

## Verifying the GPG signature (once signing is live)

Once the project signing key is configured, each release will additionally
include `checksums.txt.asc` — a detached, ASCII-armored GPG signature of the
checksums file. Verifying the signature proves the checksums file itself was
produced by the release pipeline holding the private key; combined with a
checksum match, that authenticates the archive end to end.

```bash
VERSION="v1.0.0"

# Download the checksums file and its signature
curl -LO "https://github.com/osuritz/radix/releases/download/${VERSION}/checksums.txt"
curl -LO "https://github.com/osuritz/radix/releases/download/${VERSION}/checksums.txt.asc"

# Import the radix release public key
# (the key location and fingerprint will be published here when signing goes live)
gpg --import radix-release-key.asc

# 1. Verify the signature on the checksums file
gpg --verify checksums.txt.asc checksums.txt

# 2. Then verify your archive against the now-authenticated checksums
sha256sum -c checksums.txt --ignore-missing
```

A successful verification prints `Good signature from ...`. Always compare the
reported key fingerprint against the fingerprint published in this document —
a "good" signature from an unknown key means nothing.

> **Note:** if a release predates the signing setup, `checksums.txt.asc` will
> simply not exist for that release; checksum verification above still applies.

### Maintainer note: configuring the signing key

Signing is enabled by setting the `GPG_PRIVATE_KEY` and `GPG_FINGERPRINT`
repository secrets (the release workflow skips signing when either is absent).
Two key setups are supported:

- **Passphrase-less key** — set only `GPG_PRIVATE_KEY` and `GPG_FINGERPRINT`.
- **Passphrase-protected key** — additionally set the `GPG_PASSPHRASE` secret
  to the key's passphrase. The release pipeline feeds it to `gpg` on stdin
  (`--pinentry-mode loopback --passphrase-fd 0`), so signing works on the
  headless CI runner with no pinentry prompt.

## Verifying a `go install` build

Binaries built with `go install github.com/osuritz/radix/cmd/radix@<tag>` are
compiled locally from source fetched through the Go module proxy, which
verifies module hashes against the public checksum database (`sum.golang.org`).
No additional verification steps are needed.
