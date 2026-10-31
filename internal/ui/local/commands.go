package local

import "github.com/egoist/mygo/ui"

// appCommand is the single source of truth for shortcuts, the command
// palette and command discovery in Settings.
type appCommand struct {
	ID     string
	Label  string
	Detail string
	Icon   *ui.SVG
	Run    func()
}

func (a *App) commandList() []appCommand {
	commands := []appCommand{
		{ID: "file.openFolder", Label: "Open Folder…", Detail: "Open a local folder or repository", Icon: folderIcon, Run: a.OpenFolderDialog},
		{ID: "file.openWorkspace", Label: "Open Workspace…", Detail: "Open a VS Code workspace", Icon: folderOpenIcon, Run: a.ChooseWorkspaceFile},
		{ID: "file.quickOpen", Label: "Quick Open", Detail: "Search commands and workspace files", Icon: searchIcon, Run: a.OpenQuickOpen},
		{ID: "file.newFile", Label: "New File", Detail: "Create a file in the active Explorer folder", Icon: plusIcon, Run: a.OpenNewFilePrompt},
		{ID: "file.newFolder", Label: "New Folder", Detail: "Create a folder in the active Explorer folder", Icon: folderIcon, Run: a.OpenNewFolderPrompt},
		{ID: "file.save", Label: "Save Active File", Detail: "Write the current editor buffer", Icon: changeFileIcon, Run: func() {
			a.Message = message(a.SaveActiveEditorFile())
		}},
		{ID: "search.workspace", Label: "Find in Files", Detail: "Search the current workspace", Icon: searchIcon, Run: a.OpenSearch},
		{ID: "terminal.toggle", Label: "Toggle Integrated Terminal", Detail: "Show or hide the local shell panel", Icon: terminalIcon, Run: a.ToggleTerminal},
		{ID: "settings.open", Label: "Settings", Detail: "Editor, search, terminal and keyboard shortcuts", Icon: settingsIcon, Run: a.OpenSettings},
		{ID: "view.editor", Label: "Switch to Editor", Detail: "Open the editor view", Icon: codeIcon, Run: func() { a.setMode("editor") }},
		{ID: "view.log", Label: "Switch to Git Log", Detail: "Browse commit history", Icon: logIcon, Run: func() { a.setMode("log") }},
		{ID: "view.commit", Label: "Switch to Commit", Detail: "Review and commit changes", Icon: commitIcon, Run: func() { a.setMode("commit") }},
		{ID: "repo.refresh", Label: "Refresh", Detail: "Reload the current repository and Explorer", Icon: refreshIcon, Run: a.Refresh},
		{ID: "remote.open", Label: "Open Remote…", Detail: "Connect to an SSH host", Icon: remoteIcon, Run: a.OpenRemoteDialog},
	}
	if _, session := a.remoteStatus(); session {
		commands = append(commands,
			appCommand{ID: "remote.switchFolder", Label: "Open Remote Folder…", Detail: "Switch the remote Explorer root", Icon: remoteIcon, Run: a.OpenRemoteFolderPicker},
			appCommand{ID: "remote.disconnect", Label: "Disconnect Remote", Detail: "Close the active SSH session", Icon: remoteIcon, Run: a.disconnectRemote},
		)
	}
	if a.Workspace.FilePath != "" || len(a.WorkspaceFolders) > 0 {
		commands = append(commands, appCommand{
			ID: "workspace.close", Label: "Close Workspace", Detail: "Leave the current multi-root workspace",
			Icon: folderOpenIcon, Run: a.CloseWorkspace,
		})
	}
	if a.ThemeStyle == ThemeStyleClaude {
		commands = append(commands, appCommand{
			ID: "theme.vscode", Label: "Use VS Code Theme", Detail: "Switch to the blue VS Code palette",
			Icon: settingsIcon, Run: func() { a.SetThemeStyle(ThemeStyleVSCode) },
		})
	} else {
		commands = append(commands, appCommand{
			ID: "theme.claude", Label: "Use Claude Theme", Detail: "Switch to the warm terracotta palette",
			Icon: settingsIcon, Run: func() { a.SetThemeStyle(ThemeStyleClaude) },
		})
	}
	return commands
}

func (a *App) runCommand(id string) bool {
	for _, command := range a.commandList() {
		if command.ID == id {
			command.Run()
			return true
		}
	}
	return false
}
