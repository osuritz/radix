# radix gencert

Generate self-signed certificates for local HTTPS and mTLS.

```bash
radix gencert [flags]
```

By default it generates a CA and a server certificate signed by that CA, written
to `./certs`. Import the CA into your trust store once and browsers stop warning.

## When to use it

Get local HTTPS working without a public CA: serve `radix serve --tls`, test an
HTTPS proxy, or set up mutual TLS with a client certificate.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--output`, `-o` | `./certs` | Output directory |
| `--host` | `localhost` | Comma-separated hostnames/IPs for the SANs |
| `--ca` | `true` | Generate a CA certificate |
| `--ca-cert` | | Path to an existing CA certificate to sign with |
| `--ca-key` | | Path to an existing CA private key |
| `--client` | `false` | Generate a client certificate instead of a server one |
| `--days` | `365` | Validity period in days |
| `--key-type` | `rsa` | Key type: `rsa` or `ecdsa` |
| `--key-size` | `2048` | RSA key size: `2048` or `4096` |
| `--ecdsa-curve` | `P-256` | ECDSA curve: `P-256`, `P-384`, `P-521` |
| `--org` | `Radix Development` | Organization name in the certificate subject |
| `--overwrite` | `false` | Overwrite existing certificate files |

## Output files

The default run writes five files to the output directory:

| File | What it is |
|------|------------|
| `ca.pem` | CA certificate — import this into your trust store |
| `ca-key.pem` | CA private key |
| `cert.pem` | Server certificate |
| `key.pem` | Server private key |
| `README.txt` | Usage notes and trust-store commands |

## Examples

### CA + server cert for multiple hosts

```bash
radix gencert --host localhost,127.0.0.1,myapp.local --output ./certs
```

Then serve over HTTPS:

```bash
radix serve --tls --cert ./certs/cert.pem --key ./certs/key.pem
```

### Reuse an existing CA

Generate one CA, then sign multiple server/client certs with it so you only
trust the CA once:

```bash
radix gencert --ca-cert ./certs/ca.pem --ca-key ./certs/ca-key.pem \
  --host api.local --output ./certs-api
```

### Client certificate for mTLS

```bash
radix gencert --client --ca-cert ./certs/ca.pem --ca-key ./certs/ca-key.pem
```

### ECDSA keys

```bash
radix gencert --key-type ecdsa --ecdsa-curve P-384
```

See the [TLS guide](/guides/tls) for trusting the CA and configuring mTLS.
