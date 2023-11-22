package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/anpep/udevd/internal/enumerator"
	"github.com/anpep/udevd/internal/kmod"
	"github.com/anpep/udevd/internal/monitor"
)

const sysfsMountpoint = "/sys"

type devmgr struct {
	wg  sync.WaitGroup
	km  *kmod.KMod
	mon *monitor.Monitor
}

func newDevmgr() (dm *devmgr, err error) {
	dm = &devmgr{}
	if dm.km, err = kmod.NewKMod(); err != nil {
		return nil, fmt.Errorf("cannot create kmod: %w", err)
	}
	if dm.mon, err = monitor.NewMonitor(dm); err != nil {
		return nil, fmt.Errorf("cannot create device monitor: %w", err)
	}
	return
}

func (dm *devmgr) Close() error {
	return dm.mon.Close()
}

func (dm *devmgr) Run() error {
	dm.wg.Add(1)
	go dm.mon.Bind(&dm.wg)
	dm.wg.Wait()
	return nil
}

func (dm *devmgr) HandleUevent(u monitor.Uevent) {
	if u.Action() == monitor.Add {
		modalias, err := deviceModalias(u.DevPath())
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot find modalias for device %q: %v\n", u.DevPath(), err)
			return
		}
		if err := dm.km.Load(modalias); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot load module for device %q: %v\n", u.DevPath(), err)
			return
		}
	}
}

func deviceModalias(devpath string) (string, error) {
	modaliasPath := filepath.Join(sysfsMountpoint, devpath, "modalias")
	f, err := os.Open(modaliasPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(b), "\n"), nil
}

func main() {
	dm, err := newDevmgr()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer dm.Close()

	if len(os.Args) > 1 && (os.Args[1] == "-e" || os.Args[1] == "--enumerate") {
		devs, err := enumerator.Enumerate([]string{
			"ata_device", "block", "mmc_host", "nvme", "nvme-generic", "nvme-subsystem",
			"phy", "scsi_device", "scsi_disk", "scsi_generic", "scsi_host", "net",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, dev := range devs {
			fmt.Println(dev)
			if err := enumerator.Trigger(dev); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
		}
	} else {
		if err := dm.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}
