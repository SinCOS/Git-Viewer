package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/ui/local"
)

func main() {
	smoke := flag.Bool("smoke", false, "verify native data loading and clean shutdown")
	repoPath := flag.String("repo", "", "open a local repository directory")
	sshHost := flag.String("ssh-host", "", "OpenSSH Host alias for remote repository")
	remotePath := flag.String("remote-repo", "", "remote absolute repository path")
	workspacePath := flag.String("workspace", "", "open a VS Code workspace")
	autoRefresh := flag.Bool("auto-refresh", true, "watch local repositories and poll remote repositories")
	configPath := flag.String("config", "", "application recent repositories JSON path")
	flag.Parse()
	if *configPath == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			log.Fatal("Cannot locate application configuration directory")
		}
		*configPath = filepath.Join(dir, "GitViewer", "recent-repositories.json")
	}
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: *configPath}})
	a.EnableAuto = *autoRefresh
	a.SSHHost = *sshHost
	a.SSHPath = *remotePath
	smokeFailed := false
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Git Viewer", Width: 1400, Height: 850, MinWidth: 1000, MinHeight: 550, TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 36, Content: ui.View(a.View)})
		a.Dispatch = win.Update
		win.OnFocus(func() { a.SetBackground(false) })
		win.OnBlur(func() { a.SetBackground(true) })
		a.ChooseWorkspace = func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Open VS Code workspace", Filters: []mygo.FileFilter{{Name: "VS Code Workspace", Extensions: []string{"code-workspace"}}}})
			if len(paths) == 0 {
				return "", err
			}
			return paths[0], err
		}
		a.ChooseWorkspaceSave = func() (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: "Create VS Code workspace", DefaultPath: "project.code-workspace", Filters: []mygo.FileFilter{{Name: "VS Code Workspace", Extensions: []string{"code-workspace"}}}})
		}
		a.Confirm = func(title, detail string) bool {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: win, Type: mygo.MessageQuestion, Title: title, Message: detail, Buttons: []string{"Cancel", "Proceed"}, DefaultButton: 0, CancelButton: 0})
			return err == nil && result.Button == 1
		}
		a.ChooseDirectory = func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Open local Git repository", Directory: true, ShowHiddenFiles: true})
			if len(paths) == 0 {
				return "", err
			}
			return paths[0], err
		}
		a.ChooseFile = func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Open file", ShowHiddenFiles: true})
			if len(paths) == 0 {
				return "", err
			}
			return paths[0], err
		}
		a.ChooseFolder = func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Open folder", Directory: true, ShowHiddenFiles: true})
			if len(paths) == 0 {
				return "", err
			}
			return paths[0], err
		}
		win.OnClosed(func() { a.Close(); mygo.App.Quit() })
		mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) { a.Close() })
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: "File", Submenu: []*mygo.MenuItem{
				{Label: "Open local repository…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Choose(); win.Invalidate() }},
				{Label: "Open folder…", Accelerator: "CmdOrCtrl+Alt+O", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					a.OpenFolderDialog()
					win.Invalidate()
				}},
				{Label: "Quick open file…", Accelerator: "CmdOrCtrl+P", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					a.OpenQuickOpen()
					win.Invalidate()
				}},
				{Label: "Refresh", Accelerator: "CmdOrCtrl+R", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Refresh(); win.Invalidate() }},
				{Label: "Cancel operation", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Cancel(); win.Invalidate() }},
				{Role: mygo.RoleClose},
			}},
			{Label: "Workspace", Submenu: []*mygo.MenuItem{
				{Label: "Open workspace…", Accelerator: "CmdOrCtrl+Shift+O", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.ChooseWorkspaceFile(); win.Invalidate() }},
				{Label: "Add current repository to workspace", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					a.AddToWorkspace(a.Repository.Repository.Path)
					win.Invalidate()
				}},
				{Label: "Add folder to workspace…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.AddFolderToWorkspace(); win.Invalidate() }},
				{Label: "New / Save workspace as…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.SaveWorkspace(true); win.Invalidate() }},
				{Label: "Save workspace", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.SaveWorkspace(false); win.Invalidate() }},
				{Label: "Close workspace", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.CloseWorkspace(); win.Invalidate() }},
			}},
			{Role: mygo.RoleEditMenu},
		}))
		log.Print("native local repository window created")
		if *smoke && *repoPath == "" {
			*repoPath, _ = os.Getwd()
		}
		if *sshHost != "" {
			a.OpenSSH()
		} else if *workspacePath != "" {
			a.OpenWorkspace(*workspacePath)
		} else if *repoPath != "" {
			a.Open(*repoPath)
		}
		if *smoke {
			go func() {
				for attempt := 0; attempt < 50; attempt++ {
					time.Sleep(200 * time.Millisecond)
					done := make(chan bool, 1)
					win.Update(func() {
						ready := !a.Loading && a.Repository.Repository.RepositoryID != "" && !a.Stale
						if *workspacePath != "" {
							ready = !a.Loading && a.Workspace.FilePath != "" && !a.Stale
						}
						final := ready || attempt == 49
						if final {
							smokeFailed = !ready
							if ready {
								log.Print("native data main-thread update passed")
							} else {
								log.Print("SMOKE FAILED: ", a.Message)
							}
							a.Close()
							mygo.App.Quit()
						}
						done <- final
					})
					if <-done {
						return
					}
				}
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	a.Close()
	a.Wait()
	log.Print("background workers stopped")
	if smokeFailed {
		os.Exit(1)
	}
}
