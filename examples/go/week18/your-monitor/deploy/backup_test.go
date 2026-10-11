package deploy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week18/your-monitor/internal/db"
)

const scripts = "backup"

// need fails the test in CI and skips it elsewhere: CI must run every
// test, a laptop without the tools may not.
func need(t *testing.T, why string) {
	t.Helper()
	if os.Getenv("CI") == "true" {
		t.Fatal(why)
	}
	t.Skip(why)
}

// pgBin returns a folder that holds initdb, pg_ctl, pg_dump and
// pg_restore of one Postgres version: the folder of initdb on the PATH,
// or the newest /usr/lib/postgresql/<version>/bin, where Debian and
// Ubuntu install them. It also needs age and rclone on the PATH.
func pgBin(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		need(t, "initdb refuses to run as root: run this test as another user")
	}
	for _, tool := range []string{"age", "age-keygen", "rclone", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			need(t, tool+" not found: install age, rclone and curl to run this test")
		}
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
	need(t, "initdb not found: install the Postgres server tools to run this test")
	return ""
}

// live starts an empty Postgres server of its own in a temporary folder,
// reachable only through a socket there, with the database monitor
// migrated and holding two monitors and three results, and the database
// empty migrated and holding nothing. It returns the environment that
// points pg_dump and psql at monitor.
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
	ctx := context.Background()
	var pool *pgxpool.Pool
	for _, name := range []string{"empty", "monitor"} {
		run(t, nil, filepath.Join(bin, "createdb"), "--host="+dir, "--username=monitor", name)
		pool, err = pgxpool.New(ctx, fmt.Sprintf("host=%s user=monitor dbname=%s", dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		if err := db.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, `
		insert into monitors (url, interval_seconds) values ('https://example.com', 60), ('https://example.org', 60);
		insert into check_results (monitor_id, checked_at, up) select id, now(), true from monitors;
		insert into check_results (monitor_id, checked_at, up) select id, now(), false from monitors limit 1;`)
	if err != nil {
		t.Fatal(err)
	}
	return []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "PGHOST=" + dir, "PGUSER=monitor", "PGDATABASE=monitor",
		"RCLONE_CONFIG=/dev/null", "RCLONE_CONFIG_OFFSITE_TYPE=local"}
}

// backup runs backup.sh against the database of env, encrypting to the
// public key of identity, and uploads to the folder remote. It pings a
// stand-in for healthchecks.io, unless env sets BACKUP_PING_URL.
func backup(t *testing.T, env []string, identity, remote string) string {
	t.Helper()
	out, err := script(backupEnv(t, env, identity, remote), "sh", filepath.Join(scripts, "backup.sh"))
	if err != nil {
		t.Fatalf("backup.sh: %v\n%s", err, out)
	}
	return out
}

func backupEnv(t *testing.T, env []string, identity, remote string) []string {
	t.Helper()
	recipient := strings.TrimSpace(run(t, nil, "age-keygen", "-y", identity))
	ping, _ := pinger(t)
	return append(append([]string{"BACKUP_PING_URL=" + ping}, env...),
		"WORK_DIR="+t.TempDir(), "AGE_RECIPIENT="+recipient, "BACKUP_REMOTE=offsite:"+remote)
}

// pinger starts a stand-in for healthchecks.io. It returns the ping URL
// and a function that lists the paths the server has received so far.
func pinger(t *testing.T) (string, func() []string) {
	t.Helper()
	return pingerAnswering(t, http.StatusOK)
}

// pingerAnswering is pinger with a server that answers every ping with
// the status code.
func pingerAnswering(t *testing.T, code int) (string, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, r.URL.Path)
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/ping/pg-backup", func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(paths)
	}
}

