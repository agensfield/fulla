package main

import (
	"github.com/agensfield/fulla/internal/cli"
	"os"
	"syscall"
)

func main() { syscall.Umask(0o077); os.Exit((&cli.App{}).Main(os.Args[1:])) }
