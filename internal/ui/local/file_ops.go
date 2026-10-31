package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

type fileTarget struct {
	ref    fileRef
	root   string
	remote bool
}

func (a *App) targetForPath(value string) (fileTarget, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, 0) {
		return fileTarget{}, fmt.Errorf("a path is required")
	}
	for _, root := range a.explorerRoots() {
		if root.tree == nil {
			continue
		}
		if root.tree.remote {
			base := path.Clean(root.tree.path)
			clean := path.Clean(value)
			if clean != base && !strings.HasPrefix(clean, strings.TrimSuffix(base, "/")+"/") {
				continue
			}
			return fileTarget{
				ref: remoteFileRef(root.tree.host, root.tree.path, clean), root: base, remote: true,
			}, nil
		}
		base, err := filepath.Abs(root.tree.path)
		if err != nil {
			continue
		}
		clean, err := filepath.Abs(value)
		if err != nil || !pathWithin(base, clean) {
			continue
		}
		return fileTarget{ref: localFileRef(clean, base), root: base}, nil
	}
	return fileTarget{}, fmt.Errorf("the path is outside the current Explorer roots")
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (a *App) explorerTargetDir() string {
	if tab := a.activeEditorTab(); tab != nil && tab.ref.Location == domain.LocationLocal {
		return filepath.Dir(tab.ref.Path)
	}
	if tab := a.activeEditorTab(); tab != nil {
		return path.Dir(tab.ref.Path)
	}
	return a.folderRoot()
}

func (a *App) OpenNewFilePrompt()   { a.NewFileAt(a.explorerTargetDir()) }
func (a *App) OpenNewFolderPrompt() { a.NewFolderAt(a.explorerTargetDir()) }

func (a *App) NewFileAt(dir string) {
	if strings.TrimSpace(dir) == "" {
		a.Message = "Open a folder before creating a file"
		return
	}
	a.openFilePrompt("new-file", dir, "untitled.txt", "New File", "Enter a file name or a relative path.")
}

func (a *App) NewFolderAt(dir string) {
	if strings.TrimSpace(dir) == "" {
		a.Message = "Open a folder before creating a folder"
		return
	}
	a.openFilePrompt("new-folder", dir, "new-folder", "New Folder", "Enter a folder name or a relative path.")
}

func (a *App) RenamePath(value string) {
	name := path.Base(filepath.ToSlash(value))
	a.openFilePrompt("rename", value, name, "Rename", "Enter a new name without a directory component.")
}

func (a *App) DeletePath(value string) {
	a.openFilePrompt("delete", value, "", "Delete", "This removes the path from disk. This action cannot be undone.")
}

func (a *App) openFilePrompt(kind, target, value, title, detail string) {
	a.FilePromptOpen = true
	a.FilePromptKind = kind
	a.FilePromptPath = target
	a.FilePromptValue = value
	a.FilePromptTitle = title
	a.FilePromptDetail = detail
	a.FilePromptError = ""
}

