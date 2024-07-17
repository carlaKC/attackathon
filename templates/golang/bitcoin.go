package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

// Mine n blocks using bitcoin cli.
func Mine(n int) error {
	// Santiy check for fat fingering to stop us setting our laptops on
	// fire, can be removed.
	if n > 1000 {
		return fmt.Errorf("trying to mine: %v blocks failed "+
			"santiy check", n)
	}

	cmd := exec.Command("bitcoin-cli", "-generate", strconv.Itoa(n))

	_, err := cmd.Output()
	return err
}
