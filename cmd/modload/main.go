package main

import (
	"fmt"
	"os"

	"github.com/anpep/udevd/internal/kmod"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("usage: modload MOD1 [MODN...]")
		os.Exit(1)
	}

	km, err := kmod.NewKMod()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, modname := range os.Args[1:] {
		if err := km.Load(modname); err != nil {
			fmt.Fprintf(os.Stderr, "error loading module %q: %v\n", modname, err)
			os.Exit(1)
		}
	}
}
