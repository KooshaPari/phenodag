package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	fmt.Fprintln(os.Stderr, "DEPRECATED: extend3-v3 is deprecated, use extend3-v2 instead")

	args := []string{"extend3-v2"}
	args = append(args, os.Args[1:]...)

	cmd := exec.Command("phenodag", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
