package backups

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// tree writes a small backup folder: a dump, its row counts and a nested WAL
// segment, and returns its path.
func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"monitor-20261011-031700.dump":   "PGDMP fake dump body\n",
		"monitor-20261011-031700.counts": "monitors 3\nresults 280240\n",
		"wal/000000010000000000000001":   "fake WAL segment\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// region: test

func TestVerifyIntactTree(t *testing.T) {
	dir := tree(t)
	m, err := WriteManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 {
		t.Fatalf("manifest has %d files, want 3: %v", len(m), m)
	}
	if got := Verify(dir, m); len(got) != 0 {
		t.Errorf("intact tree: problems %v, want none", got)
	}
}

func TestVerifyFindsEachKindOfDamage(t *testing.T) {
	dir := tree(t)
	m, err := WriteManifest(dir)
	if err != nil {
		t.Fatal(err)
	}

	// One flipped bit in the dump: same size, different content.
	dump := filepath.Join(dir, "monitor-20261011-031700.dump")
	b, _ := os.ReadFile(dump)
	b[0] ^= 0x01
	os.WriteFile(dump, b, 0o644)
	// A deleted WAL segment, and a file nobody listed.
	os.Remove(filepath.Join(dir, "wal", "000000010000000000000001"))
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("left behind\n"), 0o644)

	want := []Problem{
		{Path: "monitor-20261011-031700.dump", Kind: "changed"},
		{Path: "notes.txt", Kind: "extra"},
		{Path: "wal/000000010000000000000001", Kind: "missing"},
	}
	if got := Verify(dir, m); !reflect.DeepEqual(got, want) {
		t.Errorf("problems\n got %v\nwant %v", got, want)
	}
}

// endregion: test

// TestManifestFile checks the file WriteManifest leaves in the folder: the
// format of sha256sum, which `sha256sum -c SHA256SUMS` reads back. The
// manifest itself is not listed, and Verify does not report it as extra.
func TestManifestFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644)

	m, err := WriteManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	want := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03  hello.txt\n"
	if string(got) != want {
		t.Errorf("%s holds %q, want %q", ManifestName, got, want)
	}
	if p := Verify(dir, m); len(p) != 0 {
		t.Errorf("problems %v right after WriteManifest, want none", p)
	}
}

func TestVerifyMissingFolder(t *testing.T) {
	m := Manifest{"a.dump": "00"}
	want := []Problem{{Path: "a.dump", Kind: "missing"}}
	if got := Verify(filepath.Join(t.TempDir(), "gone"), m); !reflect.DeepEqual(got, want) {
		t.Errorf("problems %v, want %v", got, want)
	}
}

// TestSha256sumReadsManifest runs the coreutils sha256sum over the manifest,
// the check an operator runs on a copy without Go. It fails in CI without
// sha256sum and skips elsewhere, as the Week 17 backup tests do.
func TestSha256sumReadsManifest(t *testing.T) {
	if _, err := exec.LookPath("sha256sum"); err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal("sha256sum not found")
		}
		t.Skip("sha256sum not found")
	}
	dir := tree(t)
	if _, err := WriteManifest(dir); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sha256sum", "--check", "--strict", ManifestName)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sha256sum --check: %v\n%s", err, out)
	}

	os.WriteFile(filepath.Join(dir, "monitor-20261011-031700.dump"), []byte("PGDMP fake dump bodY\n"), 0o644)
	cmd = exec.Command("sha256sum", "--check", "--quiet", ManifestName)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("sha256sum --check passed a changed dump:\n%s", out)
	}
}
