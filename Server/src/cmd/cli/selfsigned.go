package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/weirdstreamer/server/internal/fileio"
)

// writeSelfSigned writes a self-signed certificate and its key for host into dir as cert.pem and
// key.pem: ECDSA P-256, valid for a year, for host, localhost and 127.0.0.1, as setup.sh's openssl
// command makes them. It refuses to write over either file. The key is readable by this account only.
func writeSelfSigned(dir, host string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	for _, p := range []string{certPath, keyPath} {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s already exists: move it aside first", p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	dnsNames := []string{host, "localhost"}
	if host == "localhost" {
		dnsNames = []string{"localhost"}
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return err
	}
	if err := fileio.OwnerOnly(keyPath); err != nil {
		return err
	}
	return os.WriteFile(certPath, certPEM, 0o644)
}

// handleSelfSigned is `cli config self-signed <dir> [host]`.
func handleSelfSigned(args []string) {
	if len(args) < 1 || args[0] == "" {
		fmt.Fprintln(os.Stderr, "Usage: cli config self-signed <dir> [host]   (host: this machine's name by default)")
		os.Exit(2)
	}
	dir := args[0]
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "localhost"
	}
	if len(args) >= 2 && args[1] != "" {
		host = args[1]
	}
	if err := writeSelfSigned(dir, host); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s and %s (valid for a year).\n",
		filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"))
}
