//go:build darwin

// native-dialog-probe checks real AppKit sheets against the production controller.
// It cancels every panel and confirmation, so no Git or Workspace writes run.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/ui/local"
)

func main() {
	dir, err := os.MkdirTemp("", "gitviewer-dialog-probe-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(dir, "recent.json")}})
	// A confirmation is canceled before the service is invoked.
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "probe", Location: "local", Path: dir, DisplayName: "Dialog probe"}}
	var trigger func()
	returned := false
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Git Viewer Dialog Probe", Width: 1400, Height: 850, Content: ui.View(func(c *ui.Context) {
			a.View(c)
			if trigger != nil {
				action := trigger
				trigger = nil
				action()
				returned = true
			}
		})})
		a.Dispatch = win.Update
		open := func(directory bool) (string, error) {
			opts := mygo.OpenDialogOptions{Parent: win, Directory: directory, Title: "Dialog responsiveness probe", ShowHiddenFiles: directory}
			if !directory {
				opts.Filters = []mygo.FileFilter{{Name: "VS Code Workspace", Extensions: []string{"code-workspace"}}}
			}
			paths, err := mygo.Dialog.Open(opts)
			if len(paths) == 0 {
				return "", err
			}
			return paths[0], err
		}
		a.ChooseFolder = func() (string, error) { return open(true) }
		a.ChooseWorkspace = func() (string, error) { return open(false) }
		a.ChooseWorkspaceSave = func() (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: "Save responsiveness probe", DefaultPath: "probe.code-workspace", Filters: []mygo.FileFilter{{Name: "VS Code Workspace", Extensions: []string{"code-workspace"}}}})
		}
		a.Confirm = func(title, detail string) bool {
			r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: win, Title: title, Message: detail, Buttons: []string{"Cancel", "Proceed"}})
			return err == nil && r.Button == 1
		}
		win.OnClosed(func() { a.Close(); mygo.App.Quit() })
		mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) { a.Close() })
		go func() {
			scenarios := []struct {
				name     string
				action   func()
				response uintptr
			}{{"folder-open", a.OpenFolderDialog, 0}, {"workspace-open", a.ChooseWorkspaceFile, 0}, {"workspace-save", func() { a.SaveWorkspace(true) }, 0}, {"git-confirm", func() { a.RunWrite(domain.Fetch) }, 1000}}
			for _, s := range scenarios {
				win.Update(func() { returned = false; trigger = s.action })
				seen := false
				deadline := time.Now().Add(5 * time.Second)
				for {
					result := make(chan bool, 1)
					win.Update(func() {
						native := objc.ID(win.NativeHandle())
						sheet := native.Send(objc.RegisterName("attachedSheet"))
						if !seen && returned && a.DialogOpen && sheet != 0 {
							seen = true
							native.Send(objc.RegisterName("endSheet:returnCode:"), sheet, s.response)
						}
						result <- seen && !a.DialogOpen
					})
					select {
					case done := <-result:
						if done {
							fmt.Println("PASS", s.name, "native sheet, main-thread heartbeat and cancellation")
							goto next
						}
					case <-time.After(3 * time.Second):
						fmt.Fprintln(os.Stderr, "main thread blocked during", s.name)
						os.Exit(1)
					}
					if time.Now().After(deadline) {
						fmt.Fprintln(os.Stderr, "dialog did not finish:", s.name)
						os.Exit(1)
					}
					time.Sleep(30 * time.Millisecond)
				}
			next:
			}
			win.Update(func() { trigger = a.ChooseWorkspaceFile; returned = false })
			deadline := time.Now().Add(5 * time.Second)
			for {
				done := make(chan bool, 1)
				win.Update(func() {
					sheet := objc.ID(win.NativeHandle()).Send(objc.RegisterName("attachedSheet"))
					if returned && a.DialogOpen && sheet != 0 {
						win.Close()
						done <- true
					} else {
						done <- false
					}
				})
				select {
				case closed := <-done:
					if closed {
						fmt.Println("PASS close-window-with-pending-sheet")
						return
					}
				case <-time.After(3 * time.Second):
					fmt.Fprintln(os.Stderr, "close-window heartbeat blocked")
					os.Exit(1)
				}
				if time.Now().After(deadline) {
					fmt.Fprintln(os.Stderr, "closing test sheet did not open")
					os.Exit(1)
				}
				time.Sleep(30 * time.Millisecond)
			}
		}()
	})
	go func() {
		time.Sleep(25 * time.Second)
		fmt.Fprintln(os.Stderr, "native dialog probe did not shut down")
		os.Exit(1)
	}()
	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
	a.Close()
	a.Wait()
	fmt.Println("PASS controller shutdown completed without waiting for OS callback")
}
