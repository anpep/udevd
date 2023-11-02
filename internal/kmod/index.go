package kmod

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type modname string

type modstate int

const (
	// modstateGone indicates the module is not currently loaded.
	modstateGone modstate = iota
	// modstateLive is the normal state.
	modstateLive
	// modstateComing indicates the module is being loaded.
	modstateComing
	// modstateComing indicates the module is being unloaded.
	modstateGoing
)

type module struct {
	builtin bool
	state   modstate
	path    string
	deps    []modname
}

type index struct {
	modroot, procroot string
	modules           map[modname]*module
	aliases           map[string]modname
}

const builtinName = "modules.builtin"
const moddepsName = "modules.dep"
const modaliasName = "modules.alias"

func newIndex() (i *index, err error) {
	i = &index{
		modules: make(map[modname]*module),
		aliases: make(map[string]modname),
	}
	if i.modroot, err = currentModulesRoot(); err != nil {
		return nil, err
	}
	// TODO: Make sure this is the right procfs mountpoint.
	i.procroot = "/proc"

	// Find built-in modules.
	f, err := os.Open(filepath.Join(i.modroot, builtinName))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		mod := &module{builtin: true, path: s.Text()}
		name := modpathToModname(mod.path)
		i.modules[name] = mod
	}
	if s.Err() != nil && s.Err() != io.EOF {
		return nil, s.Err()
	}

	// Find module dependencies.
	f, err = os.Open(filepath.Join(i.modroot, moddepsName))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s = bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		split := strings.SplitN(s.Text(), ":", 2)
		if len(split) != 2 {
			// Invalid syntax. Ignore this line
			continue
		}

		// Path before the colon is the module path.
		// Paths after the colon are dependency paths.
		modpath, deppaths := split[0], strings.Fields(split[1])
		mod := &module{builtin: false, path: modpath, deps: make([]modname, 0, len(deppaths))}
		name := modpathToModname(mod.path)
		for _, deppath := range deppaths {
			mod.deps = append(mod.deps, modpathToModname(deppath))
		}
		i.modules[name] = mod
	}
	if s.Err() != nil && s.Err() != io.EOF {
		return nil, s.Err()
	}

	// Find module aliases.
	f, err = os.Open(filepath.Join(i.modroot, modaliasName))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s = bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) != 3 || (len(fields) > 0 && fields[0] != "alias") {
			// Invalid syntax. Ignore this line.
			continue
		}
		alias, name := wildcardToRegexp(fields[1]), fields[2]
		i.aliases[alias] = modname(name)
	}
	if s.Err() != nil && s.Err() != io.EOF {
		return nil, s.Err()
	}

	// Find current modules' states.
	if err := i.refreshStates(); err != nil {
		return nil, err
	}
	return i, nil
}

func wildcardToRegexp(wildcard string) string {
	regexp := regexp.QuoteMeta(wildcard)
	return strings.ReplaceAll(regexp, "\\*", ".*")
}

func (i *index) find(name modname) (*module, error) {
	if mod, ok := i.modules[name]; ok {
		return mod, nil
	} else if alias, ok := i.aliases[wildcardToRegexp(string(name))]; ok {
		// Got an exact alias match.
		return i.find(alias)
	} else {
		for pattern, modname := range i.aliases {
			if match, _ := regexp.MatchString(pattern, string(name)); match {
				return i.find(modname)
			}
		}
	}
	return nil, fmt.Errorf("cannot find module %q", name)
}

func (i *index) refreshStates() error {
	f, err := os.Open(filepath.Join(i.procroot, "modules"))
	if err != nil {
		return err
	}
	defer f.Close()

	// Keep track of which modules have been unloaded.
	gone := make(map[modname]bool, len(i.modules))
	for name := range i.modules {
		gone[name] = true
	}

	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 6 {
			// Invalid syntax. Ignore this line.
			continue
		}
		name, state := modname(fields[0]), fields[2]
		if mod, ok := i.modules[name]; ok {
			// We know this module is still there.
			gone[name] = false
			switch state {
			case "Live":
				mod.state = modstateLive
			case "Loading":
				mod.state = modstateComing
			case "Unloading":
				mod.state = modstateGoing
			default:
				// Invalid syntax. Ignore this line.
				continue
			}
		}
	}
	if s.Err() != nil && s.Err() != io.EOF {
		return s.Err()
	}

	// The rest of modules are gone.
	for name, gone := range gone {
		if gone {
			i.modules[name].state = modstateGone
		}
	}
	return nil
}
