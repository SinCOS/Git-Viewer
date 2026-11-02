package domain

import (
	"context"
	"fmt"
	"os"
	"time"
)

type Operation string

const (
	Status         Operation = "status"
	Diff           Operation = "diff"
	History        Operation = "history"
	Branches       Operation = "branches"
	Worktrees      Operation = "worktrees"
	Graph          Operation = "graph"
	Stage          Operation = "stage"
	Unstage        Operation = "unstage"
	StageHunk      Operation = "stageHunk"
	UnstageHunk    Operation = "unstageHunk"
	DiscardHunk    Operation = "discardHunk"
	UndoCommit     Operation = "undoCommit"
	StashPush      Operation = "stashPush"
	StashUntracked Operation = "stashUntracked"
	StashPop       Operation = "stashPop"
	Discard        Operation = "discard"
	Commit         Operation = "commit"
	Amend          Operation = "amend"
	SwitchBranch   Operation = "switchBranch"
	CreateBranch   Operation = "createBranch"
	CherryPick     Operation = "cherryPick"
	Revert         Operation = "revert"
	Fetch          Operation = "fetch"
	FetchPrune     Operation = "fetchPrune"
	FetchAll       Operation = "fetchAll"
	PullFFOnly     Operation = "pullFfOnly"
	PullRebase     Operation = "pullRebase"
	PullFrom       Operation = "pullFrom"
	Push           Operation = "push"
	PushTo         Operation = "pushTo"
	Sync           Operation = "sync"
)

// GitOperation is the existing JSON command DTO. The executor validates
// operation-specific parameters before creating any subprocess.
type GitOperation struct {
	RepositoryID string    `json:"repositoryId"`
	Operation    Operation `json:"operation"`
	Paths        []string  `json:"paths,omitempty"`
	Ref          string    `json:"ref,omitempty"`
	Commit       string    `json:"commit,omitempty"`
	Message      string    `json:"message,omitempty"`
	Limit        int       `json:"limit,omitempty"`
	Patch        string    `json:"patch,omitempty"`
}
type Repository struct {
	RepositoryID string `json:"repositoryId"`
	Location     string `json:"location"`
	Path         string `json:"path"`
	DisplayName  string `json:"displayName"`
}

// EditorLocation identifies the transport that owns an editor path.
type EditorLocation string

const (
	LocationLocal EditorLocation = "local"
	LocationSSH   EditorLocation = "ssh"
)

// RemoteTarget is a remote folder or workspace requested by the CLI or UI.
type RemoteTarget struct {
	Host      string
	Path      string
	NewWindow bool
}

// EditorRoot is one root shown by Explorer. Remote roots always share one host
// per window.
type EditorRoot struct {
	Name         string
	Location     EditorLocation
	SSHHost      string
	Path         string
	RepositoryID string
}

// RemoteFileInfo is the bounded metadata shared by the remote editor and
// workspace layers. Paths use the remote host's POSIX conventions.
type RemoteFileInfo struct {
	Path    string
	Name    string
	Dir     bool
	Symlink bool
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}

// RemoteFileSystem is the editor-facing file API. Implementations confine all
// paths to the canonical root they were opened with.
type RemoteFileSystem interface {
	Canonical(context.Context, string) (string, error)
	Stat(context.Context, string) (RemoteFileInfo, error)
	ReadDir(context.Context, string) ([]RemoteFileInfo, error)
	ReadFile(context.Context, string, int64) ([]byte, error)
	WriteFileAtomic(context.Context, string, []byte, os.FileMode) error
	Mkdir(context.Context, string, os.FileMode) error
	Rename(context.Context, string, string) error
	Remove(context.Context, string, bool) error
	Walk(context.Context, string, map[string]bool) ([]string, error)
	Close() error
}

// RepositoryContext is internal execution state, not a persisted DTO.
type RepositoryContext struct {
	Repository Repository
	SSHHost    string
	GitDir     string
	CommonDir  string
}
type Change struct {
	Path           string  `json:"path"`
	IndexStatus    string  `json:"indexStatus"`
	WorktreeStatus string  `json:"worktreeStatus"`
	Untracked      bool    `json:"untracked"`
	Conflicted     bool    `json:"conflicted"`
	OldPath        *string `json:"oldPath,omitempty"`
}
type GitStatus struct {
	RepositoryID string   `json:"repositoryId"`
	FetchedAt    string   `json:"fetchedAt"`
	Stale        bool     `json:"stale"`
	Changes      []Change `json:"changes"`
}
type GitCommand struct {
	Repository RepositoryContext
	Operation  GitOperation
	Timeout    time.Duration
}
type GitResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}
type GitExecutor interface {
	Execute(context.Context, GitCommand) (GitResult, error)
}
type ErrorCode string

const (
	InvalidRepository     ErrorCode = "InvalidRepository"
	GitUnavailable        ErrorCode = "GitUnavailable"
	SshAuthFailed         ErrorCode = "SshAuthFailed"
	HostKeyRejected       ErrorCode = "HostKeyRejected"
	RemoteUnavailable     ErrorCode = "RemoteUnavailable"
	PermissionDenied      ErrorCode = "PermissionDenied"
	GitConflict           ErrorCode = "GitConflict"
	UnsafeOperation       ErrorCode = "UnsafeOperation"
	OperationCancelled    ErrorCode = "OperationCancelled"
	OperationStateUnknown ErrorCode = "OperationStateUnknown"
	UnsupportedGitVersion ErrorCode = "UnsupportedGitVersion"
	ParseError            ErrorCode = "ParseError"
	CommandFailed         ErrorCode = "CommandFailed"
	OperationTimedOut     ErrorCode = "OperationTimedOut"
	OutputLimitExceeded   ErrorCode = "OutputLimitExceeded"
	// ProtocolError is emitted only by the remote helper transport; it is not
	// produced by Git itself.
	ProtocolError ErrorCode = "ProtocolError"
)

type GitError struct {
	Code ErrorCode
	// Reason is a fixed application message, never raw subprocess output.
	Reason string
	Cause  error
}

func (e *GitError) Error() string { return fmt.Sprintf("Git operation failed (%s)", e.Code) }
func (e *GitError) Unwrap() error { return e.Cause }

type CommitRecord struct {
	OID        string
	Parents    []string
	Author     string
	AuthoredAt time.Time
	Subject    string
	Refs       string
}
type WorktreeRecord struct {
	Path        string
	Head        string
	Branch      string
	Bare        bool
	Detached    bool
	Locked      bool
	LockReason  string
	Prunable    bool
	PruneReason string
}
