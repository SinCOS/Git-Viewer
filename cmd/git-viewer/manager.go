package main

import (
	"context"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/bridge"
	"gitviewer/internal/codecmd"
	"gitviewer/internal/deeplink"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/remotecli"
	"gitviewer/internal/ui/local"
)

type windowState struct {
	window   *mygo.Window
	app      *local.App
	host     string
	path     string
	wait     chan struct{}
	waitOnce sync.Once
	lastUsed int64
}

func (s *windowState) finishWait() {
	s.waitOnce.Do(func() {
		if s.wait != nil {
			close(s.wait)
		}
	})
}

type windowManager struct {
	mu         sync.Mutex
	service    application.LocalService
	auto       bool
	windows    map[*mygo.Window]*windowState
	order      []*mygo.Window
	bridges    map[string]*bridge.Session
	installing map[string]bool
	prompted   map[string]bool
	approved   map[string]bool
	installed  map[string]bool
	quitting   bool
}

func newWindowManager(service application.LocalService, auto bool) *windowManager {
	return &windowManager{
		service: service, auto: auto,
		windows:    map[*mygo.Window]*windowState{},
		bridges:    map[string]*bridge.Session{},
		installing: map[string]bool{},
		prompted:   map[string]bool{},
		approved:   map[string]bool{},
		installed:  map[string]bool{},
	}
}

func (m *windowManager) openWindow(target *launchTarget) *windowState {
	app := local.New(m.service)
	app.EnableAuto = m.auto
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "Git Viewer", Width: 1400, Height: 850, MinWidth: 1000, MinHeight: 550,
		TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 36, Content: ui.View(app.View),
	})
	state := &windowState{window: win, app: app, wait: make(chan struct{})}
	app.Dispatch = win.Update
	m.mu.Lock()
	m.windows[win] = state
	m.order = append(m.order, win)
	m.mu.Unlock()
	m.wireWindow(state)
	if target != nil {
		state.host, state.path = target.Host, target.Path
		m.openTargetOnState(state, *target)
	}
	return state
}

func (m *windowManager) wireWindow(state *windowState) {
	win, app := state.window, state.app
	win.OnFocus(func() {
		app.SetBackground(false)
		m.touchWindow(state)
		if state.host != "" {
			go m.ensureHost(state.host)
		}
	})
	win.OnBlur(func() { app.SetBackground(true) })
	app.ChooseWorkspace = func() (string, error) {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: win, Title: "Open VS Code workspace",
			Filters: []mygo.FileFilter{{Name: "VS Code Workspace", Extensions: []string{"code-workspace"}}},
		})
		if len(paths) == 0 {
			return "", err
		}
		return paths[0], err
	}
	app.ChooseWorkspaceSave = func() (string, error) {
		return mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "Create VS Code workspace", DefaultPath: "project.code-workspace",
			Filters: []mygo.FileFilter{{Name: "VS Code Workspace", Extensions: []string{"code-workspace"}}},
		})
	}
	app.Confirm = func(title, detail string) bool {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: win, Type: mygo.MessageQuestion, Title: title, Message: detail,
			Buttons: []string{"Cancel", "Proceed"}, DefaultButton: 0, CancelButton: 0,
		})
		return err == nil && result.Button == 1
	}
	app.ConfirmRemoteConflict = func(file string) int {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: win, Type: mygo.MessageQuestion, Title: "Remote file changed",
			Message: "The file changed on the remote host since it was opened.\n" + file,
			Buttons: []string{"Cancel", "Overwrite", "Reload"}, DefaultButton: 2, CancelButton: 0,
		})
		if err != nil {
			return 0
		}
		return result.Button
	}
	app.ChooseFile = func() (string, error) {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Open file", ShowHiddenFiles: true})
		if len(paths) == 0 {
			return "", err
		}
		return paths[0], err
	}
	app.ChooseFolder = func() (string, error) {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Open folder", Directory: true, ShowHiddenFiles: true})
		if len(paths) == 0 {
			return "", err
		}
		return paths[0], err
	}
	app.OnRemoteOpened = func(host string) {
		m.recordRemoteOpened(state, host)
		go m.ensureHost(host)
	}
	win.OnClosed(func() {
		m.windowClosed(state)
	})
}