func (a *App) submitFilePrompt() {
	kind := a.FilePromptKind
	targetPath := a.FilePromptPath
	value := strings.TrimSpace(a.FilePromptValue)
	switch kind {
	case "replace-all":
		a.FilePromptOpen = false
		a.replaceSearchMatches(true)
		return
	case "delete":
		if a.dirtyOpenPath(targetPath, true) {
			a.FilePromptError = "Save or close files under this path before deleting it."
			return
		}
		a.FilePromptOpen = false
		a.runFileOperation(func(ctx context.Context) (func(), error) {
			target, err := a.targetForPath(targetPath)
			if err != nil {
				return nil, err
			}
			if err := a.removeTarget(ctx, target, true); err != nil {
				return nil, err
			}
			return func() {
				a.closeTabsUnder(targetPath, true)
				a.afterFileMutation(target)
				a.Message = "Deleted " + path.Base(targetPath)
			}, nil
		})
		return
	case "rename":
		if value == "" || strings.ContainsAny(value, `/\`) || value == "." || value == ".." {
			a.FilePromptError = "Enter a file or folder name without a directory component."
			return
		}
		if a.dirtyOpenPath(targetPath, true) {
			a.FilePromptError = "Save or close files under this path before renaming it."
			return
		}
		a.FilePromptOpen = false
		a.runFileOperation(func(ctx context.Context) (func(), error) {
			source, err := a.targetForPath(targetPath)
			if err != nil {
				return nil, err
			}
			destination := source
			if source.remote {
				destination.ref.Path = path.Join(path.Dir(source.ref.Path), value)
			} else {
				destination.ref.Path = filepath.Join(filepath.Dir(source.ref.Path), filepath.FromSlash(value))
			}
			if err := a.renameTarget(ctx, source, destination); err != nil {
				return nil, err
			}
			return func() {
				a.renameOpenTabs(targetPath, destination.ref.Path)
				a.afterFileMutation(source)
				a.Message = "Renamed to " + value
			}, nil
		})
		return
	case "new-file", "new-folder":
		if value == "" || filepath.IsAbs(value) || path.IsAbs(filepath.ToSlash(value)) || hasParentComponent(filepath.ToSlash(value)) {
			a.FilePromptError = "Enter a relative path inside the Explorer root."
			return
		}
		a.FilePromptOpen = false
		a.runFileOperation(func(ctx context.Context) (func(), error) {
			base, err := a.targetForPath(targetPath)
			if err != nil {
				return nil, err
			}
			target := base
			if base.remote {
				target.ref.Path = path.Join(base.ref.Path, filepath.ToSlash(value))
			} else {
				target.ref.Path = filepath.Join(base.ref.Path, filepath.FromSlash(value))
			}
			if kind == "new-folder" {
				err = a.makeDirectory(ctx, target)
			} else {
				err = a.createFile(ctx, target)
			}
			if err != nil {
				return nil, err
			}
			return func() {
				a.afterFileMutation(target)
				if kind == "new-file" {
					a.Message = message(a.openEditorRef(target.ref))
				} else {
					a.Message = "Created " + value
				}
			}, nil
		})
	}
}

func (a *App) dirtyOpenPath(value string, recursive bool) bool {
	for _, tab := range a.EditorTabs {
		if !tab.state.Buffer.Modified() {
			continue
		}
		if pathMatches(tab.ref, value, recursive) {
			return true
		}
	}
	return false
}

func (a *App) closeTabsUnder(value string, recursive bool) {
	for i := len(a.EditorTabs) - 1; i >= 0; i-- {
		if pathMatches(a.EditorTabs[i].ref, value, recursive) {
			a.closeEditorTab(i)
		}
	}
}

func (a *App) renameOpenTabs(from, to string) {
	source, err := a.targetForPath(from)
	if err != nil {
		return
	}
	for _, tab := range a.EditorTabs {
		if !pathMatches(tab.ref, from, true) {
			continue
		}
		if source.remote {
			rel, err := filepath.Rel(filepath.FromSlash(from), filepath.FromSlash(tab.ref.Path))
			if err != nil {
				continue
			}
			tab.ref.Path = path.Join(to, filepath.ToSlash(rel))
		} else {
			rel, err := filepath.Rel(from, tab.ref.Path)
			if err != nil {
				continue
			}
			tab.ref.Path = filepath.Join(to, rel)
		}
	}
}

func pathMatches(ref fileRef, value string, recursive bool) bool {
	if ref.Location == domain.LocationSSH {
		clean, target := path.Clean(ref.Path), path.Clean(value)
		return clean == target || recursive && strings.HasPrefix(clean, strings.TrimSuffix(target, "/")+"/")
	}
	clean, err := filepath.Abs(ref.Path)
	if err != nil {
		return false
	}
	target, err := filepath.Abs(value)
	if err != nil {
		return false
	}
	return clean == target || recursive && pathWithin(target, clean)
}

func (a *App) runFileOperation(work func(context.Context) (func(), error)) {
	if a.closed {
		return
	}
	dispatch := a.Dispatch
	if dispatch == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		apply, err := work(ctx)
		if err != nil {
			a.Message = fileOperationMessage(err)
			return
		}
		apply()
		return
	}
	a.Message = "Updating files…"
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		apply, err := work(ctx)
		dispatch(func() {
			if a.closed {
				return
			}
			if err != nil {
				a.Message = fileOperationMessage(err)
				return
			}
			apply()
		})
	}()
}

func fileOperationMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, os.ErrExist) {
		return "A file or folder already exists at that path"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "The file or folder no longer exists"
	}
	return err.Error()
}

func (a *App) afterFileMutation(target fileTarget) {
	a.resetExplorerCache()
	a.Refresh()
}

func (a *App) createFile(ctx context.Context, target fileTarget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if target.remote {
		client := a.remoteClient(target.ref.Host, target.ref.Root)
		if client == nil {
			return fmt.Errorf("remote connection is unavailable")
		}
		if _, err := client.Stat(ctx, target.ref.Path); err == nil {
			return os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return client.WriteFileAtomic(ctx, target.ref.Path, nil, 0o644)
	}
	clean, err := validateLocalTarget(target.root, target.ref.Path, true)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func (a *App) makeDirectory(ctx context.Context, target fileTarget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if target.remote {
		client := a.remoteClient(target.ref.Host, target.ref.Root)
		if client == nil {
			return fmt.Errorf("remote connection is unavailable")
		}
		return client.Mkdir(ctx, target.ref.Path, 0o755)
	}
	clean, err := validateLocalTarget(target.root, target.ref.Path, true)
	if err != nil {
		return err
	}
	return os.Mkdir(clean, 0o755)
}

func (a *App) renameTarget(ctx context.Context, source, destination fileTarget) error {
	if source.ref.Location != destination.ref.Location || source.ref.Host != destination.ref.Host || source.ref.Root != destination.ref.Root {
		return fmt.Errorf("rename must stay in the same Explorer root")
	}
	if source.remote {
		client := a.remoteClient(source.ref.Host, source.ref.Root)
		if client == nil {
			return fmt.Errorf("remote connection is unavailable")
		}
		return client.Rename(ctx, source.ref.Path, destination.ref.Path)
	}
	if _, err := validateLocalTarget(source.root, source.ref.Path, false); err != nil {
		return err
	}
	cleanDestination, err := validateLocalTarget(destination.root, destination.ref.Path, true)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(cleanDestination); err == nil {
		return os.ErrExist
	}
	return os.Rename(source.ref.Path, cleanDestination)
}

func (a *App) removeTarget(ctx context.Context, target fileTarget, recursive bool) error {
	if target.remote {
		client := a.remoteClient(target.ref.Host, target.ref.Root)
		if client == nil {
			return fmt.Errorf("remote connection is unavailable")
		}
		return client.Remove(ctx, target.ref.Path, recursive)
	}
	clean, err := validateLocalTarget(target.root, target.ref.Path, false)
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(target.root)
	if err != nil {
		return err
	}
	if clean == root {
		return fmt.Errorf("the Explorer root cannot be removed")
	}
	if recursive {
		return os.RemoveAll(clean)
	}
	return os.Remove(clean)
}

func validateLocalTarget(root, target string, allowMissing bool) (string, error) {
	if strings.ContainsRune(root, 0) || strings.ContainsRune(target, 0) {
		return "", fmt.Errorf("invalid path")
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	canonicalRoot, err = filepath.Abs(canonicalRoot)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !pathWithin(canonicalRoot, absolute) {
		return "", fmt.Errorf("path must stay inside the Explorer root")
	}
	info, statErr := os.Lstat(absolute)
	if statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symbolic links cannot be changed here")
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", err
		}
		if !pathWithin(canonicalRoot, resolved) {
			return "", fmt.Errorf("path resolves outside the Explorer root")
		}
		return resolved, nil
	}
	if !allowMissing {
		return "", statErr
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	if !pathWithin(canonicalRoot, parent) {
		return "", fmt.Errorf("parent resolves outside the Explorer root")
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return "", fmt.Errorf("parent folder is unavailable")
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func writeFileAtomic(target string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("only regular files can be saved")
		}
		if mode == 0 {
			mode = info.Mode().Perm()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	dir := filepath.Dir(target)
	temp, err := os.CreateTemp(dir, ".gitviewer-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(name)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Chmod(mode.Perm()); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func (a *App) DuplicatePath(value string) {
	a.runFileOperation(func(ctx context.Context) (func(), error) {
		source, err := a.targetForPath(value)
		if err != nil {
			return nil, err
		}
		destination := source
		if source.remote {
			destination.ref.Path = duplicateRemotePath(source.ref.Path, func(candidate string) bool {
				client := a.remoteClient(source.ref.Host, source.ref.Root)
				if client == nil {
					return true
				}
				_, err := client.Stat(ctx, candidate)
				return err == nil
			})
		} else {
			destination.ref.Path = duplicateLocalPath(source.ref.Path)
		}
		if err := a.copyTarget(ctx, source, destination); err != nil {
			return nil, err
		}
		return func() {
			a.afterFileMutation(source)
			a.Message = "Duplicated as " + path.Base(destination.ref.Path)
		}, nil
	})
}

func duplicateLocalPath(source string) string {
	dir, name := filepath.Split(source)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 10000; i++ {
		suffix := " copy"
		if i > 1 {
			suffix = fmt.Sprintf(" copy %d", i)
		}
		candidate := filepath.Join(dir, base+suffix+ext)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return filepath.Join(dir, base+" copy"+ext)
}

func duplicateRemotePath(source string, exists func(string) bool) string {
	dir, name := path.Split(source)
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 10000; i++ {
		suffix := " copy"
		if i > 1 {
			suffix = fmt.Sprintf(" copy %d", i)
		}
		candidate := path.Join(dir, base+suffix+ext)
		if !exists(candidate) {
			return candidate
		}
	}
	return path.Join(dir, base+" copy"+ext)
}

func (a *App) copyTarget(ctx context.Context, source, destination fileTarget) error {
	if source.remote {
		client := a.remoteClient(source.ref.Host, source.ref.Root)
		if client == nil {
			return fmt.Errorf("remote connection is unavailable")
		}
		info, err := client.Stat(ctx, source.ref.Path)
		if err != nil {
			return err
		}
		if info.Dir || info.Symlink {
			return fmt.Errorf("remote folder duplication is not supported yet")
		}
		if info.Size > maxEditorFileSize {
			return fmt.Errorf("remote file is too large to duplicate")
		}
		data, err := client.ReadFile(ctx, source.ref.Path, maxEditorFileSize)
		if err != nil {
			return err
		}
		return client.WriteFileAtomic(ctx, destination.ref.Path, data, info.Mode)
	}
	info, err := os.Lstat(source.ref.Path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic links cannot be duplicated")
	}
	if info.Mode().IsRegular() {
		input, err := os.Open(source.ref.Path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(destination.ref.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			_ = os.Remove(destination.ref.Path)
			return copyErr
		}
		return closeErr
	}
	if !info.IsDir() {
		return fmt.Errorf("only files and folders can be duplicated")
	}
	return copyLocalDirectory(ctx, source.ref.Path, destination.ref.Path)
}

func copyLocalDirectory(ctx context.Context, source, destination string) error {
	if err := os.Mkdir(destination, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		src := filepath.Join(source, entry.Name())
		dst := filepath.Join(destination, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() {
			if err := copyLocalDirectory(ctx, src, dst); err != nil {
				return err
			}
			continue
		}
		input, err := os.Open(src)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (a *App) CopyPath(value string) {
	if a.CopyText != nil {
		a.CopyText(value)
		a.Message = "Copied path"
	}
}

func (a *App) RevealPath(value string) {
	target, err := a.targetForPath(value)
	if err != nil {
		a.Message = err.Error()
		return
	}
	if target.remote {
		a.Message = "Reveal is available for local files only"
		return
	}
	if runtime.GOOS != "darwin" {
		a.Message = "Reveal in Finder is available only on macOS"
		return
	}
	flag := "-R"
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		flag = ""
	}
	args := []string{}
	if flag != "" {
		args = append(args, flag)
	}
	args = append(args, value)
	if err := exec.Command("/usr/bin/open", args...).Run(); err != nil {
		a.Message = "Could not reveal the path in Finder"
		return
	}
	a.Message = "Revealed in Finder"
}

func (a *App) explorerPathMenu(m *ui.Menu, value string, directory bool) {
	if directory {
		if m.Item("New File…").Chosen() {
			a.NewFileAt(value)
		}
		if m.Item("New Folder…").Chosen() {
			a.NewFolderAt(value)
		}
		m.Separator()
	}
	if m.Item("Rename…").Chosen() {
		a.RenamePath(value)
	}
	if m.Item("Duplicate").Chosen() {
		a.DuplicatePath(value)
	}
	if m.Item("Delete…").Chosen() {
		a.DeletePath(value)
	}
	m.Separator()
	if m.Item("Copy Path").Chosen() {
		a.CopyPath(value)
	}
	if m.Item("Reveal in Finder").Chosen() {
		a.RevealPath(value)
	}
}

func (a *App) filePromptView(c *ui.Context) {
	ui.Modal(c, &a.FilePromptOpen, func() {
		width, height := c.Size()
		w := min(float32(560), max(float32(380), width-80))
		h := min(float32(330), max(float32(220), height-80))
		ui.Column(c).Label(a.FilePromptTitle).Width(w).MaxHeight(h).Gap(0).
			Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(10).Clip().Children(func() {
			ui.Row(c).Height(44).FillWidth().Padding(0, 16).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Text(c, a.FilePromptTitle).Bold().FontSize(14).Grow(1)
			})
			ui.Column(c).Grow(1).FillWidth().Padding(16).Gap(10).Children(func() {
				ui.Text(c, a.FilePromptDetail).FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Text(c, a.FilePromptPath).FontSize(11).TextColor(c.Theme().Text).NoWrap()
				var input ui.Element
				if a.FilePromptKind != "delete" && a.FilePromptKind != "replace-all" {
					input = ui.TextInput(c, &a.FilePromptValue).Label(a.FilePromptTitle).FillWidth().Height(30).AutoFocus()
					if input.Submitted() {
						a.submitFilePrompt()
						return
					}
				}
				if a.FilePromptError != "" {
					ui.Text(c, a.FilePromptError).FontSize(11).TextColor(c.Theme().Danger)
				}
			})
			ui.Row(c).Height(48).FillWidth().Padding(0, 16).Gap(8).AlignItems(ui.Center).Justify(ui.End).
				Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Button(c, "Cancel").Height(28).OnClick(func() { a.FilePromptOpen = false })
				label := "Create"
				if a.FilePromptKind == "rename" {
					label = "Rename"
				} else if a.FilePromptKind == "delete" {
					label = "Delete"
				} else if a.FilePromptKind == "replace-all" {
					label = "Replace All"
				}
				button := ui.Button(c, label).Height(28).OnClick(a.submitFilePrompt)
				if a.FilePromptKind == "delete" || a.FilePromptKind == "replace-all" {
					button.Background(c.Theme().Danger)
				}
			})
		})
	})
}
