package git

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	"strings"
	"sync"
	"time"
)

type WriteResult struct {
	Status   domain.GitStatus
	Verified bool
	Unknown  bool
}

// maxHunkPatch bounds the patch text a single hunk operation may carry. The
// diff preview itself is bounded, so this only guards direct callers.
const maxHunkPatch = 1 << 20

type writeGate struct {
	token chan struct{}
	users int
}

var writes = struct {
	sync.Mutex
	gates map[string]*writeGate
}{gates: map[string]*writeGate{}}

func acquire(ctx context.Context, key string) (func(), error) {
	writes.Lock()
	g := writes.gates[key]
	if g == nil {
		g = &writeGate{token: make(chan struct{}, 1)}
		g.token <- struct{}{}
		writes.gates[key] = g
	}
	g.users++
	writes.Unlock()
	releaseRef := func() {
		writes.Lock()
		g.users--
		if g.users == 0 {
			delete(writes.gates, key)
		}
		writes.Unlock()
	}
	select {
	case <-ctx.Done():
		releaseRef()
		return nil, &domain.GitError{Code: domain.OperationCancelled}
	case <-g.token:
	}
	if ctx.Err() != nil {
		g.token <- struct{}{}
		releaseRef()
		return nil, &domain.GitError{Code: domain.OperationCancelled}
	}
	return func() { g.token <- struct{}{}; releaseRef() }, nil
}
func (r LocalReader) Write(ctx context.Context, repo domain.RepositoryContext, op domain.GitOperation) (WriteResult, error) {
	result := WriteResult{}
	reject := func() (WriteResult, error) { return result, &domain.GitError{Code: domain.UnsafeOperation} }
	reason := func(text string) (WriteResult, error) {
		return result, &domain.GitError{Code: domain.UnsafeOperation, Reason: text}
	}
	if repo.Repository.RepositoryID == "" || op.RepositoryID != repo.Repository.RepositoryID || repo.CommonDir == "" {
		return reject()
	}
	if op.Limit != 0 {
		return reject()
	}
	if op.Commit != "" && op.Operation != domain.CreateBranch && op.Operation != domain.CherryPick && op.Operation != domain.Revert {
		return reject()
	}
	switch op.Operation {
	case domain.Stage, domain.Unstage, domain.Discard:
		if len(op.Paths) == 0 || op.Ref != "" || op.Message != "" || op.Patch != "" {
			return reject()
		}
	case domain.StageHunk, domain.UnstageHunk, domain.DiscardHunk:
		if len(op.Paths) != 1 || op.Ref != "" || op.Message != "" || op.Patch == "" ||
			len(op.Patch) > maxHunkPatch || strings.ContainsRune(op.Patch, 0) {
			return reject()
		}
	case domain.UndoCommit, domain.StashPush, domain.StashUntracked, domain.StashPop:
		if len(op.Paths) > 0 || op.Ref != "" || op.Message != "" || op.Patch != "" {
			return reject()
		}
	case domain.Commit, domain.Amend:
		if strings.TrimSpace(op.Message) == "" || len(op.Message) > 1024*1024 || strings.ContainsRune(op.Message, 0) || len(op.Paths) > 0 || op.Ref != "" || op.Patch != "" {
			return reject()
		}
	case domain.SwitchBranch:
		if op.Ref == "" || strings.HasPrefix(op.Ref, "-") || strings.ContainsAny(op.Ref, "\x00\r\n") || op.Message != "" || len(op.Paths) > 0 || op.Patch != "" {
			return reject()
		}
	case domain.CreateBranch:
		if op.Ref == "" || op.Commit == "" || op.Message != "" || len(op.Paths) > 0 || op.Patch != "" {
			return reject()
		}
	case domain.CherryPick, domain.Revert:
		if op.Commit == "" || op.Ref != "" || op.Message != "" || len(op.Paths) > 0 || op.Patch != "" {
			return reject()
		}
	case domain.Fetch, domain.FetchPrune, domain.FetchAll, domain.PullFFOnly, domain.PullRebase, domain.Push:
		if len(op.Paths) > 0 || op.Ref != "" || op.Message != "" || op.Patch != "" {
			return reject()
		}
	case domain.PullFrom, domain.PushTo:
		if len(op.Paths) > 0 || op.Ref == "" || op.Message != "" || op.Patch != "" {
			return reject()
		}
	default:
		return reject()
	}
	release, err := acquire(ctx, repo.Repository.Location+"\x00"+repo.SSHHost+"\x00"+repo.CommonDir)
	if err != nil {
		return result, err
	}
	defer release()
	status, err := r.Status(ctx, repo)
	if err != nil {
		return result, err
	}
	var args []string
	switch op.Operation {
	case domain.Stage, domain.Unstage:
		allowed := map[string]bool{}
		renamed := map[string]string{}
		for _, c := range status.Changes {
			if op.Operation == domain.Stage || c.IndexStatus != " " && c.IndexStatus != "?" {
				allowed[c.Path] = true
				if c.OldPath != nil {
					allowed[*c.OldPath] = true
					if op.Operation == domain.Unstage || c.WorktreeStatus == "R" || c.WorktreeStatus == "C" {
						renamed[c.Path] = *c.OldPath
					}
				}
			}
		}
		paths := []string{}
		seen := map[string]bool{}
		for _, p := range op.Paths {
			if !validPath(p) || !allowed[p] {
				return reject()
			}
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
			// Both halves of a rename belong to the selected file.
			if old, ok := renamed[p]; ok && !seen[old] {
				if !validPath(old) {
					return reject()
				}
				seen[old] = true
				paths = append(paths, old)
			}
		}
		args = []string{"add", "--"}
		if op.Operation == domain.Unstage {
			_, headErr := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", "HEAD")
			if headErr != nil {
				refs, e := r.run(ctx, repo.Repository.Path, "for-each-ref", "--count=1", "--format=%(objectname)")
				if e != nil || len(refs.Stdout) > 0 {
					return result, headErr
				}
				args = []string{"rm", "--cached", "--"}
			} else {
				args = []string{"restore", "--staged", "--"}
			}
		}
		args = append(args, paths...)
	case domain.StageHunk, domain.UnstageHunk, domain.DiscardHunk:
		path := op.Paths[0]
		if !validPath(path) || !validHunkPatch(op.Patch, path) {
			return reject()
		}
		found := false
		for _, c := range status.Changes {
			if c.Path != path || c.Untracked || c.Conflicted || c.OldPath != nil {
				continue
			}
			switch op.Operation {
			case domain.StageHunk:
				found = c.WorktreeStatus == "M"
			case domain.UnstageHunk:
				found = c.IndexStatus == "M"
			case domain.DiscardHunk:
				found = c.WorktreeStatus == "M"
			}
		}
		if !found {
			return reason("Select a modified file with a hunk that can be changed")
		}
		args = []string{"apply", "--unidiff-zero", "--whitespace=nowarn"}
		if op.Operation != domain.DiscardHunk {
			args = append(args, "--cached")
		}
		if op.Operation == domain.UnstageHunk || op.Operation == domain.DiscardHunk {
			args = append(args, "--reverse")
		}
		args = append(args, "-")
		r.Input = []byte(op.Patch)
	case domain.UndoCommit:
		if _, e := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", "HEAD^"); e != nil {
			return reason("There is no previous commit to undo")
		}
		args = []string{"reset", "--soft", "HEAD^"}
	case domain.StashPush:
		args = []string{"stash", "push"}
	case domain.StashUntracked:
		args = []string{"stash", "push", "--include-untracked"}
	case domain.StashPop:
		args = []string{"stash", "pop"}
	case domain.Discard:
		changes := map[string]domain.Change{}
		for _, c := range status.Changes {
			changes[c.Path] = c
		}
		paths := []string{}
		seen := map[string]bool{}
		for _, p := range op.Paths {
			c, ok := changes[p]
			if !validPath(p) || !ok || c.Untracked || c.Conflicted || c.OldPath != nil ||
				(c.IndexStatus != " " && c.IndexStatus != "M" && c.IndexStatus != "A") ||
				(c.WorktreeStatus != "M" && c.WorktreeStatus != "D") {
				return reason("Select tracked files with safely discardable unstaged changes")
			}
			if seen[p] {
				continue
			}
			// Only a single ordinary index entry can safely be restored. In particular,
			// restoring a submodule does not discard its nested working tree changes.
			entry, e := r.run(ctx, repo.Repository.Path, "ls-files", "--stage", "-z", "--", p)
			if e != nil {
				return result, e
			}
			fields, e := tokens(entry.Stdout)
			if e != nil || len(fields) != 1 {
				return reason("Cannot safely restore this index entry")
			}
			parts := strings.SplitN(string(fields[0]), "\t", 2)
			if len(parts) != 2 || parts[1] != p || !(strings.HasPrefix(parts[0], "100644 ") || strings.HasPrefix(parts[0], "100755 ") || strings.HasPrefix(parts[0], "120000 ")) || !strings.HasSuffix(parts[0], " 0") {
				return reason("Cannot safely restore this index entry")
			}
			// Git restore can remove untracked or ignored files in a directory
			// occupying a tracked file's path. Include ignored files in this check.
			others, e := r.run(ctx, repo.Repository.Path, "ls-files", "--others", "-z", "--", p)
			if e != nil {
				return result, e
			}
			if len(others.Stdout) != 0 {
				return reason("Untracked content occupies the selected path")
			}
			seen[p] = true
			paths = append(paths, p)
		}
		args = append([]string{"restore", "--worktree", "--"}, paths...)
	case domain.Commit, domain.Amend:
		staged := false
		for _, c := range status.Changes {
			if c.Conflicted {
				return reason("Resolve conflicted files before committing")
			}
			staged = staged || c.IndexStatus != " " && c.IndexStatus != "?"
		}
		if op.Operation == domain.Commit && !staged {
			return reason("No staged changes to commit")
		}
		args = []string{"commit", "--file=-"}
		if op.Operation == domain.Amend {
			if _, e := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", "HEAD^{commit}"); e != nil {
				return result, e
			}
			args = []string{"commit", "--amend", "--file=-"}
		}
		r.Input = []byte(op.Message)
	case domain.SwitchBranch:
		if len(status.Changes) > 0 {
			return reason("Working tree must be clean before switching branches")
		}
		branches, e := r.BranchList(ctx, repo)
		if e != nil {
			return result, e
		}
		found := false
		for _, b := range branches {
			if b.Name == "refs/heads/"+op.Ref {
				found = true
			}
		}
		if !found {
			return reason("Select an existing local branch")
		}
		trees, e := r.WorktreeList(ctx, repo)
		if e != nil {
			return result, e
		}
		for _, w := range trees {
			if w.Branch == "refs/heads/"+op.Ref && w.Path != repo.Repository.Path {
				return reason("Branch is checked out in another worktree")
			}
		}
		args = []string{"switch", "--no-guess", "--", op.Ref}
	case domain.CreateBranch:
		if len(status.Changes) > 0 {
			return reason("Working tree must be clean before creating a branch")
		}
		if !oidPattern.MatchString(op.Commit) || strings.HasPrefix(op.Ref, "-") || strings.ContainsAny(op.Ref, "\x00\r\n") {
			return reject()
		}
		if _, e := r.run(ctx, repo.Repository.Path, "check-ref-format", "--branch", op.Ref); e != nil {
			return reason("Enter a valid branch name")
		}
		if _, e := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", op.Commit+"^{commit}"); e != nil {
			return reason("Select a commit that exists in this repository")
		}
		args = []string{"switch", "-c", op.Ref, op.Commit}
	case domain.CherryPick, domain.Revert:
		if len(status.Changes) > 0 {
			return reason("Working tree must be clean before changing commits")
		}
		if !oidPattern.MatchString(op.Commit) {
			return reject()
		}
		if _, e := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", op.Commit+"^{commit}"); e != nil {
			return reason("Select a commit that exists in this repository")
		}
		action := "cherry-pick"
		if op.Operation == domain.Revert {
			action = "revert"
		}
		args = []string{action, "--no-edit", op.Commit}
	case domain.Fetch:
		args = []string{"fetch", "--progress"}
	case domain.FetchPrune:
		args = []string{"fetch", "--progress", "--prune"}
	case domain.FetchAll:
		args = []string{"fetch", "--progress", "--all"}
	case domain.PullFFOnly, domain.PullRebase, domain.PullFrom, domain.Push, domain.PushTo:
		pull := op.Operation == domain.PullFFOnly || op.Operation == domain.PullRebase || op.Operation == domain.PullFrom
		if pull && len(status.Changes) > 0 {
			return reason("Working tree must be clean before pulling")
		}
		remoteName, mergeRef := "", ""
		current := ""
		if op.Operation == domain.PullFrom || op.Operation == domain.PushTo {
			var ok bool
			remoteName, mergeRef, ok = splitRemoteRef(op.Ref)
			if !ok {
				return reject()
			}
		} else {
			branches, e := r.BranchList(ctx, repo)
			if e != nil {
				return result, e
			}
			upstream := ""
			for _, b := range branches {
				if b.Current {
					upstream = b.Upstream
					current = strings.TrimPrefix(b.Name, "refs/heads/")
				}
			}
			if upstream == "" || current == "" {
				return reason("Current branch has no upstream; configure it in Git first")
			}
			// Read Git's effective upstream remote and merge ref, then send explicit
			// arguments so push.default, mirror and push refspec config cannot widen scope.
			remote, e := r.run(ctx, repo.Repository.Path, "config", "--get", "branch."+current+".remote")
			if e != nil {
				return result, e
			}
			merge, e := r.run(ctx, repo.Repository.Path, "config", "--get", "branch."+current+".merge")
			if e != nil {
				return result, e
			}
			remoteName = strings.TrimSuffix(string(remote.Stdout), "\n")
			mergeRef = strings.TrimSuffix(string(merge.Stdout), "\n")
			if remoteName == "" || strings.HasPrefix(remoteName, "-") || strings.ContainsAny(remoteName, "\r\n\x00") || !strings.HasPrefix(mergeRef, "refs/heads/") || strings.ContainsAny(mergeRef, "\r\n\x00:") {
				return reject()
			}
		}
		switch op.Operation {
		case domain.PullFFOnly:
			args = []string{"pull", "--ff-only", "--no-rebase", "--progress", "--", remoteName, mergeRef}
		case domain.PullRebase:
			args = []string{"pull", "--rebase", "--progress", "--", remoteName, mergeRef}
		case domain.PullFrom:
			args = []string{"pull", "--ff-only", "--no-rebase", "--progress", "--", remoteName, mergeRef}
		case domain.Push:
			args = []string{"-c", "remote." + remoteName + ".mirror=false", "push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", remoteName, "refs/heads/" + current + ":" + mergeRef}
		case domain.PushTo:
			args = []string{"-c", "remote." + remoteName + ".mirror=false", "push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", remoteName, "HEAD:" + mergeRef}
		}
	}
	if op.Operation == domain.Fetch || op.Operation == domain.FetchPrune || op.Operation == domain.FetchAll ||
		op.Operation == domain.PullFFOnly || op.Operation == domain.PullRebase || op.Operation == domain.PullFrom ||
		op.Operation == domain.Push || op.Operation == domain.PushTo {
		// Git may itself invoke SSH for an upstream. Keep that second SSH hop
		// noninteractive and strict too, regardless of repository core.sshCommand.
		args = append([]string{"-c", "core.sshCommand=ssh -T -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=5 -o ConnectionAttempts=1 -o ClearAllForwardings=yes -o PermitLocalCommand=no -o RemoteCommand=none"}, args...)
	}
	r.Timeout = 2 * time.Minute
	r.WriteMode = true
	writeOutput, err := r.run(ctx, repo.Repository.Path, args...)
	var ge *domain.GitError
	if repo.Repository.Location == "ssh" && errors.As(err, &ge) && (ge.Code == domain.OperationCancelled || ge.Code == domain.OperationTimedOut || ge.Code == domain.RemoteUnavailable || ge.Code == domain.OutputLimitExceeded) {
		result.Unknown = true
		err = &domain.GitError{Code: domain.OperationStateUnknown, Reason: "Remote operation outcome is unknown; refresh before another write", Cause: err}
	}
	// Verification is independent of the canceled operation and never retries it.
	verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	r.Input = nil
	r.WriteMode = false
	r.Timeout = 10 * time.Second
	verified, geErr := r.Status(verifyCtx, repo)
	result.Status = verified
	result.Verified = geErr == nil
	if err == nil {
		err = geErr
	}
	if repo.Repository.Location == "ssh" && geErr != nil && !result.Unknown {
		result.Unknown = true
		err = &domain.GitError{Code: domain.OperationStateUnknown, Reason: "Remote operation outcome is unknown; refresh before another write", Cause: err}
	}
	if err != nil && errors.As(err, &ge) && ge.Code == domain.CommandFailed &&
		(op.Operation == domain.Fetch || op.Operation == domain.FetchPrune || op.Operation == domain.FetchAll ||
			op.Operation == domain.PullFFOnly || op.Operation == domain.PullRebase || op.Operation == domain.PullFrom ||
			op.Operation == domain.Push || op.Operation == domain.PushTo) {
		output := strings.ToLower(string(writeOutput.Stderr))
		switch {
		case strings.Contains(output, "host key verification failed"), strings.Contains(output, "remote host identification has changed"):
			err = &domain.GitError{Code: domain.HostKeyRejected, Cause: err}
		case strings.Contains(output, "permission denied (publickey"), strings.Contains(output, "authentication failed"):
			err = &domain.GitError{Code: domain.SshAuthFailed, Cause: err}
		}
	}
	if err != nil && errors.As(err, &ge) && ge.Code == domain.CommandFailed {
		text := "Git rejected the operation"
		switch op.Operation {
		case domain.Commit, domain.Amend:
			text = "Commit failed; check Git identity and commit hooks"
		case domain.Push, domain.PushTo:
			text = "Push failed or was rejected by the remote"
		case domain.PullFFOnly, domain.PullRebase, domain.PullFrom:
			text = "Pull failed; check the upstream and local changes"
		case domain.Fetch, domain.FetchPrune, domain.FetchAll:
			text = "Fetch failed; check remote access and authentication"
		}
		err = &domain.GitError{Code: ge.Code, Reason: text, Cause: err}
	}
	return result, err
}

