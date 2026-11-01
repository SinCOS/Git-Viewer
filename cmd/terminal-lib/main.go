package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo/plugins/terminal"
)

func main() {
	destDir := "bin"
	if len(os.Args) > 1 {
		destDir = os.Args[1]
	}
	dest := filepath.Join(destDir, libraryName())
	if _, err := os.Stat(dest); err == nil {
		fmt.Println(dest)
		return
	}
	source, err := terminal.LibraryPath()
	if err != nil {
		fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		fatal(err)
	}
	dest = filepath.Join(destDir, filepath.Base(source))
	tmp, err := os.CreateTemp(destDir, "."+filepath.Base(dest)+".*")
	if err != nil {
		fatal(err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		fatal(err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		fatal(err)
	}
	if err := tmp.Close(); err != nil {
		fatal(err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		fatal(err)
	}
	fmt.Println(dest)
}

func libraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libghostty-vt.dylib"
	case "windows":
		return "ghostty-vt.dll"
	default:
		return "libghostty-vt.so"
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "terminal-lib:", err)
	os.Exit(1)
}
