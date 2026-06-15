# TLS / HTTPS guide

Run radix over HTTPS with self-signed certificates, redirect plain HTTP, send
HSTS, and enable mutual TLS — all locally, no public CA.

## 1. Generate certificates

[`radix gencert`](/commands/gencert) creates a CA and a server certificate
signed by it:

```bash
radix gencert --host localhost,127.0.0.1 --output ./certs
```

This writes five files to `./certs`:

| File | What it is |
|------|------------|
| `ca.pem` | CA certificate — import into your trust store |
| `ca-key.pem` | CA private key |
| `cert.pem` | Server certificate |
| `key.pem` | Server private key |
| `README.txt` | Usage and trust-store commands |

Pass every hostname and IP you'll use in `--host` so they end up in the
certificate SANs; otherwise browsers reject the name.

## 2. Trust the CA

Import `ca.pem` once and your browser/OS stops warning about the self-signed
cert:

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain ./certs/ca.pem

# Linux
sudo cp ./certs/ca.pem /usr/local/share/ca-certificates/radix-ca.crt && \
  sudo update-ca-certificates

# Windows
certutil -addstore -f "ROOT" .\certs\ca.pem
```

Firefox keeps its own store: **Preferences → Privacy & Security → View
Certificates → Import** `ca.pem`.

## 3. Serve over HTTPS

Point any command at the server cert and key with `--tls`:

```bash
radix serve --tls --cert ./certs/cert.pem --key ./certs/key.pem
```

`--tls` works the same way for `proxy`, `echo`, and `mock`.

### Minimum TLS version

Negotiation defaults to TLS 1.2. Require 1.3 with:

```bash
radix serve --tls --cert ./certs/cert.pem --key ./certs/key.pem \
  --tls-min-version 1.3
```

## 4. Redirect HTTP to HTTPS + HSTS

Serve HTTPS on one port and run a plain-HTTP listener that 308-redirects to it.
The redirect port must differ from the HTTPS port. Add `--hsts` to send
`Strict-Transport-Security`:

```bash
radix serve --tls --cert ./certs/cert.pem --key ./certs/key.pem \
  --port 8443 \
  --http-redirect --http-port 8080 \
  --hsts --hsts-max-age 31536000
```

::: warning
`--http-redirect` and `--hsts` both require `--tls`. `--http-port` must differ
from `--port`. `--hsts-max-age 0` clears the policy rather than setting it to
zero seconds.
:::

## 5. Mutual TLS (mTLS)

Require clients to present a certificate signed by your CA with `--client-auth`
and `--ca`:

```bash
radix echo --tls \
  --cert ./certs/cert.pem --key ./certs/key.pem \
  --client-auth --ca ./certs/ca.pem
```

Generate a client certificate from the same CA. Write it to a **separate**
output directory — `gencert` always names the pair `cert.pem`/`key.pem`, so
reusing `./certs` would clash with the server cert:

```bash
radix gencert --client --output ./certs-client \
  --ca-cert ./certs/ca.pem --ca-key ./certs/ca-key.pem
```

Then connect with it:

```bash
curl --cacert ./certs/ca.pem \
  --cert ./certs-client/cert.pem --key ./certs-client/key.pem \
  https://localhost:8080/anything
```

With `radix echo`, the response's `tls.client_cert` block reports the
certificate the client presented — a quick way to confirm mTLS is working. See
the [echo response shape](/commands/echo#response-shape).

## Backend TLS (proxy)

When proxying to an HTTPS backend, relax verification for a self-signed backend
with `--tls-skip-verify`, or supply backend CA/client certs in the config file
(`proxy.backend_ca`, `proxy.backend_cert`, `proxy.backend_key`) for mTLS to the
upstream. See [Configuration](/configuration#radix-yml-keys).

```bash
radix proxy https://backend.internal --tls-skip-verify
```
