package main

import (
	"io"
	"os"
)

func getLinuxPF() (io.ReadCloser, error) {
	return os.Open(os.Args[0])
}
