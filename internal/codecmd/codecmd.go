package codecmd

import (
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
)

var ErrInvalid = errors.New("invalid code arguments")

type TargetKind string

const (
	TargetAuto      TargetKind = "auto"
	TargetFolder    TargetKind = "folder"
	TargetFile      TargetKind = "file"
	TargetWorkspace TargetKind = "workspace"
)

type Target struct {
	Path string
	Kind TargetKind
}

type Goto struct {
	Path   string
	Line   int
	Column int
}

type Diff struct {
	Left  string
	Right string
}

type Request struct {
	NewWindow   bool
	ReuseWindow bool
	Add         bool
	Wait        bool
	Targets     []Target
	Goto        *Goto
	Diff        *Diff
}

type Result struct {
	Request  Request
	Delegate bool
}

// Parse recognizes editor-opening arguments. Extension management, version
// queries and unknown options are delegated to the backed-up original code.
func Parse(args []string) (Result, error) {
	var result Result
	req := &result.Request
	var targets []string
	stopFlags := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !stopFlags && arg == "--" {
			stopFlags = true
			continue
		}
		if !stopFlags && strings.HasPrefix(arg, "-") && arg != "-" {
			name, value, hasValue := splitFlag(arg)
			switch name {
			case "-n", "--new-window":
				req.NewWindow = true
			case "-r", "--reuse-window":
				req.ReuseWindow = true
			case "--add":
				req.Add = true
			case "--wait":
				req.Wait = true
			case "-g", "--goto":
				if !hasValue {
					i++
					if i >= len(args) {
						return Result{}, ErrInvalid
					}
					value = args[i]
				}
				target, err := parseGoto(value)
				if err != nil {
					return Result{}, err
				}
				req.Goto = &target
			case "--diff":
				left, right := value, ""
				if !hasValue {
					i++
					if i >= len(args) {
						return Result{}, ErrInvalid
					}
					left = args[i]
					i++
					if i >= len(args) {
						return Result{}, ErrInvalid
					}
					right = args[i]
				} else {
					i++
					if i >= len(args) {
						return Result{}, ErrInvalid
					}
					right = args[i]
				}
				if left == "" || right == "" {
					return Result{}, ErrInvalid
				}
				req.Diff = &Diff{Left: left, Right: right}
			case "--folder-uri", "--file-uri":
				if !hasValue {
					i++
					if i >= len(args) {
						return Result{}, ErrInvalid
					}
					value = args[i]
				}
				target, err := parseURI(value, name == "--folder-uri")
				if err != nil {
					return Result{}, err
				}
				req.Targets = append(req.Targets, target)
			default:
				result.Delegate = true
				return result, nil
			}
			continue
		}
		targets = append(targets, arg)
	}
	if req.Add && req.NewWindow {
		return Result{}, ErrInvalid
	}
	if req.Diff != nil && len(targets) > 0 {
		return Result{}, ErrInvalid
	}
	for _, target := range targets {
		req.Targets = append(req.Targets, targetForPath(target))
	}
	if req.Goto != nil {
		req.Targets = append(req.Targets, targetForPath(req.Goto.Path))
	}
	return result, nil
}

func splitFlag(arg string) (name, value string, hasValue bool) {
	if i := strings.IndexByte(arg, '='); i >= 0 {
		return arg[:i], arg[i+1:], true
	}
	return arg, "", false
}

func targetForPath(value string) Target {
	kind := TargetAuto
	if strings.HasSuffix(strings.ToLower(value), ".code-workspace") {
		kind = TargetWorkspace
	}
	return Target{Path: value, Kind: kind}
}

func parseGoto(value string) (Goto, error) {
	file, line, column, ok := splitLocation(value)
	if !ok {
		return Goto{}, ErrInvalid
	}
	return Goto{Path: file, Line: line, Column: column}, nil
}

func splitLocation(value string) (string, int, int, bool) {
	if value == "" {
		return "", 0, 0, false
	}
	last := strings.LastIndexByte(value, ':')
	if last <= 0 || last == len(value)-1 {
		return value, 1, 1, true
	}
	lastValue, err := strconv.Atoi(value[last+1:])
	if err != nil {
		return value, 1, 1, true
	}
	before := value[:last]
	second := strings.LastIndexByte(before, ':')
	if second > 0 {
		if lineValue, lineErr := strconv.Atoi(before[second+1:]); lineErr == nil {
			if lineValue <= 0 || lastValue <= 0 {
				return "", 0, 0, false
			}
			return before[:second], lineValue, lastValue, true
		}
	}
	if lastValue <= 0 {
		return "", 0, 0, false
	}
	return before, lastValue, 1, true
}

func parseURI(raw string, folder bool) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, ErrInvalid
	}
	var file string
	switch {
	case u.Scheme == "file":
		file = u.Path
	case u.Scheme == "vscode-remote" && strings.HasPrefix(u.Host, "ssh-remote+"):
		file = u.Path
	default:
		return Target{}, ErrInvalid
	}
	if !path.IsAbs(file) || strings.ContainsRune(file, 0) {
		return Target{}, ErrInvalid
	}
	kind := TargetFile
	if folder || strings.HasSuffix(strings.ToLower(file), ".code-workspace") {
		kind = TargetFolder
		if strings.HasSuffix(strings.ToLower(file), ".code-workspace") {
			kind = TargetWorkspace
		}
	}
	return Target{Path: path.Clean(file), Kind: kind}, nil
}