func (m *windowManager) recordRemoteOpened(state *windowState, host string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state.host, state.path = host, state.app.SSHPath
}

func (m *windowManager) touchWindow(state *windowState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, win := range m.order {
		if win == state.window {
			m.order = append(m.order[:i], m.order[i+1:]...)
			m.order = append(m.order, state.window)
			return
		}
	}
}

func (m *windowManager) windowClosed(state *windowState) {
	state.app.Close()
	go state.app.Wait()
	state.finishWait()
	m.mu.Lock()
	delete(m.windows, state.window)
	for i, win := range m.order {
		if win == state.window {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	last := len(m.windows) == 0
	quitting := m.quitting
	m.mu.Unlock()
	if last && !quitting {
		mygo.App.Quit()
	}
}

func (m *windowManager) closeAll() {
	m.mu.Lock()
	m.quitting = true
	states := make([]*windowState, 0, len(m.windows))
	for _, state := range m.windows {
		states = append(states, state)
	}
	bridges := make([]*bridge.Session, 0, len(m.bridges))
	for _, session := range m.bridges {
		bridges = append(bridges, session)
	}
	m.bridges = map[string]*bridge.Session{}
	m.mu.Unlock()
	for _, state := range states {
		state.app.Close()
		state.finishWait()
	}
	for _, session := range bridges {
		_ = session.Close()
	}
}

func (m *windowManager) wait() {
	m.mu.Lock()
	states := make([]*windowState, 0, len(m.windows))
	for _, state := range m.windows {
		states = append(states, state)
	}
	m.mu.Unlock()
	for _, state := range states {
		state.app.Wait()
	}
}

func (m *windowManager) stateForWindow(win *mygo.Window) *windowState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.windows[win]; state != nil {
		return state
	}
	for i := len(m.order) - 1; i >= 0; i-- {
		if state := m.windows[m.order[i]]; state != nil {
			return state
		}
	}
	return nil
}

func (m *windowManager) mostRecent(host string) *windowState {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.order) - 1; i >= 0; i-- {
		state := m.windows[m.order[i]]
		if state != nil && (host == "" || state.host == host) {
			return state
		}
	}
	return nil
}

func (m *windowManager) openTargetOnState(state *windowState, target launchTarget) {
	switch target.Kind {
	case "local-workspace":
		state.app.OpenWorkspace(target.Path)
	case "local-folder":
		state.app.Open(target.Path)
		if target.Line > 0 {
			state.app.OpenEditorFile(target.Path)
			state.app.GoToActiveEditor(target.Line, target.Column)
		}
	case "remote-workspace":
		state.app.OpenRemoteWorkspace(target.Host, target.Path)
	case "remote-file":
		state.app.OpenRemoteFile(target.Host, target.Path, target.Line, target.Column)
	case "remote-diff":
		state.app.OpenRemoteDiff(target.Host, target.Path, target.Right)
	case "remote-folder":
		state.app.OpenRemoteFolder(target.Host, target.Path)
	}
	state.window.Focus()
}

func (m *windowManager) OpenTarget(ctx context.Context, target launchTarget) *windowState {
	if target.Host != "" && target.Kind != "remote-workspace" && target.Kind != "remote-file" && target.Kind != "remote-diff" {
		if info, err := m.service.RemoteInfo(ctx, target.Host, target.Path); err == nil {
			if strings.HasSuffix(strings.ToLower(target.Path), ".code-workspace") {
				target.Kind = "remote-workspace"
			} else if info.Dir {
				target.Kind = "remote-folder"
			} else {
				target.Kind = "remote-file"
			}
		}
	}
	if target.Host != "" {
		if canonical, err := m.service.CanonicalRemotePath(ctx, target.Host, target.Path); err == nil {
			target.Path = canonical
		}
	}
	if target.Add {
		if state := m.mostRecent(target.Host); state != nil {
			folder := target.Path
			if target.Kind == "remote-file" {
				folder = path.Dir(folder)
			}
			state.app.AddRemoteFolders(target.Host, []string{folder})
			state.window.Focus()
			return state
		}
		target.NewWindow = true
	}
	if !target.NewWindow && target.Host != "" {
		if state := m.matchingRemote(target.Host, target.Path); state != nil {
			m.openTargetOnState(state, target)
			return state
		}
	}
	state := m.openWindow(&target)
	return state
}

