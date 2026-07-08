package cli

import (
	"bytes"
	"net"
	"path/filepath"
	"testing"
)

// occupiedPort returns a 127.0.0.1 port that stays bound for the lifetime of
// the test (the backing listener is closed via t.Cleanup). A server configured
// with this port fails its own bind deterministically, which lets tests drive
// the full body of a run* command (handler construction, middleware chain, TLS
// setup, admin server) without ever needing to stop a running server.
func occupiedPort(t *testing.T) int {
	t.Helper()
	ln, _ := boundListener(t)
	return ln.Addr().(*net.TCPAddr).Port
}

// resetGencertFlags resets every gencert flag global to its registered default
// and restores the previous values when the test finishes. The gencert flags
// live in package globals bound by init(), so tests that mutate them must go
// through this to stay isolated.
func resetGencertFlags(t *testing.T) {
	t.Helper()
	oldHosts, oldOutput, oldDays, oldOrg := gencertHosts, gencertOutput, gencertDays, gencertOrg
	oldKeySize, oldKeyType, oldCurve := gencertKeySize, gencertKeyType, gencertECDSACurve
	oldCA, oldCACert, oldCAKey := gencertCA, gencertCACert, gencertCAKey
	oldClient, oldOverwrite := gencertClient, gencertOverwrite
	t.Cleanup(func() {
		gencertHosts, gencertOutput, gencertDays, gencertOrg = oldHosts, oldOutput, oldDays, oldOrg
		gencertKeySize, gencertKeyType, gencertECDSACurve = oldKeySize, oldKeyType, oldCurve
		gencertCA, gencertCACert, gencertCAKey = oldCA, oldCACert, oldCAKey
		gencertClient, gencertOverwrite = oldClient, oldOverwrite
	})

	gencertHosts = "localhost"
	gencertOutput = "./certs"
	gencertDays = 365
	gencertOrg = "Radix Development"
	gencertKeySize = 2048
	gencertKeyType = "rsa"
	gencertECDSACurve = "P-256"
	gencertCA = true
	gencertCACert = ""
	gencertCAKey = ""
	gencertClient = false
	gencertOverwrite = false
}

// genTestCerts generates a CA + server certificate (ECDSA, for speed) into a
// temp directory via the real gencert command and returns the cert, key, and
// CA paths. Useful for tests that need a working TLS configuration.
func genTestCerts(t *testing.T) (certPath, keyPath, caPath string) {
	t.Helper()
	resetGencertFlags(t)

	dir := t.TempDir()
	gencertOutput = dir
	gencertKeyType = "ecdsa"

	var buf bytes.Buffer
	gencertCmd.SetOut(&buf)
	t.Cleanup(func() { gencertCmd.SetOut(nil) })

	if err := runGencert(gencertCmd, nil); err != nil {
		t.Fatalf("failed to generate test certificates: %v", err)
	}
	return filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"), filepath.Join(dir, "ca.pem")
}
