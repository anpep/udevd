package kmod

import (
	"path/filepath"
	"strings"
	"syscall"
)

const modulesRoot = "/lib/modules"

func currentModulesRoot() (path string, err error) {
	// Get current kernel release.
	u := &syscall.Utsname{}
	if err := syscall.Uname(u); err != nil {
		return "", err
	}
	// Convert fixed-size int8 array into a rune slice.
	r := make([]rune, 0)
	for _, b := range u.Release {
		if b == '\x00' {
			break
		}
		r = append(r, rune(b))
	}
	return filepath.Join(modulesRoot, string(r)), nil
}

func modpathToModname(path string) modname {
	basename := filepath.Base(path)
	noext := strings.Split(basename, ".")[0]
	return modname(strings.ReplaceAll(noext, "-", "_"))
}
