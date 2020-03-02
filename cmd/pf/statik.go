// +build !linux

package main

import (
	"io"

	"github.com/rakyll/statik/fs"

	_ "github.com/sclevine/packfile/statik"
)

func getLinuxPF() (io.ReadCloser, error) {
	sfs, err := fs.New()
	if err != nil {
		return nil, err
	}
	return sfs.Open("/pf.linux")
}
