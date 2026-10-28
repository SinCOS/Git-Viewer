package remotecli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gitviewer/internal/infrastructure/ssh"
)

const Version = "1"

const wrapper = `#!/bin/sh
# GITVIEWER_REMOTE_CLI_VERSION=1
set -eu

config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/gitviewer"
state_file="$config_dir/install"
endpoint_file="$config_dir/endpoint"
host=""
if [ -r "$state_file" ]; then
  host=$(sed -n 's/^host=//p' "$state_file" | head -n 1)
fi
delegate=0
wait_for_close=0

for arg in "$@"; do
  case "$arg" in
    --install-extension|--uninstall-extension|--list-extensions|--show-versions|--update-extensions|--disable-extension|--enable-extension|--extensions-dir|--extensions-dir=*|--user-data-dir|--user-data-dir=*|--profile|--profile=*|--help|-h|--version|-v|--status|--telemetry|--disable-telemetry|--log|--log=*|--verbose|--force|--locate-shell-integration-path|--remote|--sync)
      delegate=1
      ;;
    --wait)
      wait_for_close=1
      ;;
    --new-window|-n|--reuse-window|-r|--add|--goto|-g|--diff|--folder-uri|--file-uri|--folder-uri=*|--file-uri=*|--goto=*|-g=*|--)
      ;;
    -*)
      delegate=1
      ;;
  esac
done

if [ "$delegate" -eq 1 ]; then
  original=""
  delegated=""
  if [ -r "$state_file" ]; then
    original=$(sed -n 's/^original=//p' "$state_file" | head -n 1)
    delegated=$(sed -n 's/^delegate=//p' "$state_file" | head -n 1)
  fi
  if [ -z "$delegated" ]; then
    delegated=$original
  fi
  if [ -n "$delegated" ] && [ -x "$delegated" ]; then
    exec "$delegated" "$@"
  fi
  echo "gitviewer: this command needs an original VS Code code command, but none was found" >&2
  exit 1
fi

if [ -r "$endpoint_file" ] && command -v curl >/dev/null 2>&1; then
  socket=""
  token=""
  endpoint_host=""
  while IFS='=' read -r key value; do
    case "$key" in
      socket) socket=$value ;;
      token) token=$value ;;
      host) endpoint_host=$value ;;
    esac
  done < "$endpoint_file"
  if [ -n "$endpoint_host" ]; then
    host=$endpoint_host
  fi
  if [ -n "$socket" ] && [ -n "$token" ] && [ -n "$host" ] && [ -S "$socket" ]; then
    send_args() {
      printf '%s\0' "$token" "$host" "$PWD"
      for arg in "$@"; do
        printf '%s\0' "$arg"
      done
    }
    if send_args "$@" | curl --silent --show-error --fail --unix-socket "$socket" \
      -H 'Content-Type: application/x-gitviewer-argv' --data-binary @- \
      http://localhost/v1/open >/dev/null; then
      exit 0
    fi
  fi
fi

target=""
kind="folder"
new_window=0
goto_value=""
line=""
column=""
diff_left=""
diff_right=""
diff_state=0
skip_next=0
for arg in "$@"; do
  if [ "$skip_next" -eq 1 ]; then
    goto_value=$arg
    skip_next=0
    continue
  fi
  case "$arg" in
    --new-window|-n) new_window=1 ;;
    --goto|-g) skip_next=1 ;;
    --goto=*|-g=*) goto_value=${arg#*=} ;;
    --diff) diff_state=1 ;;
    --add|--reuse-window|-r|--wait|--folder-uri|--file-uri|--folder-uri=*|--file-uri=*) ;;
    --) ;;
    -*) ;;
    *)
      if [ "$diff_state" -eq 1 ]; then
        diff_left=$arg
        diff_state=2
      elif [ "$diff_state" -eq 2 ]; then
        diff_right=$arg
        diff_state=0
      elif [ -z "$target" ]; then
        target=$arg
      fi
      ;;
  esac
done
if [ -n "$goto_value" ]; then
  case "$goto_value" in
    *:*:*)
      column=${goto_value##*:}
      rest=${goto_value%:*}
      line=${rest##*:}
      target=${rest%:*}
      ;;
    *:*)
      line=${goto_value##*:}
      target=${goto_value%:*}
      ;;
    *)
      target=$goto_value
      ;;
  esac
fi
if [ -n "$diff_left" ]; then
  target=$diff_left
  kind="file"
fi
if [ -z "$target" ]; then
  target=$PWD
fi
case "$target" in
  /*) ;;
  *) target="$PWD/$target" ;;
esac
case "$target" in
  *.code-workspace) kind="workspace" ;;
  *)
    if [ -d "$target" ]; then
      kind="folder"
    else
      kind="file"
    fi
    ;;
esac
path64=$(printf '%s' "$target" | base64 | tr '+/' '-_' | tr -d '=\n')
link="gitviewer://open?host=$(printf '%s' "$host" | sed 's/[^A-Za-z0-9_.-]/%3F/g')&path64=$path64&kind=$kind&newWindow=$new_window"
if [ -n "$line" ]; then
  link="$link&line=$line"
fi
if [ -n "$column" ]; then
  link="$link&column=$column"
fi
if [ -n "$diff_right" ]; then
  diff64=$(printf '%s' "$diff_right" | base64 | tr '+/' '-_' | tr -d '=\n')
  link="$link&diffPath64=$diff64"
fi
if [ "$wait_for_close" -eq 1 ]; then
  echo "gitviewer: bridge unavailable; --wait cannot be honored by the deep-link fallback" >&2
fi
printf '\033]8;;%s\033\\%s\033]8;;\033\\\n' "$link" "$link"
`