// splitRemoteRef parses "remote:refs/heads/branch" for PullFrom and PushTo.
func splitRemoteRef(value string) (string, string, bool) {
	remote, ref, ok := strings.Cut(value, ":")
	if !ok || remote == "" || strings.HasPrefix(remote, "-") ||
		strings.ContainsAny(remote, "\x00\r\n\t :") ||
		!strings.HasPrefix(ref, "refs/heads/") || strings.ContainsAny(ref, "\x00\r\n\t :") {
		return "", "", false
	}
	return remote, ref, true
}

// validHunkPatch accepts the single-file, single-hunk patch the diff view
// builds for StageHunk and UnstageHunk. The caller has already checked the
// repository-relative path; this keeps a direct DTO caller from smuggling a
// second file or an unrelated header into the index write.
func validHunkPatch(patch, path string) bool {
	if path == "" || strings.ContainsAny(path, "\x00\r\n\t\"") {
		return false
	}
	lines := strings.Split(patch, "\n")
	diffHeaders, hunkHeaders, oldHeaders, newHeaders := 0, 0, 0, 0
	inHeader := true
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			diffHeaders++
			if line != "diff --git a/"+path+" b/"+path {
				return false
			}
		case strings.HasPrefix(line, "@@ "):
			hunkHeaders++
			inHeader = false
		case strings.HasPrefix(line, "Binary files "), line == "GIT binary patch":
			return false
		case inHeader && strings.HasPrefix(line, "--- "):
			oldHeaders++
			if line != "--- a/"+path {
				return false
			}
		case inHeader && strings.HasPrefix(line, "+++ "):
			newHeaders++
			if line != "+++ b/"+path {
				return false
			}
		}
	}
	return diffHeaders == 1 && hunkHeaders == 1 && oldHeaders == 1 && newHeaders == 1
}
