package kmod

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/sys/unix"
)

type KMod struct {
	*index
	mu sync.Mutex
}

func NewKMod() (km *KMod, err error) {
	km = &KMod{}
	if km.index, err = newIndex(); err != nil {
		return nil, err
	}
	return
}

func (km *KMod) Load(name string) (err error) {
	km.mu.Lock()
	defer km.mu.Unlock()

	var mod *module
	modname := modname(name)
	if mod, err = km.index.find(modname); err != nil {
		return err
	}

	if mod.builtin {
		return fmt.Errorf("module %q is built-in", modname)
	}

	// Obtain latest module states.
	if mod.state == modstateLive {
		// Module already loaded.
		return nil
	} else if mod.state != modstateGone {
		// Module either loading or unloading.
		return fmt.Errorf("module %q is busy", modname)
	}

	// Obtain real path of the module.
	root, err := currentModulesRoot()
	if err != nil {
		return err
	}
	realpath := filepath.Join(root, mod.path)

	f, err := os.Open(realpath)
	if err != nil {
		return err
	}
	defer f.Close()
	switch filepath.Ext(realpath) {
	case ".zst":
		d, err := zstd.NewReader(f)
		if err != nil {
			return err
		}
		defer d.Close()
		return loadFromReader(d)
	default:
		return loadFromFd(f.Fd())
	}
}

func loadFromFd(fd uintptr) error {
	return unix.FinitModule(int(fd), "", 0)
}

func loadFromReader(r io.Reader) error {
	image, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if err := unix.InitModule(image, ""); err != nil {
		return err
	}
	return nil
}
