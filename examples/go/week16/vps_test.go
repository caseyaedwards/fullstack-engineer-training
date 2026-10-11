package week16

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

var vpsDir = filepath.Join(repo, "deploy/vps")

// TestFirewallOpensThreePorts runs firewall.sh with a fake ufw first on
// PATH, which records its arguments, and checks every rule the script
// sets, in order. Nothing opens 6443.
func TestFirewallOpensThreePorts(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	fake := "#!/bin/sh\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "ufw"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", filepath.Join(vpsDir, "firewall.sh"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("firewall.sh: %v\n%s", err, out)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{
		"default deny incoming",
		"default allow outgoing",
		"allow 22/tcp",
		"allow 80/tcp",
		"allow 443/tcp",
		"allow from 10.42.0.0/16 to any",
		"allow from 10.43.0.0/16 to any",
		"--force enable",
		"status verbose",
	}
	if !slices.Equal(got, want) {
		t.Errorf("firewall.sh runs\n  ufw %s\nwant\n  ufw %s", strings.Join(got, "\n  ufw "), strings.Join(want, "\n  ufw "))
	}
}

// sshdPath returns the sshd binary, or "" when OpenSSH server is not
// installed. Ubuntu keeps it in /usr/sbin, which is not on the PATH of
// every user.
func sshdPath() string {
	if p, err := exec.LookPath("sshd"); err == nil {
		return p
	}
	if _, err := os.Stat("/usr/sbin/sshd"); err == nil {
		return "/usr/sbin/sshd"
	}
	return ""
}

// TestSSHDropInsWinAndConfineTheTunnelUser loads the two drop-in files
// next to a 50-cloud-init.conf that turns passwords on, the way Ubuntu
// includes /etc/ssh/sshd_config.d/*.conf, and prints the settings sshd
// applies with sshd -T. Passwords and root stay off for everyone, and the
// tunnel user can forward to 127.0.0.1:6443 and nothing else.
func TestSSHDropInsWinAndConfineTheTunnelUser(t *testing.T) {
	sshd := sshdPath()
	if sshd == "" {
		t.Skip("sshd is not installed; on Ubuntu: sudo apt install openssh-server")
	}
	dir := t.TempDir()
	for _, name := range []string{"00-hardening.conf", "10-tunnel.conf"} {
		b, err := os.ReadFile(filepath.Join(vpsDir, "sshd", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "50-cloud-init.conf"), []byte("PasswordAuthentication yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// sshd -T refuses to run without a host key.
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	hostKey := filepath.Join(dir, "host_ed25519")
	if err := os.WriteFile(hostKey, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "sshd_config")
	conf := "Include " + filepath.Join(dir, "*.conf") + "\nHostKey " + hostKey + "\nX11Forwarding yes\n"
	if err := os.WriteFile(main, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}

	effective := func(user string) map[string]string {
		out, err := exec.Command(sshd, "-T", "-f", main, "-C", "user="+user+",host=client.example,addr=203.0.113.5").CombinedOutput()
		if err != nil {
			t.Fatalf("sshd -T for %s: %v\n%s", user, err, out)
		}
		m := map[string]string{}
		for _, line := range strings.Split(string(out), "\n") {
			if k, v, ok := strings.Cut(line, " "); ok {
				m[k] = v
			}
		}
		return m
	}

	for _, user := range []string{"ada", "root", "tunnel"} {
		got := effective(user)
		for _, k := range []string{"passwordauthentication", "kbdinteractiveauthentication", "permitrootlogin"} {
			if got[k] != "no" {
				t.Errorf("user %s: %s is %q, want no", user, k, got[k])
			}
		}
	}
	tunnel := effective("tunnel")
	for k, v := range map[string]string{
		"allowtcpforwarding":   "local",
		"permitopen":           "127.0.0.1:6443",
		"permittty":            "no",
		"x11forwarding":        "no",
		"allowagentforwarding": "no",
		"forcecommand":         "/usr/sbin/nologin",
	} {
		if tunnel[k] != v {
			t.Errorf("user tunnel: %s is %q, want %q", k, tunnel[k], v)
		}
	}
	// The Match block ends with its file: other users keep the defaults.
	if ada := effective("ada"); ada["permitopen"] != "any" || ada["x11forwarding"] != "yes" {
		t.Errorf("user ada: permitopen %q and x11forwarding %q; the tunnel block leaks past its file", ada["permitopen"], ada["x11forwarding"])
	}
}
