package codecmd

import "testing"

func TestEditorArguments(t *testing.T) {
	result, err := Parse([]string{"--new-window", "--wait", "--goto", "/tmp/a.go:12:4"})
	if err != nil {
		t.Fatal(err)
	}
	request := result.Request
	if result.Delegate || !request.NewWindow || !request.Wait || request.Goto == nil {
		t.Fatalf("unexpected request: %+v", result)
	}
	if request.Goto.Path != "/tmp/a.go" || request.Goto.Line != 12 || request.Goto.Column != 4 {
		t.Fatalf("goto = %+v", request.Goto)
	}
	if len(request.Targets) != 1 || request.Targets[0].Path != "/tmp/a.go" {
		t.Fatalf("targets = %+v", request.Targets)
	}
}

func TestDiffAndURIs(t *testing.T) {
	result, err := Parse([]string{"--diff", "/tmp/a", "/tmp/b", "--folder-uri", "file:///tmp/中文%20dir"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Request.Diff == nil || result.Request.Diff.Left != "/tmp/a" || result.Request.Diff.Right != "/tmp/b" {
		t.Fatalf("diff = %+v", result.Request.Diff)
	}
	if len(result.Request.Targets) != 1 || result.Request.Targets[0].Path != "/tmp/中文 dir" || result.Request.Targets[0].Kind != TargetFolder {
		t.Fatalf("targets = %+v", result.Request.Targets)
	}
}

func TestDelegateAndAdd(t *testing.T) {
	result, err := Parse([]string{"--install-extension", "publisher.extension"})
	if err != nil || !result.Delegate {
		t.Fatalf("extension command not delegated: %+v %v", result, err)
	}
	result, err = Parse([]string{"--add", "src"})
	if err != nil || !result.Request.Add || len(result.Request.Targets) != 1 {
		t.Fatalf("add = %+v %v", result, err)
	}
	if result.Request.Targets[0].Kind != TargetAuto {
		t.Fatalf("kind = %s", result.Request.Targets[0].Kind)
	}
}
