package cnb

import (
	"io/ioutil"
	"os"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/exec"
	"github.com/sclevine/packfile/layers"
	"github.com/sclevine/packfile/link"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type planProvide struct {
	Name string `toml:"name"`
}

type planSections struct {
	Requires []link.Require `toml:"requires"`
	Provides []planProvide  `toml:"provides"`
}

func Detect(pf *packfile.Packfile, ctxDir, platformDir, planPath string) error {
	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	shell := packfile.DefaultShell
	if s := pf.Config.Shell; s != "" {
		shell = s
	}
	lock := sync.NewLock()
	var provides []planProvide
	var linkLayers []link.Layer
	for i := range pf.Layers {
		layer := &pf.Layers[i]
		if layer.Provide != nil || layer.Build != nil {
			provides = append(provides, planProvide{Name: layer.Name})
		}
		if layer.Require == nil && layer.Build == nil {
			continue
		}
		mdDir, err := ioutil.TempDir("", "packfile.md."+layer.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		detectLayer := &layers.Detect{
			Streamer: sync.NewStreamer(),
			Kernel:   sync.NewKernel(layer.Name, lock, false),
			Layer:    layer,
			AppDir:   appDir,
		}
		if require := layer.Require; require != nil {
			if require.Runner != nil {
				detectLayer.Metadata = metadata.NewMemory()
				detectLayer.RequireRunner = require.Runner
			} else {
				detectLayer.Metadata = metadata.NewFS(mdDir)
				detectLayer.RequireRunner = &exec.Exec{
					Exec:   shellOverride(require.Exec, shell),
					Name:   layer.Name,
					CtxDir: ctxDir,
				}
			}
		} else {
			detectLayer.Metadata = metadata.NewMemory()
		}
		linkLayers = append(linkLayers, detectLayer)
	}
	lock.Add(len(linkLayers))
	link.Layers(linkLayers)
	for i := range linkLayers {
		go func(i int) {
			defer linkLayers[i].Close()
			sync.RunNode(linkLayers[i])
		}(i)
	}
	for i := range linkLayers {
		linkLayers[i].Stream(os.Stdout, os.Stderr)
	}
	for i := range linkLayers {
		sync.WaitForNode(linkLayers[i])
	}
	requires, err := link.Requires(linkLayers)
	if err != nil {
		return err
	}
	return writeTOML(planSections{requires, provides}, planPath)
}