const installScript = `set -eu
umask 077
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/gitviewer"
state_file="$config_dir/install"
mkdir -p "$config_dir" "$HOME/.local/bin" "$HOME/.local/share/gitviewer/code-backups"
original=""
backup=""
delegate=""
if [ -r "$state_file" ]; then
  original=$(sed -n 's/^original=//p' "$state_file" | head -n 1)
  backup=$(sed -n 's/^backup=//p' "$state_file" | head -n 1)
  delegate=$(sed -n 's/^delegate=//p' "$state_file" | head -n 1)
fi
found=$(command -v code 2>/dev/null || true)
target=""
if [ -n "$found" ]; then
  if head -n 2 "$found" 2>/dev/null | grep -q 'GITVIEWER_REMOTE_CLI_VERSION'; then
    target=$found
  elif [ -w "$found" ] && [ -w "$(dirname "$found")" ]; then
    backup="$HOME/.local/share/gitviewer/code-backups/code-$(date +%Y%m%d%H%M%S)-$$"
    cp -p "$found" "$backup"
    original=$found
    delegate=$backup
    target=$found
  fi
fi
if [ -z "$target" ]; then
  target="$HOME/.local/bin/code"
  if [ -e "$target" ] && ! head -n 2 "$target" 2>/dev/null | grep -q 'GITVIEWER_REMOTE_CLI_VERSION'; then
    if [ -z "$backup" ]; then
      backup="$HOME/.local/share/gitviewer/code-backups/code-$(date +%Y%m%d%H%M%S)-$$"
      cp -p "$target" "$backup"
    fi
    if [ -z "$original" ]; then
      original=$target
    fi
    delegate=$backup
  fi
fi
if [ -n "$found" ] && [ "$found" != "$target" ]; then
  original=${original:-$found}
  delegate=$found
fi
if [ -z "$delegate" ] && [ -n "$backup" ]; then
  delegate=$backup
fi
tmp="$target.gitviewer.$$"
cat > "$tmp" <<'GITVIEWER_WRAPPER'
__WRAPPER__
GITVIEWER_WRAPPER
chmod 755 "$tmp"
mv "$tmp" "$target"
ln -sf "$target" "$HOME/.local/bin/gitviewer"
{
  printf 'original=%s\n' "$original"
  printf 'backup=%s\n' "$backup"
  printf 'delegate=%s\n' "$delegate"
  printf 'target=%s\n' "$target"
  printf 'host=__HOST__\n'
  printf 'version=__VERSION__\n'
} > "$state_file.tmp.$$"
chmod 600 "$state_file.tmp.$$"
mv "$state_file.tmp.$$" "$state_file"
if [ -n "$found" ] && [ "$target" != "$found" ]; then
  echo "gitviewer: installed $target; put ~/.local/bin before the directory containing $found in PATH" >&2
fi
`

func script(host string) string {
	result := strings.ReplaceAll(installScript, "__WRAPPER__", wrapper)
	result = strings.ReplaceAll(result, "__VERSION__", Version)
	return strings.ReplaceAll(result, "__HOST__", host)
}

func Install(ctx context.Context, cfg ssh.Executor, host string) error {
	if !ssh.ValidHost(host) {
		return fmt.Errorf("invalid SSH host")
	}
	executable := cfg.SSHPath
	if executable == "" {
		executable = "ssh"
	}
	args := cfg.ConnectionArgs()
	args = append(args, host, "sh -s")
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, executable, args...)
	cmd.Stdin = strings.NewReader(script(host))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return runCtx.Err()
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = "remote command failed"
		}
		return fmt.Errorf("%s", detail)
	}
	return nil
}

// InstallStatus is the remote install record used to distinguish a first-time
// prompt from an automatic wrapper update.
type InstallStatus struct {
	Installed bool
	Original  string
	Backup    string
	Delegate  string
	Target    string
	Host      string
	Version   int
}

// Inspect reads the remote install record without modifying the host. A
// missing record is not an error.
func Inspect(ctx context.Context, cfg ssh.Executor, host string) (InstallStatus, error) {
	if !ssh.ValidHost(host) {
		return InstallStatus{}, fmt.Errorf("invalid SSH host")
	}
	executable := cfg.SSHPath
	if executable == "" {
		executable = "ssh"
	}
	remote := `set -eu
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/gitviewer"
state_file="$config_dir/install"
if [ ! -r "$state_file" ]; then
  exit 3
fi
sed -n 's/^original=/original=/p; s/^backup=/backup=/p; s/^delegate=/delegate=/p; s/^target=/target=/p; s/^host=/host=/p; s/^version=/version=/p' "$state_file"`
	encoded, err := ssh.Encode([]string{"sh", "-c", remote})
	if err != nil {
		return InstallStatus{}, err
	}
	args := cfg.ConnectionArgs()
	args = append(args, host, encoded)
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, executable, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return InstallStatus{}, runCtx.Err()
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
			return InstallStatus{}, nil
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = "cannot inspect remote code command"
		}
		return InstallStatus{}, fmt.Errorf("%s", detail)
	}
	status := InstallStatus{Installed: true}
	for _, line := range strings.Split(stdout.String(), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "original":
			status.Original = value
		case "backup":
			status.Backup = value
		case "delegate":
			status.Delegate = value
		case "target":
			status.Target = value
		case "host":
			status.Host = value
		case "version":
			status.Version, _ = strconv.Atoi(value)
		}
	}
	return status, nil
}