func (m *windowManager) matchingRemote(host, file string) *windowState {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.order) - 1; i >= 0; i-- {
		state := m.windows[m.order[i]]
		if state == nil || state.host != host || state.path == "" {
			continue
		}
		root := strings.TrimSuffix(state.path, "/")
		if file == root || strings.HasPrefix(file, root+"/") {
			return state
		}
	}
	return nil
}

func (m *windowManager) openLaunch(ctx context.Context, options launchOptions) *windowState {
	var state *windowState
	for _, target := range options.Targets {
		if state == nil || target.NewWindow || target.Add {
			state = m.OpenTarget(ctx, target)
			continue
		}
		m.openTargetOnState(state, target)
	}
	if state == nil {
		state = m.openWindow(nil)
	}
	return state
}

func (m *windowManager) openDeepLink(raw string) {
	request, err := deeplink.Parse(raw)
	if err != nil {
		log.Printf("ignored invalid deep link: %v", err)
		return
	}
	kind := "remote-folder"
	if request.Kind == deeplink.File {
		kind = "remote-file"
	} else if request.Kind == deeplink.Workspace {
		kind = "remote-workspace"
	}
	if request.DiffPath != "" {
		kind = "remote-diff"
	}
	m.OpenTarget(context.Background(), launchTarget{
		Kind: kind, Host: request.Host, Path: request.Path, NewWindow: request.NewWindow,
		Right: request.DiffPath, Line: request.Line, Column: request.Column,
	})
}

func (m *windowManager) ensureHost(host string) {
	if !ssh.ValidHost(host) {
		return
	}
	m.mu.Lock()
	if current := m.bridges[host]; current != nil {
		select {
		case <-current.Done():
			delete(m.bridges, host)
		default:
			m.mu.Unlock()
			m.ensureRemoteCLI(host)
			return
		}
	}
	m.mu.Unlock()
	cfg := bridge.Config{
		SSHPath: m.service.Remote.SSHPath, ConfigPath: m.service.Remote.ConfigPath,
		KnownHostsFile: m.service.Remote.KnownHostsFile,
	}
	session, err := bridge.Start(context.Background(), cfg, host, m.handleCode)
	if err != nil {
		log.Printf("remote bridge unavailable for %s: %v", host, err)
	} else {
		m.mu.Lock()
		if old := m.bridges[host]; old != nil {
			select {
			case <-old.Done():
			default:
				m.mu.Unlock()
				_ = session.Close()
				m.ensureRemoteCLI(host)
				return
			}
		}
		m.bridges[host] = session
		m.mu.Unlock()
		go m.watchBridge(host, session)
	}
	m.ensureRemoteCLI(host)
}

func (m *windowManager) watchBridge(host string, session *bridge.Session) {
	<-session.Done()
	m.mu.Lock()
	if m.bridges[host] == session {
		delete(m.bridges, host)
	}
	m.mu.Unlock()
}

func (m *windowManager) ensureRemoteCLI(host string) {
	m.mu.Lock()
	if m.installing[host] {
		m.mu.Unlock()
		return
	}
	m.installing[host] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.installing, host)
		m.mu.Unlock()
	}()
	state := m.mostRecent(host)
	if state == nil {
		return
	}
	executor := ssh.Executor{
		SSHPath: m.service.Remote.SSHPath, ConfigPath: m.service.Remote.ConfigPath,
		KnownHostsFile: m.service.Remote.KnownHostsFile,
	}
	m.mu.Lock()
	installed := m.installed[host]
	approved := m.approved[host]
	prompted := m.prompted[host]
	m.mu.Unlock()
	if installed {
		return
	}
	if !approved {
		if prompted {
			return
		}
		status, err := remotecli.Inspect(context.Background(), executor, host)
		if err != nil {
			log.Printf("cannot inspect remote code command on %s: %v", host, err)
		} else if status.Installed && status.Host == host {
			approved = true
			m.mu.Lock()
			m.approved[host] = true
			m.prompted[host] = true
			m.mu.Unlock()
		}
	}
	if !approved {
		if state.app.Confirm == nil {
			return
		}
		m.mu.Lock()
		m.prompted[host] = true
		m.mu.Unlock()
		if !state.app.Confirm(
			"Install remote code command",
			"Install or update the Git Viewer code command on "+host+"? Existing code is backed up when writable.",
		) {
			return
		}
		m.mu.Lock()
		m.approved[host] = true
		m.mu.Unlock()
	}
	if err := remotecli.Install(context.Background(), executor, host); err != nil {
		log.Printf("cannot install remote code command on %s: %v", host, err)
		return
	}
	m.mu.Lock()
	m.installed[host] = true
	m.prompted[host] = true
	m.mu.Unlock()
	state.window.Update(func() { state.app.Message = "Remote code command installed" })
	state.window.Invalidate()
}

