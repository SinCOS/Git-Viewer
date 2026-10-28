package local

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func TestCaptureRemoteUI(t *testing.T) {
	output := os.Getenv("GITVIEWER_CAPTURE_REMOTE_UI")
	if output == "" {
		t.Skip("set GITVIEWER_CAPTURE_REMOTE_UI to export renderer evidence")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, dark := range []bool{false, true} {
		for _, mode := range []string{"login", "picker", "password"} {
			a := &App{RemoteOpen: mode != "password", remoteChooseHost: mode == "login", RemoteHostInput: "dev-server", RemotePathInput: "/home/richard/projects/git-viewer", SSHHosts: []string{"dev-server", "staging"}, AskPasswordOpen: mode == "password", AskPasswordHost: "dev-server", AskPasswordPrompt: "richard@dev-server password:", Locations: []domain.SavedLocation{{Kind: "ssh-folder", Host: "dev-server", Path: "/home/richard/projects"}, {Kind: "ssh-folder", Host: "staging", Path: "/var/www/app"}}}
			if mode == "picker" {
				a.remoteBrowser = &remoteFolderBrowser{host: "dev-server", path: "/home/richard/projects", ctx: ctx, cancel: cancel, client: &fakeRemoteFileSystem{}, entries: []domain.RemoteFileInfo{{Name: "git-viewer", Path: "/home/richard/projects/git-viewer", Dir: true}, {Name: "api-service", Path: "/home/richard/projects/api-service", Dir: true}, {Name: "design-system", Path: "/home/richard/projects/design-system", Dir: true}, {Name: "team.code-workspace", Path: "/home/richard/projects/team.code-workspace"}}}
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(vscodeTheme(c.Theme()))
				ui.Column(c).Fill().Children(func() { ui.Box(c).Grow(1); a.remoteStatusView(c) })
				a.remoteOpenView(c)
				a.passwordPromptView(c)
			}, 1000, 760)
			tt.SetDark(dark)
			tt.Frame()
			suffix := "light"
			if dark {
				suffix = "dark"
			}
			f, err := os.Create(filepath.Join(output, mode+"-"+suffix+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}
