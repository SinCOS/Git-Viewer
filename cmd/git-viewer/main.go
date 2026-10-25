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
	smoke := flag.Bool("smoke", false, "verify native local repository status and clean shutdown")
	repoPath := flag.String("repo", "", "open a local repository directory")
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
	smokeFailed := false
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Git Viewer — Local", Width: 1400, Height: 850, MinWidth: 1000, MinHeight: 550, Content: ui.View(a.View)})
		a.Dispatch = win.Update
		a.ChooseDirectory = func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Open local Git repository", Directory: true, ShowHiddenFiles: true})
			if len(paths) == 0 {
				return "", err
			}
			return paths[0], err
		}
		win.OnClosed(func() { a.Close(); mygo.App.Quit() })
		mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) { a.Close() })
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: "Repository", Submenu: []*mygo.MenuItem{
				{Label: "Open local repository…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Choose(); win.Invalidate() }},
				{Label: "Refresh", Accelerator: "CmdOrCtrl+R", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Refresh(); win.Invalidate() }},
				{Label: "Cancel read", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Cancel(); win.Invalidate() }},
				{Role: mygo.RoleClose},
			}},
			{Role: mygo.RoleEditMenu},
		}))
		log.Print("native local repository window created")
		if *smoke && *repoPath == "" {
			*repoPath, _ = os.Getwd()
		}
		if *repoPath != "" {
			a.Open(*repoPath)
		}
		if *smoke {
			go func() {
				for attempt := 0; attempt < 50; attempt++ {
					time.Sleep(200 * time.Millisecond)
					done := make(chan bool, 1)
					win.Update(func() {
						ready := !a.Loading && a.Repository.Repository.RepositoryID != "" && !a.Stale
						final := ready || attempt == 49
						if final {
							smokeFailed = !ready
							if ready {
								log.Print("native local status main-thread update passed")
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
