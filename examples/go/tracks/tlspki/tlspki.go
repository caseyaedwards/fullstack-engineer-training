// Package tlspki builds a two-level public key infrastructure with
// crypto/x509: a root certificate authority, a leaf certificate for one
// DNS name, and the checks a TLS client runs on that leaf.
package tlspki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math"
	"math/big"
	"time"
)

// region: ca

// NewCA creates a self-signed root certificate authority named cn, valid
// for ten years from now, and returns its certificate and private key.
func NewCA(cn string, now time.Time) (*x509.Certificate, crypto.Signer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true, // writes IsCA into the certificate
		MaxPathLenZero:        true, // signs leaves only, no intermediates
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	// Template and parent are the same certificate: the CA signs itself.
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, key, err
}

// newSerial returns a random 128-bit serial number. A CA must never give
// two certificates the same serial, and random serials need no counter.
func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

// endregion: ca

// region: leaf

// IssueLeaf has the CA sign a certificate for dnsName, valid from now for
// validFor. The leaf key is generated here and thrown away: the example
// checks certificates, not handshakes. With ACME, the client generates the
// key and sends the CA only a certificate signing request.
func IssueLeaf(ca *x509.Certificate, key crypto.Signer, dnsName string, now time.Time, validFor time.Duration) (*x509.Certificate, error) {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName}, // the subject alternative name clients check
		NotBefore:    now,
		NotAfter:     now.Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, leafKey.Public(), key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

// endregion: leaf

// region: verify

// Verify runs the checks of a TLS client: leaf is valid at now, names name,
// and chains to root. It returns x509.CertificateInvalidError for a leaf
// outside its dates, x509.HostnameError for another name, and
// x509.UnknownAuthorityError when root did not sign it.
func Verify(leaf, root *x509.Certificate, name string, now time.Time) error {
	roots := x509.NewCertPool()
	roots.AddCert(root)
	_, err := leaf.Verify(x509.VerifyOptions{
		DNSName:     name,
		Roots:       roots,
		CurrentTime: now, // the zero value means the clock of the machine
	})
	return err
}

// DaysLeft returns the whole days from now until c expires, rounded down.
// It is negative once c has expired.
func DaysLeft(c *x509.Certificate, now time.Time) int {
	return int(math.Floor(c.NotAfter.Sub(now).Hours() / 24))
}

// endregion: verify
