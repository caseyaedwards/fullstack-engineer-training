package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week17/your-monitor/internal/db"
)

const scripts = "k8s/overlays/prod/backup"

// pgBin returns a folder that holds initdb, pg_ctl, pg_dump and
// pg_restore of one Postgres version: the folder of initdb on the PATH,
// or the newest /usr/lib/postgresql/<version>/bin, where Debian and
// Ubuntu install them. Without them, the test fails in CI and is skipped
// elsewhere. initdb refuses to run as root.
func pgBin(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("initdb refuses to run as root: run this test as another user")
	}
	if p, err := exec.LookPath("initdb"); err == nil {
		return filepath.Dir(p)
	}
	dirs, _ := filepath.Glob("/usr/lib/postgresql/*/bin")
	slices.SortFunc(dirs, func(a, b string) int { // 9 sorts before 16
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	})
	if len(dirs) > 0 {
		dir := dirs[len(dirs)-1]
		if _, err := os.Stat(filepath.Join(dir, "initdb")); err == nil {
			return dir
		}
	}
	if os.Getenv("CI") == "true" {
		t.Fatal("initdb not found: CI must provide the Postgres server tools")
	}
	t.Skip("initdb not found: install the Postgres server tools to run this test")
	return ""
}

// live starts an empty Postgres server of its own in a temporary folder,
// reachable only through a socket there, with the database monitor
// migrated and holding two monitors and three results. It returns the
// environment that points pg_dump and psql at it.
func live(t *testing.T, bin string) []string {
	t.Helper()
	// A short folder: a Unix socket path holds at most 107 bytes.
	dir, err := os.MkdirTemp("", "pg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	run(t, nil, filepath.Join(bin, "initdb"), "--pgdata="+dir+"/data", "--username=monitor", "--auth=trust")
	run(t, nil, filepath.Join(bin, "pg_ctl"), "--pgdata="+dir+"/data", "--options=-c listen_addresses='' -k "+dir,
		"--log="+dir+"/log", "--wait", "start")
	t.Cleanup(func() {
		exec.Command(filepath.Join(bin, "pg_ctl"), "--pgdata="+dir+"/data", "--mode=immediate", "stop").Run()
	})
	run(t, nil, filepath.Join(bin, "createdb"), "--host="+dir, "--username=monitor", "monitor")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, fmt.Sprintf("host=%s user=monitor dbname=monitor", dir))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		insert into monitors (url, interval_seconds) values ('https://example.com', 60), ('https://example.org', 60);
		insert into check_results (monitor_id, checked_at, up) select id, now(), true from monitors;
		insert into check_results (monitor_id, checked_at, up) select id, now(), false from monitors limit 1;`)
	if err != nil {
		t.Fatal(err)
	}
	return []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "PGHOST=" + dir, "PGUSER=monitor", "PGDATABASE=monitor"}
}

// run runs a command with env added to the environment, and fails the
// test when it fails.
func run(t *testing.T, env []string, name string, args ...string) string {
	t.Helper()
	out, err := script(env, name, args...)
	if err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
	return out
}

func script(env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// region: backup-test

// TestBackupKeepsFourteenDays runs backup.sh: it writes one new dump and
// deletes the dumps that are 14 or more days old, and the leftovers of a
// dump that never finished.
func TestBackupKeepsFourteenDays(t *testing.T) {
	env := live(t, pgBin(t))
	backups := t.TempDir()
	now := time.Now()
	old := map[string]time.Duration{
		"monitor-20260901-031700.dump":         14*24*time.Hour + time.Hour, // deleted
		"monitor-20260902-031700.dump":         13*24*time.Hour + time.Hour, // kept
		"monitor-20260903-031700.dump.partial": 2 * time.Hour,               // deleted
	}
	for name, age := range old {
		p := filepath.Join(backups, name)
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	out := run(t, append(env, "BACKUP_DIR="+backups), "sh", filepath.Join(scripts, "backup.sh"))
	if !regexp.MustCompile(`(?m)^backup: monitor-\d{8}-\d{6}\.dump, \d+ KiB in \d+ s$`).MatchString(out) {
		t.Errorf("output:\n%s\nwant a line backup: monitor-<time>.dump, <n> KiB in <n> s", out)
	}
	left, _ := filepath.Glob(filepath.Join(backups, "*"))
	var names []string
	for _, p := range left {
		names = append(names, filepath.Base(p))
	}
	if len(names) != 2 || names[0] != "monitor-20260902-031700.dump" || !strings.HasPrefix(names[1], "monitor-2") {
		t.Errorf("backups left: %v, want the 13-day-old dump and the new one", names)
	}
}

// endregion: backup-test

// region: drill-test

// TestRestoreDrill takes a real dump and restores it with the drill, then
// breaks the drill three ways: no dump, a dump too old, a dump cut short.
func TestRestoreDrill(t *testing.T) {
	bin := pgBin(t)
	env := live(t, bin)
	backups := t.TempDir()
	run(t, append(env, "BACKUP_DIR="+backups), "sh", filepath.Join(scripts, "backup.sh"))

	drill := func(extra ...string) (string, error) {
		work, err := os.MkdirTemp("", "drill")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(work) })
		return script(append(append(env, "BACKUP_DIR="+backups, "DRILL_DIR="+work), extra...),
			"sh", filepath.Join(scripts, "restore-drill.sh"))
	}

	out, err := drill()
	if err != nil {
		t.Fatalf("drill: %v\n%s", err, out)
	}
	for _, want := range []string{"restored: 2 3 0 3\n", "live:     2 3 0 3\n", "restore drill: ok, restore "} {
		if !strings.Contains(out, want) {
			t.Errorf("drill output:\n%s\nwant it to contain %q", out, want)
		}
	}

	if out, err := drill("MAX_AGE_HOURS=0"); err == nil || !strings.Contains(out, "hours old") {
		t.Errorf("dump older than MAX_AGE_HOURS: err %v, output:\n%s", err, out)
	}

	dumps, _ := filepath.Glob(filepath.Join(backups, "monitor-*.dump"))
	if err := os.Truncate(dumps[0], 2000); err != nil {
		t.Fatal(err)
	}
	if out, err := drill(); err == nil || !strings.Contains(out, "restore drill: FAILED: pg_restore") {
		t.Errorf("dump cut short: err %v, output:\n%s", err, out)
	}

	os.Remove(dumps[0])
	if out, err := drill(); err == nil || !strings.Contains(out, "no dump") {
		t.Errorf("no dump: err %v, output:\n%s", err, out)
	}
}

// endregion: drill-test
