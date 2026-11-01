package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"gitviewer/internal/application"
	"gitviewer/internal/askpass"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/storage"
)

func main() {
	if askpass.IsHelper(os.Args) {
		os.Exit(askpass.RunHelper(os.Args[1:]))
	}
	mygo.App.SetName("Git Viewer")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	defaultConfig, err := defaultConfigPath()
	if err != nil {
		log.Fatal("Cannot locate application configuration directory")
	}
	workingDir, _ := os.Getwd()
	options, err := parseLaunch(os.Args[1:], defaultConfig, workingDir)
	if err != nil {
		log.Fatal(err)
	}
	service := application.LocalService{
		Recent: storage.RecentRepositories{Path: options.ConfigPath},
	}
	manager := newWindowManager(service, options.AutoRefresh)
	if broker, err := askpass.Start(manager.promptPassword); err != nil {
		log.Printf("SSH password prompts unavailable: %v", err)
	} else if err := broker.InstallEnvironment(); err != nil {
		log.Printf("SSH password prompts unavailable: %v", err)
		broker.Close()
	} else {
		askpass.SetDefault(broker)
		manager.service.Remote.AllowPassword = true
		defer broker.Close()
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		next, parseErr := parseLaunch(args, defaultConfig, workingDir)
		if parseErr != nil {
			log.Print(parseErr)
			return
		}
		manager.openLaunch(context.Background(), next)
	})
	mygo.App.OnOpenURL(manager.openDeepLink)
	mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) {
		manager.closeAll()
	})
	_ = mygo.App.RegisterURLScheme("gitviewer")

	smokeFailed := false
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(manager.menu())
		if options.Smoke && len(options.Targets) == 0 {
			options.Targets = append(options.Targets, manager.makeLocalTarget(workingDir))
		}
		state := manager.openLaunch(context.Background(), options)
		if state == nil {
			return
		}
		if options.Smoke {
			go runSmoke(state, &smokeFailed)
		}
	})
	if err := mygo.App.Run(); err != nil {
		ssh.CloseConnections()
		log.Fatal(err)
	}
	manager.closeAll()
	manager.wait()
	ssh.CloseConnections()
	log.Print("background workers stopped")
	if smokeFailed {
		os.Exit(1)
	}
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "GitViewer", "recent-repositories.json"), nil
}

func runSmoke(state *windowState, failed *bool) {
	for attempt := 0; attempt < 50; attempt++ {
		time.Sleep(200 * time.Millisecond)
		done := make(chan bool, 1)
		state.window.Update(func() {
			ready := !state.app.Loading && state.app.Repository.Repository.RepositoryID != "" && !state.app.Stale
			if state.app.Workspace.FilePath != "" || len(state.app.RemoteRoots) > 0 {
				ready = !state.app.Loading && !state.app.Stale && (state.app.Workspace.FilePath != "" || len(state.app.RemoteRoots) > 0)
			}
			final := ready || attempt == 49
			if final {
				*failed = !ready
				if ready {
					log.Print("native data main-thread update passed")
				} else {
					log.Print("SMOKE FAILED: ", state.app.Message)
				}
				state.window.Close()
			}
			done <- final
		})
		if <-done {
			return
		}
	}
}
