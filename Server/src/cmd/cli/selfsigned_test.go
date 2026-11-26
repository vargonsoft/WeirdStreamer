package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWriteSelfSigned(t *testing.T) {
	dir := t.TempDir()
	if err := writeSelfSigned(dir, "media-box"); err != nil {
		t.Fatalf("writeSelfSigned: %v", err)
	}
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("cert.pem has no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if !slices.Contains(cert.DNSNames, "media-box") || !slices.Contains(cert.DNSNames, "localhost") {
		t.Errorf("DNSNames = %v, want media-box and localhost", cert.DNSNames)
	}
	hasLoopback := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			hasLoopback = true
		}
	}
	if !hasLoopback {
		t.Errorf("IPAddresses = %v, want 127.0.0.1", cert.IPAddresses)
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		t.Errorf("certificate not valid now: %v to %v", cert.NotBefore, cert.NotAfter)
	}
	if days := cert.NotAfter.Sub(now).Hours() / 24; days < 360 || days > 370 {
		t.Errorf("expires in %.0f days, want about a year", days)
	}

	// A second call must refuse and leave both files alone.
	certBefore, _ := os.ReadFile(certPath)
	keyBefore, _ := os.ReadFile(keyPath)
	if err := writeSelfSigned(dir, "media-box"); err == nil {
		t.Fatal("second writeSelfSigned into the same dir succeeded, want an error")
	}
	certAfter, _ := os.ReadFile(certPath)
	keyAfter, _ := os.ReadFile(keyPath)
	if !bytes.Equal(certBefore, certAfter) || !bytes.Equal(keyBefore, keyAfter) {
		t.Error("a refused write changed the existing files")
	}
}
