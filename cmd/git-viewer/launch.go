package main

import (
	"flag"
	"io"
	"path/filepath"
	"strings"
)

type launchTarget struct {
	Kind      string
	Host      string
	Path      string
	Right     string
	NewWindow bool
	Wait      bool
	Add       bool
	Line      int
	Column    int
}

type launchOptions struct {
	Smoke       bool
	AutoRefresh bool
	ConfigPath  string
	Targets     []launchTarget
}

func parseLaunch(args []string, defaultConfig, workingDir string) (launchOptions, error) {
	options := launchOptions{AutoRefresh: true, ConfigPath: defaultConfig}
	fs := flag.NewFlagSet("git-viewer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&options.Smoke, "smoke", false, "verify native data loading and clean shutdown")
	repoPath := fs.String("repo", "", "open a local repository directory")
	sshHost := fs.String("ssh-host", "", "OpenSSH Host alias for remote files")
	remotePath := fs.String("remote-repo", "", "remote absolute folder or workspace path")
	workspacePath := fs.String("workspace", "", "open a VS Code workspace")
	fs.BoolVar(&options.AutoRefresh, "auto-refresh", true, "watch local repositories and poll remote repositories")
	fs.StringVar(&options.ConfigPath, "config", defaultConfig, "application recent repositories JSON path")
	newWindow := fs.Bool("new-window", false, "open a new window")
	wait := fs.Bool("wait", false, "wait until the opened window closes")
	add := fs.Bool("add", false, "add to the most recent matching remote window")
	gotoValue := fs.String("goto", "", "open file at line[:column]")
	if err := fs.Parse(args); err != nil {
		return launchOptions{}, err
	}
	if options.ConfigPath == "" {
		options.ConfigPath = defaultConfig
	}
	resolve := func(value string) string {
		if value == "" || filepath.IsAbs(value) {
			return value
		}
		return filepath.Join(workingDir, value)
	}
	line, column := parseLineColumn(*gotoValue)
	if *sshHost != "" {
		remote := *remotePath
		if remote == "" {
			remote = *workspacePath
		}
		if remote == "" && fs.NArg() > 0 {
			remote = fs.Arg(0)
		}
		if remote != "" {
			kind := "remote-folder"
			if strings.HasSuffix(strings.ToLower(remote), ".code-workspace") {
				kind = "remote-workspace"
			}
			options.Targets = append(options.Targets, launchTarget{
				Kind: kind, Host: *sshHost, Path: remote, NewWindow: *newWindow, Wait: *wait, Add: *add,
				Line: line, Column: column,
			})
		}
	} else if *workspacePath != "" {
		options.Targets = append(options.Targets, launchTarget{
			Kind: "local-workspace", Path: resolve(*workspacePath), NewWindow: *newWindow, Wait: *wait,
		})
	} else if *repoPath != "" {
		options.Targets = append(options.Targets, launchTarget{
			Kind: "local-folder", Path: resolve(*repoPath), NewWindow: *newWindow, Wait: *wait,
		})
	} else if fs.NArg() > 0 {
		for _, value := range fs.Args() {
			kind := "local-folder"
			if strings.HasSuffix(strings.ToLower(value), ".code-workspace") {
				kind = "local-workspace"
			}
			options.Targets = append(options.Targets, launchTarget{
				Kind: kind, Path: resolve(value), NewWindow: *newWindow, Wait: *wait,
			})
		}
	}
	return options, nil
}

func parseLineColumn(value string) (int, int) {
	if value == "" {
		return 0, 0
	}
	var line, column int
	parts := strings.Split(value, ":")
	if len(parts) >= 2 {
		line = atoiPositive(parts[len(parts)-2])
		column = atoiPositive(parts[len(parts)-1])
	}
	return line, column
}

func atoiPositive(value string) int {
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