func (m *windowManager) handleCode(ctx context.Context, request bridge.OpenRequest) (<-chan struct{}, error) {
	parsed, err := codecmd.Parse(request.Argv)
	if err != nil {
		return nil, err
	}
	if parsed.Delegate {
		return nil, fmt.Errorf("unsupported command was not delegated by the remote wrapper")
	}
	code := parsed.Request
	absolute := func(value string) string {
		if value == "" {
			return value
		}
		if path.IsAbs(value) {
			return path.Clean(value)
		}
		return path.Join(request.WorkingDir, value)
	}
	if code.Diff != nil {
		if code.ReuseWindow && !code.NewWindow {
			if state := m.mostRecent(request.Host); state != nil {
				state.host, state.path = request.Host, absolute(code.Diff.Left)
				state.app.OpenRemoteDiff(request.Host, absolute(code.Diff.Left), absolute(code.Diff.Right))
				state.window.Focus()
				if code.Wait {
					return state.wait, nil
				}
				return nil, nil
			}
		}
		target := launchTarget{
			Kind: "remote-diff", Host: request.Host, Path: absolute(code.Diff.Left),
			Right:     absolute(code.Diff.Right),
			NewWindow: code.NewWindow, Wait: code.Wait,
		}
		state := m.OpenTarget(ctx, target)
		if code.Wait {
			return state.wait, nil
		}
		return nil, nil
	}
	var targets []launchTarget
	for _, target := range code.Targets {
		file := absolute(target.Path)
		kind := m.remoteKind(ctx, request.Host, file)
		if target.Kind == codecmd.TargetWorkspace {
			kind = "remote-workspace"
		} else if target.Kind == codecmd.TargetFile {
			kind = "remote-file"
		} else if target.Kind == codecmd.TargetFolder {
			kind = "remote-folder"
		}
		next := launchTarget{
			Kind: kind, Host: request.Host, Path: file,
			NewWindow: code.NewWindow, Wait: code.Wait, Add: code.Add,
		}
		if code.Goto != nil && path.Clean(file) == path.Clean(absolute(code.Goto.Path)) {
			next.Kind = "remote-file"
			next.Line = code.Goto.Line
			next.Column = code.Goto.Column
		}
		targets = append(targets, next)
	}
	if len(targets) == 0 {
		targets = append(targets, launchTarget{Kind: "remote-folder", Host: request.Host, Path: request.WorkingDir, NewWindow: code.NewWindow, Wait: code.Wait})
	}
	if code.Add {
		if state := m.mostRecent(request.Host); state != nil {
			var folders []string
			for _, target := range targets {
				folder := target.Path
				if target.Kind == "remote-file" {
					folder = path.Dir(folder)
				}
				folders = append(folders, folder)
			}
			state.app.AddRemoteFolders(request.Host, folders)
			state.window.Focus()
			if code.Wait {
				return state.wait, nil
			}
			return nil, nil
		}
	}
	if code.ReuseWindow && !code.NewWindow && len(targets) > 0 {
		if state := m.mostRecent(request.Host); state != nil {
			state.host, state.path = request.Host, targets[0].Path
			allFolders := true
			folders := make([]string, 0, len(targets))
			for _, target := range targets {
				if target.Kind != "remote-folder" {
					allFolders = false
					break
				}
				folders = append(folders, target.Path)
			}
			if allFolders {
				state.app.OpenRemoteFolders(request.Host, folders)
			} else {
				for _, target := range targets {
					m.openTargetOnState(state, target)
				}
			}
			if code.Wait {
				return state.wait, nil
			}
			return nil, nil
		}
	}
	if len(targets) > 1 {
		allFolders := true
		var folders []string
		for _, target := range targets {
			if target.Kind != "remote-folder" {
				allFolders = false
				break
			}
			folders = append(folders, target.Path)
		}
		if allFolders {
			state := m.openWindow(nil)
			state.host, state.path = request.Host, folders[0]
			state.app.OpenRemoteFolders(request.Host, folders)
			if code.Wait {
				return state.wait, nil
			}
			return nil, nil
		}
	}
	state := m.OpenTarget(ctx, targets[0])
	for _, target := range targets[1:] {
		m.openTargetOnState(state, target)
	}
	if code.Wait {
		return state.wait, nil
	}
	return nil, nil
}

