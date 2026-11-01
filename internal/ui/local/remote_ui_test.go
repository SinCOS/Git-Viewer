package local

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func TestRemotePickerSelectionNavigationAndAlignment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entries := []domain.RemoteFileInfo{
		{Name: "a", Path: "/root/a", Dir: true},
		{Name: "long-directory-name", Path: "/root/long-directory-name", Dir: true},
		{Name: "team.code-workspace", Path: "/root/team.code-workspace"},
	}
	files := &fakeRemoteFileSystem{dirs: map[string][]domain.RemoteFileInfo{"/root/a": {}}}
	a := &App{RemoteOpen: true, RemoteHostInput: "dev", RemotePathInput: "/root", remoteBrowser: &remoteFolderBrowser{host: "dev", path: "/root", entries: entries, client: files, ctx: ctx, cancel: cancel}}
	queue := make(chan func(), 4)
	a.Dispatch = func(apply func()) { queue <- apply }
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(vscodeTheme(c.Theme())); a.remoteOpenView(c) }, 900, 720)
		tt.SetDark(dark)
		tt.Frame()
		short, ok := tt.Find("a")
		if !ok {
			t.Fatal(tt.Texts())
		}
		long, ok := tt.Find("long-directory-name")
		if !ok || short.X != long.X {
			t.Fatalf("names not aligned: %v %v", short, long)
		}
		if err := tt.Click("Select remote a"); err != nil {
			t.Fatal(err)
		}
		if a.RemotePathInput != "/root/a" || a.remoteBrowser.path != "/root" {
			t.Fatal("single click must select without entering")
		}
		if err := tt.Click("Select remote team.code-workspace"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !tt.HasText("Open workspace") {
			t.Fatal(tt.Texts())
		}
		a.remoteBrowser.filter = "no-match"
		tt.Frame()
		if !tt.HasText("No matching folders or workspaces.") {
			t.Fatal(tt.Texts())
		}
		a.remoteBrowser.filter = ""
		tt.Frame()
		if err := tt.Click("Select remote a"); err != nil {
			t.Fatal(err)
		}
		tt.Key(0, ui.KeyEnter)
		select {
		case apply := <-queue:
			apply()
		case <-time.After(time.Second):
			t.Fatal("Enter did not navigate")
		}
		if a.remoteBrowser.path != "/root/a" || a.RemotePathInput != "/root/a" {
			t.Fatal("navigation failed")
		}
		a.remoteBrowser.path, a.remoteBrowser.navigation = "/root", "/root"
		a.remoteBrowser.entries = entries
	}
	a.workers.Wait()
}

func TestRemoteStatusUsesSessionsAndBottomLeftMenu(t *testing.T) {
	a := &App{SSHHost: "saved-only"}
	if label, session := a.remoteStatus(); session || strings.Contains(label, "saved-only") {
		t.Fatal(label, session)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.remoteBrowser = &remoteFolderBrowser{host: "dev", loading: true, ctx: ctx, cancel: cancel}
	if label, _ := a.remoteStatus(); !strings.Contains(label, "Connecting") {
		t.Fatal(label)
	}
	a.remoteBrowser.client = &fakeRemoteFileSystem{}
	a.remoteBrowser.loading = false
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { ui.Box(c).Grow(1); a.remoteStatusView(c) }) }, 900, 600)
	tt.Frame()
	r, ok := tt.Find("Remote connection")
	if !ok || r.X != 0 || r.Y < 575 {
		t.Fatalf("not bottom-left: %v", r)
	}
	if err := tt.Click("Remote connection"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Disconnect Remote"); err != nil {
		t.Fatal(err)
	}
	if a.remoteBrowser != nil || a.SSHHost != "" {
		t.Fatal("disconnect retained session")
	}
}

func TestDisconnectRemotePreservesDirtyBuffer(t *testing.T) {
	const file = "/srv/project/main.go"
	files := &fakeRemoteFileSystem{info: map[string]domain.RemoteFileInfo{file: {Path: file, Size: 3, Mode: 0o600}}, files: map[string][]byte{file: []byte("old")}}
	a := newRemoteEditorApp(files)
	if err := a.openRemoteEditorFile(remoteFileRef("dev", "/srv/project", file)); err != nil {
		t.Fatal(err)
	}
	a.ActiveEditorBuffer().Set("unsaved")
	a.disconnectRemote()
	if a.ActiveEditorBuffer().String() != "unsaved" || !strings.Contains(a.RemoteOpenMessage, "unsaved") || len(a.remoteFiles) == 0 {
		t.Fatal("disconnect lost dirty session")
	}
	a.ActiveEditorBuffer().MarkSaved()
	a.disconnectRemote()
	if len(a.EditorTabs) != 0 || len(a.remoteFiles) != 0 {
		t.Fatal("disconnect retained remote resources")
	}
}

func TestRemotePanelsFitNarrowWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, picker := range []bool{false, true} {
		a := &App{RemoteOpen: true, RemoteHostInput: "dev", RemotePathInput: "/root"}
		if picker {
			a.remoteBrowser = &remoteFolderBrowser{host: "dev", path: "/root", ctx: ctx, cancel: cancel, client: &fakeRemoteFileSystem{}}
		}
		for i := 0; i < 5; i++ {
			a.Locations = append(a.Locations, domain.SavedLocation{Kind: "ssh-folder", Host: "dev", Path: strings.Repeat("/folder", i+1)})
		}
		tt := ui.NewTester(func(c *ui.Context) { a.remoteOpenView(c) }, 640, 480)
		tt.Frame()
		for _, label := range []string{"Remote panel", "Cancel"} {
			r, ok := tt.Find(label)
			if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 640 || r.Y+r.H > 480 {
				t.Fatalf("picker=%v %s outside window: %v", picker, label, r)
			}
		}
	}
}

func TestRemotePickerOffersOpenWorkspaceForAnyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &App{RemoteOpen: true, RemoteHostInput: "dev", RemotePathInput: "/root", remoteBrowser: &remoteFolderBrowser{host: "dev", path: "/root", client: &fakeRemoteFileSystem{}, ctx: ctx, cancel: cancel}}
	tt := ui.NewTester(func(c *ui.Context) { a.remoteOpenView(c) }, 900, 720)
	tt.Frame()
	if !tt.HasText("Open workspace") || !tt.HasText("Open folder") {
		t.Fatalf("folder selection must still offer a workspace: %v", tt.Texts())
	}
	a.RemotePathInput = "/root/team.code-workspace"
	tt.Frame()
	if err := tt.Click("Open workspace"); err != nil {
		t.Fatal(err)
	}
	if a.RemoteOpenMessage != "" {
		t.Fatal(a.RemoteOpenMessage)
	}
}
