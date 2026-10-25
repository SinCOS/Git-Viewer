package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/ui/poc"
)

func main() {
	smoke := flag.Bool("smoke", false, "verify native window, background update and clean shutdown")
	flag.Parse()
	a := poc.New()
	a.GraphMode = *smoke
	smokeFailed := false
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Git Viewer — Phase 0", Width: 1400, Height: 850, MinWidth: 900, MinHeight: 550, Content: ui.View(a.View)})
		a.Tasks.Dispatch = win.Update
		win.OnClosed(func() { a.Tasks.Close(); mygo.App.Quit() })
		mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) { a.Tasks.Close() })
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: "Prototype", Submenu: []*mygo.MenuItem{
				{Label: "Health / Version", Accelerator: "CmdOrCtrl+R", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Health(); win.Invalidate() }},
				{Label: "Graph / Diff", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.GraphMode = !a.GraphMode; win.Invalidate() }},
				{Role: mygo.RoleClose},
			}},
			{Role: mygo.RoleEditMenu},
		}))
		log.Print("native window created")
		if *smoke {
			a.Health()
			go func() {
				time.Sleep(2 * time.Second)
				win.Update(func() {
					if a.Tasks.Connection != "online" {
						smokeFailed = true
						log.Print("SMOKE FAILED: ", a.Tasks.Message)
					} else {
						log.Print("native main-thread background update passed")
					}
					a.Tasks.Close()
					mygo.App.Quit()
				})
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	a.Tasks.Close()
	a.Tasks.Wait()
	log.Print("background workers stopped")
	if smokeFailed {
		os.Exit(1)
	}
}