func (m *windowManager) menu() *mygo.Menu {
	action := func(fn func(*windowState, *mygo.Window)) func(*mygo.MenuItem, *mygo.Window) {
		return func(_ *mygo.MenuItem, win *mygo.Window) {
			state := m.stateForWindow(win)
			if state == nil {
				return
			}
			fn(state, state.window)
		}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Open folder…", Accelerator: "CmdOrCtrl+O", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.OpenFolderDialog()
				win.Invalidate()
			})},
			{Label: "Open remote…", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.OpenRemoteDialog()
				win.Invalidate()
			})},
			{Label: "Quick open file…", Accelerator: "CmdOrCtrl+P", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.OpenQuickOpen()
				win.Invalidate()
			})},
			{Label: "Refresh", Accelerator: "CmdOrCtrl+R", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.Refresh()
				win.Invalidate()
			})},
			{Label: "Cancel operation", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.Cancel()
				win.Invalidate()
			})},
			{Role: mygo.RoleClose},
		}},
		{Label: "Workspace", Submenu: []*mygo.MenuItem{
			{Label: "Open workspace…", Accelerator: "CmdOrCtrl+Shift+O", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.ChooseWorkspaceFile()
				win.Invalidate()
			})},
			{Label: "Add current repository to workspace", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.AddToWorkspace(state.app.Repository.Repository.Path)
				win.Invalidate()
			})},
			{Label: "Add folder to workspace…", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.AddFolderToWorkspace()
				win.Invalidate()
			})},
			{Label: "New / Save workspace as…", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.SaveWorkspace(true)
				win.Invalidate()
			})},
			{Label: "Save workspace", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.SaveWorkspace(false)
				win.Invalidate()
			})},
			{Label: "Close workspace", Click: action(func(state *windowState, win *mygo.Window) {
				state.app.CloseWorkspace()
				win.Invalidate()
			})},
		}},
		{Role: mygo.RoleEditMenu},
	})
}

func (m *windowManager) primary() *windowState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.order) == 0 {
		return nil
	}
	return m.windows[m.order[0]]
}

func (m *windowManager) windowCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.windows)
}

func (m *windowManager) makeLocalTarget(value string) launchTarget {
	if strings.HasSuffix(strings.ToLower(value), ".code-workspace") {
		return launchTarget{Kind: "local-workspace", Path: filepath.Clean(value)}
	}
	return launchTarget{Kind: "local-folder", Path: filepath.Clean(value)}
}

func (m *windowManager) remoteKind(ctx context.Context, host, file string) string {
	info, err := m.service.RemoteInfo(ctx, host, file)
	if err != nil {
		return "remote-folder"
	}
	if strings.HasSuffix(strings.ToLower(file), ".code-workspace") {
		return "remote-workspace"
	}
	if info.Dir {
		return "remote-folder"
	}
	return "remote-file"
}

func (m *windowManager) openDomainRoot(root domain.EditorRoot) {
	if root.Location == domain.LocationSSH {
		m.OpenTarget(context.Background(), launchTarget{Kind: "remote-folder", Host: root.SSHHost, Path: root.Path})
		return
	}
	m.OpenTarget(context.Background(), launchTarget{Kind: "local-folder", Path: root.Path})
}
