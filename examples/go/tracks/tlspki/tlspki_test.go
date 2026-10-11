package tlspki

import (
	"crypto/x509"
	"errors"
	"testing"
	"time"
)

// now is a fixed clock: every test passes the time in, and none reads the
// wall clock, which keeps them from breaking on the day a certificate ends.
var now = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

// issue makes a CA and a 90-day leaf for status.example.com.
func issue(t *testing.T) (leaf, root *x509.Certificate) {
	t.Helper()
	root, key, err := NewCA("Your Monitor Test CA", now)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err = IssueLeaf(root, key, "status.example.com", now, 90*day)
	if err != nil {
		t.Fatal(err)
	}
	return leaf, root
}

// region: test

func TestFreshLeafVerifies(t *testing.T) {
	leaf, root := issue(t)
	if err := Verify(leaf, root, "status.example.com", now.Add(time.Minute)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestWrongNameFails(t *testing.T) {
	leaf, root := issue(t)
	err := Verify(leaf, root, "api.example.com", now.Add(time.Minute))
	var hostErr x509.HostnameError
	if !errors.As(err, &hostErr) {
		t.Fatalf("Verify for another name: %v, want x509.HostnameError", err)
	}
}

func TestExpiredLeafFails(t *testing.T) {
	leaf, root := issue(t)
	err := Verify(leaf, root, "status.example.com", now.Add(91*day))
	var invalid x509.CertificateInvalidError
	if !errors.As(err, &invalid) || invalid.Reason != x509.Expired {
		t.Fatalf("Verify after expiry: %v, want x509.CertificateInvalidError with reason Expired", err)
	}
}

func TestDaysLeft(t *testing.T) {
	leaf, _ := issue(t)
	if got := DaysLeft(leaf, now.Add(30*day)); got != 60 {
		t.Errorf("DaysLeft 30 days into a 90-day leaf = %d, want 60", got)
	}
	if got := DaysLeft(leaf, now.Add(91*day)); got != -1 {
		t.Errorf("DaysLeft one day after expiry = %d, want -1", got)
	}
}

// endregion: test

// TestUnknownRootFails checks the chain itself: a leaf from one CA does not
// verify against the root of another, even with the right name and time.
func TestUnknownRootFails(t *testing.T) {
	leaf, _ := issue(t)
	other, _, err := NewCA("Someone Else CA", now)
	if err != nil {
		t.Fatal(err)
	}
	err = Verify(leaf, other, "status.example.com", now.Add(time.Minute))
	var unknown x509.UnknownAuthorityError
	if !errors.As(err, &unknown) {
		t.Fatalf("Verify against another root: %v, want x509.UnknownAuthorityError", err)
	}
}

// TestCertificateFields checks what makes a CA a CA, and what limits a leaf
// to serving one name over TLS.
func TestCertificateFields(t *testing.T) {
	leaf, root := issue(t)
	if !root.IsCA || root.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Errorf("root: IsCA %v, KeyUsage %b, want a CA that can sign certificates", root.IsCA, root.KeyUsage)
	}
	if leaf.IsCA {
		t.Error("leaf is a CA: anyone holding its key can issue certificates")
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "status.example.com" {
		t.Errorf("leaf DNSNames %v, want [status.example.com]", leaf.DNSNames)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("leaf ExtKeyUsage %v, want server authentication only", leaf.ExtKeyUsage)
	}
	if leaf.SerialNumber.Cmp(root.SerialNumber) == 0 {
		t.Error("leaf and root share a serial number")
	}
}
