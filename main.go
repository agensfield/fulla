package main

import (
	"github.com/agensfield/fulla/internal/cli"
	"github.com/agensfield/fulla/internal/clipboard"
	"os"
	"syscall"
)

func main() {
	syscall.Umask(0o077)
	if len(os.Args) == 2 && os.Args[1] == clipboard.WorkerFlag {
		os.Exit(clipboard.Worker())
	}
	os.Exit((&cli.App{}).Main(os.Args[1:]))
}
