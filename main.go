package main

import (
	"fmt"
	"os"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "No config found. Run setup first.\n")
		os.Exit(1)
	}
	fmt.Printf("Vault: %s\n", cfg.Vault)
}
