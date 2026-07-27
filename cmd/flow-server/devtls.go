package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// devTLSConfig returns a TLS config carrying a self-signed certificate for
// localhost, used ONLY in dev (FLOW_DEV=1). Serving over TLS lets browsers
// negotiate HTTP/2, which multiplexes the persistent SSE stream and ordinary
// page loads over a single connection — avoiding the HTTP/1.1 6-connections-
// per-host starvation that makes the app appear to hang after a few tabs.
//
// The cert+key are generated once and cached under dir so the browser sees a
// stable certificate across dev restarts (accept it once, or enable
// chrome://flags/#allow-insecure-localhost). NEVER use this in production.
func devTLSConfig(dir string) (*tls.Config, error) {
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "flow-dev localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		// Leaf server cert: only DigitalSignature. keyCertSign would require
		// CA:TRUE (RFC 5280 §4.2.1.3); pairing it with CA:FALSE makes the cert
		// non-standards-compliant — Go rejects it and Chrome hard-fails it
		// (ERR_SSL_SERVER_CERT_BAD_FORMAT, not even bypassable).
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// Best-effort cache so the cert is stable across restarts.
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = os.WriteFile(certPath, certPEM, 0o600)
		_ = os.WriteFile(keyPath, keyPEM, 0o600)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}
