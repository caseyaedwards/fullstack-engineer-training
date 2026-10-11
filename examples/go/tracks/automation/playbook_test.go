package automation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// play holds the parts of an Ansible play that the checks read. A task is a
// map, because its module is a key whose name varies.
type play struct {
	Name      string           `yaml:"name"`
	Hosts     string           `yaml:"hosts"`
	Vars      map[string]any   `yaml:"vars"`
	PreTasks  []map[string]any `yaml:"pre_tasks"`
	Tasks     []map[string]any `yaml:"tasks"`
	PostTasks []map[string]any `yaml:"post_tasks"`
	Handlers  []map[string]any `yaml:"handlers"`
}

// keywords are the task keys that are not a module, from the task keyword
// list of ansible-core. Every with_ key, such as with_items, is a keyword
// too. The one key of a task that is not a keyword names its module.
var keywords = []string{
	"name", "notify", "listen", "when", "register", "become", "become_user",
	"become_method", "become_flags", "become_exe", "remote_user", "connection",
	"port", "loop", "loop_control", "tags", "vars", "args", "environment",
	"changed_when", "failed_when", "check_mode", "diff", "ignore_errors",
	"ignore_unreachable", "any_errors_fatal", "no_log", "retries", "delay",
	"until", "delegate_to", "delegate_facts", "run_once", "async", "poll",
	"module_defaults", "collections", "debugger", "block", "rescue", "always",
	"timeout", "throttle",
}

func keyword(k string) bool {
	return slices.Contains(keywords, k) || strings.HasPrefix(k, "with_")
}

// commands are the modules that run whatever you hand them. Ansible cannot
// tell whether such a task changed anything.
var commands = []string{
	"ansible.builtin.command", "ansible.builtin.shell",
	"ansible.legacy.command", "ansible.legacy.shell", "command", "shell",
}

