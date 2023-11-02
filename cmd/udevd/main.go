package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/anpep/udevd/internal/kmod"
	"github.com/anpep/udevd/internal/monitor"
)

type handler struct {
	km *kmod.KMod
}

func (h *handler) HandleUevent(u monitor.Uevent) {
	if u.Action() == monitor.Add {
		modalias_path := filepath.Join("/sys", u.DevPath(), "modalias")
		b, err := ioutil.ReadFile(modalias_path)
		if err != nil {
			return
		}

		modalias := strings.TrimSuffix(string(b), "\n")
		if len(modalias) == 0 {
			return
		}

		// Load the module.
		if err := h.km.Load(modalias); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot load module %q: %v\n", string(modalias), err)
		}
		fmt.Printf("loaded module %q for device %q\n", modalias, u.DevPath())
	}
}

func testMonitor() error {
	km, err := kmod.NewKMod()
	if err != nil {
		return fmt.Errorf("cannot create kmod: %w", err)
	}
	mon, err := monitor.NewMonitor(&handler{km: km})
	if err != nil {
		return fmt.Errorf("cannot create device monitor: %w", err)
	}
	defer mon.Close()

	mon.Bind()
	return nil
}

func main() {
	if err := testMonitor(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
