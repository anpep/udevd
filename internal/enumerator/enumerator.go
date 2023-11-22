package enumerator

import (
	"io/fs"
	"os"
	"path/filepath"
)

const sysfsPath = "/sys"

func Enumerate(classes []string) (ueFilePaths []string, err error) {
	err = filepath.WalkDir(filepath.Join(sysfsPath, "bus"), func(path string, d fs.DirEntry, _ error) error {
		if !d.Type().IsRegular() || d.Name() != "uevent" {
			return nil
		}
		ueFilePaths = append(ueFilePaths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(classes) == 0 {
		// Enumerate all behind /class
		err = filepath.WalkDir(filepath.Join(sysfsPath, "class"), func(path string, d fs.DirEntry, _ error) error {
			if d.Type()&fs.ModeSymlink == 0 {
				return nil
			}
			ueventPath := filepath.Join(path, "uevent")
			_, err := os.Stat(ueventPath)
			if os.IsNotExist(err) {
				return nil
			} else if err != nil {
				return err
			}
			ueFilePaths = append(ueFilePaths, ueventPath)
			return nil
		})
	} else {
		for _, class := range classes {
			classPath := filepath.Join(sysfsPath, "class", class)
			d, err := os.Open(classPath)
			if err != nil {
				return nil, err
			}
			entries, err := d.ReadDir(0)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				if entry.Type()&fs.ModeSymlink == 0 {
					continue
				}
				ueventPath := filepath.Join(classPath, entry.Name(), "uevent")
				_, err := os.Stat(ueventPath)
				if os.IsNotExist(err) {
					return nil, err
				} else if err != nil {
					return nil, err
				}
				ueFilePaths = append(ueFilePaths, ueventPath)
			}
		}
	}

	if err != nil {
		return nil, err
	}

	return ueFilePaths, err
}

func Trigger(ueFilePath string) error {
	f, err := os.OpenFile(ueFilePath, os.O_EXCL|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("add"); err != nil {
		return err
	}
	return nil
}
