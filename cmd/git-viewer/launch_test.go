package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"gitviewer/internal/application"
	"gitviewer/internal/infrastructure/storage"
)

func TestParseLaunchRemoteHelper(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"default", nil, "auto"},
		{"auto", []string{"-remote-helper=auto"}, "auto"},
		{"off", []string{"-remote-helper", "off"}, "off"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, err := parseLaunch(test.args, "/tmp/config.json", "/tmp")
			if err != nil || options.RemoteHelper != test.want {
				t.Fatalf("mode = %q, error = %v, want %q", options.RemoteHelper, err, test.want)
			}
		})
	}
	for _, mode := range []string{"", "on", "AUTO", " auto", "off "} {
		if _, err := parseLaunch([]string{"-remote-helper=" + mode}, "/tmp/config.json", "/tmp"); err == nil {
			t.Fatalf("accepted invalid remote helper mode %q", mode)
		}
	}
}

func TestParseLaunchRemoteFolderAndWorkspace(t *testing.T) {
	options, err := parseLaunch(
		[]string{"-ssh-host", "dev-box", "-remote-repo", "/srv/中文 project", "-new-window", "-wait", "-goto", "/srv/中文 project/main.go:12:4"},
		"/tmp/config.json",
		"/tmp",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Targets) != 1 {
		t.Fatalf("targets = %+v", options.Targets)
	}
	target := options.Targets[0]
	if target.Host != "dev-box" || target.Path != "/srv/中文 project" || target.Kind != "remote-folder" || !target.NewWindow || !target.Wait || target.Line != 12 || target.Column != 4 {
		t.Fatalf("target = %+v", target)
	}

	options, err = parseLaunch([]string{"-ssh-host", "dev-box", "-remote-repo", "/srv/project.code-workspace"}, "/tmp/config.json", "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Targets) != 1 || options.Targets[0].Kind != "remote-workspace" {
		t.Fatalf("workspace target = %+v", options.Targets)
	}
}

func TestParseLaunchResolvesLocalTargets(t *testing.T) {
	options, err := parseLaunch([]string{"one", "two.code-workspace"}, "/tmp/config.json", "/work")
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Targets) != 2 {
		t.Fatalf("targets = %+v", options.Targets)
	}
	if options.Targets[0].Path != filepath.Join("/work", "one") || options.Targets[0].Kind != "local-folder" {
		t.Fatalf("first target = %+v", options.Targets[0])
	}
	if options.Targets[1].Path != filepath.Join("/work", "two.code-workspace") || options.Targets[1].Kind != "local-workspace" {
		t.Fatalf("second target = %+v", options.Targets[1])
	}
}

func TestMatchingRemoteUsesPathBoundaryAndRecentOrder(t *testing.T) {
	service := application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}}
	manager := newWindowManager(service, false)
	first, second := &mygo.Window{}, &mygo.Window{}
	manager.windows[first] = &windowState{window: first, host: "dev", path: "/srv/project"}
	manager.windows[second] = &windowState{window: second, host: "dev", path: "/srv/other"}
	manager.order = []*mygo.Window{first, second}
	if got := manager.matchingRemote("dev", "/srv/project/src/main.go"); got == nil || got.window != first {
		t.Fatalf("matching window = %+v", got)
	}
	if got := manager.matchingRemote("dev", "/srv/project-other/file"); got != nil {
		t.Fatalf("path prefix escaped root: %+v", got)
	}
	if got := manager.mostRecent("dev"); got == nil || got.window != second {
		t.Fatalf("most recent = %+v", got)
	}
}

func TestParseLineColumn(t *testing.T) {
	for _, test := range []struct {
		value        string
		line, column int
	}{
		{"", 0, 0},
		{"12", 0, 0},
		{"12:4", 12, 4},
		{"dir:name/file.go:12:4", 12, 4},
		{"file.go:0:4", 0, 4},
	} {
		line, column := parseLineColumn(test.value)
		if line != test.line || column != test.column {
			t.Fatalf("%q = %d:%d, want %d:%d", test.value, line, column, test.line, test.column)
		}
	}
}