// fqcn matches a fully qualified collection name: namespace.collection.module.
var fqcn = regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+\.[a-z0-9_]+$`)

// region: checks

// problems returns every rule that the playbook in src breaks: a task or
// handler without a name, a module without its fully qualified name, a
// notify that names no handler, and a command or shell task that cannot
// report changed honestly.
func problems(src []byte) []string {
	var plays []play
	if err := yaml.Unmarshal(src, &plays); err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, p := range plays {
		handlers := map[string]bool{}
		for _, h := range p.Handlers {
			handlers[str(h["name"])] = true
			for _, topic := range list(h["listen"]) {
				handlers[topic] = true
			}
		}
		var tasks []map[string]any
		for _, group := range [][]map[string]any{p.PreTasks, p.Tasks, p.PostTasks, p.Handlers} {
			tasks = append(tasks, flatten(group)...)
		}
		for i, task := range tasks {
			name := str(task["name"])
			if name == "" {
				name = fmt.Sprintf("task %d", i+1)
				out = append(out, fmt.Sprintf("play %q: %s has no name", p.Name, name))
			}
			modules := modulesOf(task)
			if len(modules) == 0 {
				out = append(out, fmt.Sprintf("%q runs no module", name))
				continue
			}
			if len(modules) > 1 {
				out = append(out, fmt.Sprintf("%q holds more than one module: %s", name, strings.Join(modules, ", ")))
				continue
			}
			module := modules[0]
			if !fqcn.MatchString(module) {
				out = append(out, fmt.Sprintf("%q calls %s, want its fully qualified name", name, module))
			}
			for _, n := range list(task["notify"]) {
				if !handlers[n] {
					out = append(out, fmt.Sprintf("%q notifies %q, and no handler has that name", name, n))
				}
			}
			if slices.Contains(commands, module) && !honest(task, module) {
				out = append(out, fmt.Sprintf("%q runs %s without creates, removes or changed_when", name, module))
			}
		}
	}
	slices.Sort(out)
	return out
}

// honest reports whether a command or shell task can tell Ansible the
// truth about change: creates or removes skip it when the file says the
// work is done, and changed_when decides from its result.
func honest(task map[string]any, module string) bool {
	if _, ok := task["changed_when"]; ok {
		return true
	}
	for _, args := range []any{task[module], task["args"]} {
		switch a := args.(type) {
		case map[string]any:
			if a["creates"] != nil || a["removes"] != nil {
				return true
			}
		case string:
			if strings.Contains(a, "creates=") || strings.Contains(a, "removes=") {
				return true
			}
		}
	}
	return false
}

// endregion: checks

// flatten returns the tasks of a list, with the tasks inside each block,
// rescue and always section in place of the block itself.
func flatten(tasks []map[string]any) []map[string]any {
	var out []map[string]any
	for _, t := range tasks {
		nested := false
		for _, section := range []string{"block", "rescue", "always"} {
			if inner, ok := t[section].([]any); ok {
				nested = true
				for _, it := range inner {
					if m, ok := it.(map[string]any); ok {
						out = append(out, flatten([]map[string]any{m})...)
					}
				}
			}
		}
		if !nested {
			out = append(out, t)
		}
	}
	return out
}

// modulesOf returns the keys of a task that are not keywords, sorted. A
// valid task has exactly one: its module.
func modulesOf(task map[string]any) []string {
	var out []string
	for k := range task {
		if !keyword(k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// list reads a value that Ansible accepts as one string or a list of them.
func list(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, str(e))
		}
		return out
	}
	return nil
}

// region: playbook-test

// TestPlaybook checks playbook.yml itself: it must break no rule.
func TestPlaybook(t *testing.T) {
	src, err := os.ReadFile("playbook.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems(src) {
		t.Error("playbook.yml: " + p)
	}
}

// endregion: playbook-test

// TestTemplateUsesDefinedVariables checks that every template the playbook
// names exists, and that each variable it reads is a play variable or one
// that Ansible sets itself.
func TestTemplateUsesDefinedVariables(t *testing.T) {
	src, err := os.ReadFile("playbook.yml")
	if err != nil {
		t.Fatal(err)
	}
	var plays []play
	if err := yaml.Unmarshal(src, &plays); err != nil {
		t.Fatal(err)
	}
	builtin := map[string]bool{"ansible_managed": true, "inventory_hostname": true}
	used := 0
	for _, p := range plays {
		for _, task := range flatten(p.Tasks) {
			args, ok := task["ansible.builtin.template"].(map[string]any)
			if !ok {
				continue
			}
			used++
			b, err := os.ReadFile(filepath.Join("templates", str(args["src"])))
			if err != nil {
				t.Fatalf("%q: %v", task["name"], err)
			}
			for _, m := range regexp.MustCompile(`\{\{\s*(\w+)`).FindAllStringSubmatch(string(b), -1) {
				if _, ok := p.Vars[m[1]]; !ok && !builtin[m[1]] {
					t.Errorf("templates/%s reads %s, which the play does not define", args["src"], m[1])
				}
			}
		}
	}
	if used == 0 {
		t.Error("playbook.yml has no ansible.builtin.template task")
	}
}

// TestChecksCatchMistakes breaks a copy of playbook.yml once per rule and
// expects that rule, and only that rule, to fail.
func TestChecksCatchMistakes(t *testing.T) {
	good, err := os.ReadFile("playbook.yml")
	if err != nil {
		t.Fatal(err)
	}
	src := string(good)
	tests := []struct {
		name, old, new, want string
	}{
		{"no name", "    - name: Install chrony\n      ansible.builtin.package:", "    - ansible.builtin.package:", `play "Keep the clock in sync with chrony": task 1 has no name`},
		{"short module name", "      ansible.builtin.package:", "      package:", `"Install chrony" calls package, want its fully qualified name`},
		{"unknown handler", "      notify: Restart chrony", "      notify: Restart chronyd", `"Write chrony.conf" notifies "Restart chronyd", and no handler has that name`},
		{"command without changed_when", "      changed_when: false\n", "", `"Read the time sources" runs ansible.builtin.command without creates, removes or changed_when`},
		{"two modules", "      ansible.builtin.package:\n", "      ansible.builtin.apt:\n        name: chrony\n      ansible.builtin.package:\n", `"Install chrony" holds more than one module: ansible.builtin.apt, ansible.builtin.package`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(src, tt.old) {
				t.Fatalf("playbook.yml holds no %q to replace", tt.old)
			}
			got := problems([]byte(strings.Replace(src, tt.old, tt.new, 1)))
			if !slices.Equal(got, []string{tt.want}) {
				t.Errorf("problems = %q, want [%q]", got, tt.want)
			}
		})
	}

	// Keywords such as with_items, async and remote_user are not modules.
	more := strings.Replace(src, "      check_mode: false\n", "      check_mode: false\n      with_items: [1]\n      async: 60\n      poll: 5\n      remote_user: admin\n", 1)
	if got := problems([]byte(more)); len(got) != 0 {
		t.Errorf("a task with more keywords gives problems %q, want none", got)
	}

	// creates, in the module arguments or under args:, makes a command honest.
	for _, args := range []string{
		"      ansible.builtin.command:\n        cmd: chronyc -n sources\n        creates: /run/chrony/chronyd.pid\n",
		"      ansible.builtin.command: chronyc -n sources\n      args:\n        creates: /run/chrony/chronyd.pid\n",
	} {
		ok := strings.Replace(src, "      ansible.builtin.command: chronyc -n sources\n      changed_when: false\n", args, 1)
		if got := problems([]byte(ok)); len(got) != 0 {
			t.Errorf("a command with creates gives problems %q, want none:\n%s", got, args)
		}
	}
}

// TestAnsibleSyntaxCheck asks Ansible itself to parse the playbook when
// ansible-playbook is installed: pipx install ansible-core
func TestAnsibleSyntaxCheck(t *testing.T) {
	bin, err := exec.LookPath("ansible-playbook")
	if err != nil {
		t.Skip("ansible-playbook is not on PATH")
	}
	cmd := exec.Command(bin, "--syntax-check", "-i", "inventory.ini", "playbook.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("ansible-playbook --syntax-check: %v\n%s", err, out)
	}
}