// key writes a new age identity and returns the path of the file.
func key(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.txt")
	run(t, nil, "age-keygen", "-o", path)
	return path
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

// script runs a command with env added to the environment. An entry of the
// form "-NAME" removes NAME from the environment instead.
func script(env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	for _, e := range env {
		if name, ok := strings.CutPrefix(e, "-"); ok {
			cmd.Env = slices.DeleteFunc(cmd.Env, func(kv string) bool {
				return kv == e || strings.HasPrefix(kv, name+"=")
			})
		}
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// region: backup-test

// TestBackupUploadsOnlyCiphertext runs backup.sh: the remote gets the
// dump, encrypted with age, and the row counts of the same snapshot, and
// no unencrypted dump is left behind.
func TestBackupUploadsOnlyCiphertext(t *testing.T) {
	env := live(t, pgBin(t))
	remote := t.TempDir()
	ping, pings := pinger(t)
	out := backup(t, append(env, "BACKUP_PING_URL="+ping), key(t), remote)
	if got := pings(); !slices.Equal(got, []string{"/ping/pg-backup"}) {
		t.Errorf("healthchecks.io got pings %q, want one success ping, /ping/pg-backup", got)
	}
	if !regexp.MustCompile(`(?m)^backup: monitor-\d{8}-\d{6}\.dump\.age, \d+ KiB, counts 2 3 0 3, in \d+ s$`).MatchString(out) {
		t.Errorf("output:\n%s\nwant a line backup: monitor-<time>.dump.age, <n> KiB, counts 2 3 0 3, in <n> s", out)
	}
	files, _ := filepath.Glob(filepath.Join(remote, "*"))
	if len(files) != 2 || !strings.HasSuffix(files[0], ".counts") || !strings.HasSuffix(files[1], ".dump.age") {
		t.Fatalf("remote holds %v, want one .counts and one .dump.age", files)
	}
	head, _ := os.ReadFile(files[1])
	if !bytes.HasPrefix(head, []byte("age-encryption.org/v1\n")) {
		t.Errorf("%s is not an age file", files[1])
	}
	if counts, _ := os.ReadFile(files[0]); string(counts) != "2 3 0 3\n" {
		t.Errorf("counts %q, want \"2 3 0 3\\n\"", counts)
	}
}

// endregion: backup-test

// TestBackupPingsFailure runs backup.sh against a database that does not
// exist: it pings /fail first, then exits with an error.
func TestBackupPingsFailure(t *testing.T) {
	env := live(t, pgBin(t))
	ping, pings := pinger(t)
	out, err := script(backupEnv(t, append(env, "PGDATABASE=missing", "BACKUP_PING_URL="+ping), key(t), t.TempDir()),
		"sh", filepath.Join(scripts, "backup.sh"))
	if err == nil || !strings.Contains(out, "backup: FAILED: psql exited with") {
		t.Errorf("err %v, output:\n%s\nwant backup: FAILED: psql exited with", err, out)
	}
	if got := pings(); !slices.Equal(got, []string{"/ping/pg-backup/fail"}) {
		t.Errorf("healthchecks.io got pings %q, want one failure ping, /ping/pg-backup/fail", got)
	}
}

// TestBackupSurvivesAFailedSuccessPing runs backup.sh against a
// healthchecks.io that answers 500: curl tries the success ping four
// times, and the backup still exits 0, because the dump is uploaded and a
// retry of the Job would only take a second one.
func TestBackupSurvivesAFailedSuccessPing(t *testing.T) {
	env := live(t, pgBin(t))
	remote := t.TempDir()
	ping, pings := pingerAnswering(t, http.StatusInternalServerError)
	out, err := script(backupEnv(t, append(env, "BACKUP_PING_URL="+ping), key(t), remote),
		"sh", filepath.Join(scripts, "backup.sh"))
	if err != nil || !strings.Contains(out, "backup: the success ping failed") {
		t.Errorf("err %v, output:\n%s\nwant exit 0 and the line backup: the success ping failed", err, out)
	}
	want := []string{"/ping/pg-backup", "/ping/pg-backup", "/ping/pg-backup", "/ping/pg-backup"}
	if got := pings(); !slices.Equal(got, want) {
		t.Errorf("healthchecks.io got pings %q, want four tries of the success ping", got)
	}
	if files, _ := filepath.Glob(filepath.Join(remote, "*")); len(files) != 2 {
		t.Errorf("remote holds %v, want the dump and its counts", files)
	}
}

// TestBackupNeedsThePingURL: without BACKUP_PING_URL, backup.sh stops
// before it dumps anything.
func TestBackupNeedsThePingURL(t *testing.T) {
	out, err := script([]string{"BACKUP_PING_URL="}, "sh", filepath.Join(scripts, "backup.sh"))
	if err == nil || !strings.Contains(out, "set BACKUP_PING_URL in the offsite Secret") {
		t.Errorf("err %v, output:\n%s\nwant it to ask for BACKUP_PING_URL", err, out)
	}
}

// region: drill-test

// TestRestoreDrill takes a real backup, then runs the drill on copies of
// the remote: as it is, with a copy dated 2 minutes ahead, which the clock
// skew allows, and broken ten ways. Each way fails with a message that
// names its cause, which is what the reader sees in the drill log. The live
// database holds two monitors, which LIVE_MONITORS=2 tells the drill.
func TestRestoreDrill(t *testing.T) {
	env := live(t, pgBin(t))
	identity := key(t)
	good := t.TempDir()
	backup(t, env, identity, good)
	files, _ := filepath.Glob(filepath.Join(good, "*.dump.age"))
	dump := filepath.Base(files[0])

	// A whole backup of the empty database, dump and counts: what a
	// backup with the wrong PGDATABASE looks like. Its counts match its
	// dump; only the live count shows that it is the wrong database.
	emptied := t.TempDir()
	backup(t, append(env, "PGDATABASE=empty"), identity, emptied)
	stem := strings.TrimSuffix(dump, ".dump.age")
	// plant copies the good backup under the name of a backup taken at.
	plant := func(remote string, at time.Time) {
		name := at.UTC().Format("monitor-20060102-150405")
		for _, ext := range []string{".dump.age", ".counts"} {
			b, _ := os.ReadFile(filepath.Join(remote, stem+ext))
			os.WriteFile(filepath.Join(remote, name+ext), b, 0o644)
		}
	}

	tests := []struct {
		name  string
		env   []string
		spoil func(remote string)
		want  string
	}{
		{"a good backup", nil, func(string) {}, "restore drill: ok, restore "},
		{"no backup at all", nil, func(r string) { os.RemoveAll(r); os.Mkdir(r, 0o755) }, "FAILED: no dump"},
		{"the newest backup is too old", []string{"MAX_AGE_HOURS=0"}, func(string) {}, "hours old"},
		{"the newest backup is 2 minutes ahead, inside the clock skew", nil, func(r string) {
			plant(r, time.Now().Add(2*time.Minute))
		}, "restore drill: ok, restore "},
		{"the newest backup is 10 minutes ahead", nil, func(r string) {
			plant(r, time.Now().Add(10*time.Minute))
		}, "is dated more than 5 minutes in the future"},
		{"the dump is cut short", nil, func(r string) { os.Truncate(filepath.Join(r, dump), 2000) }, "FAILED: age could not decrypt"},
		{"another key encrypted it", []string{"AGE_IDENTITY=" + key(t)}, func(string) {}, "FAILED: age could not decrypt"},
		{"the counts differ", nil, func(r string) {
			os.WriteFile(filepath.Join(r, stem+".counts"), []byte("2 4 0 3\n"), 0o644)
		}, "FAILED: the restored counts differ"},
		{"LIVE_MONITORS is unset", []string{"-LIVE_MONITORS"}, func(string) {}, "FAILED: LIVE_MONITORS must be the monitor count"},
		{"LIVE_MONITORS is empty", []string{"LIVE_MONITORS="}, func(string) {}, "FAILED: LIVE_MONITORS must be the monitor count"},
		{"LIVE_MONITORS is not a number", []string{"LIVE_MONITORS=two"}, func(string) {}, "FAILED: LIVE_MONITORS must be the monitor count"},
		{"a backup of the empty database", nil, func(r string) {
			os.RemoveAll(r)
			os.CopyFS(r, os.DirFS(emptied))
		}, "FAILED: the restored database has no monitors, the live one has 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := t.TempDir()
			if err := os.CopyFS(remote, os.DirFS(good)); err != nil {
				t.Fatal(err)
			}
			tt.spoil(remote)
			drill := append(env, "BACKUP_REMOTE=offsite:"+remote, "AGE_IDENTITY="+identity,
				"LIVE_MONITORS=2", "BACKUP_DIR="+t.TempDir(), "DRILL_DIR="+shortDir(t))
			out, err := script(append(drill, tt.env...), "sh", filepath.Join(scripts, "restore-drill.sh"))
			if ok := strings.HasPrefix(tt.want, "restore drill: ok"); ok != (err == nil) || !strings.Contains(out, tt.want) {
				t.Errorf("err %v, output:\n%s\nwant it to contain %q", err, out, tt.want)
			}
		})
	}
}

// endregion: drill-test

// shortDir returns a new temporary folder with a short path: the drill
// puts the socket of its server there, and a socket path holds at most
// 107 bytes.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "drill")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
