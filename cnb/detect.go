package cnb

import (
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/layers"
	"github.com/sclevine/packfile/sync"
)

type planProvide struct {
	Name string `toml:"name"`
}

type planSections struct {
	Requires []layers.Require `toml:"requires"`
	Provides []planProvide    `toml:"provides"`
}

func Detect(pf *packfile.Packfile, platformDir, planPath string) error {
	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	shell := packfile.DefaultShell
	if s := pf.Config.Shell; s != "" {
		shell = s
	}
	var provides []planProvide
	var linkLayers []layers.LinkLayer
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		if lp.Provide != nil || lp.Build != nil {
			provides = append(provides, planProvide{Name: lp.Name})
		}
		if lp.Require == nil && lp.Build == nil {
			continue
		}
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		linkLayers = append(linkLayers, &layers.Detect{
			Streamer: sync.NewStreamer(),
			LinkShare: layers.LinkShare{
				MetadataDir: mdDir,
			},
			Layer:  lp,
			Shell:  shell,
			AppDir: appDir,
		})
	}
	syncLayers := layers.ToSyncLayers(linkLayers)
	for i := range syncLayers {
		go func(i int) {
			defer linkLayers[i].Close()
			syncLayers[i].Run()
		}(i)
	}
	for i := range linkLayers {
		linkLayers[i].Stream(os.Stdout, os.Stderr)
	}
	for i := range syncLayers {
		syncLayers[i].Wait()
	}
	requires, err := layers.ReadRequires(linkLayers)
	if err != nil {
		return err
	}
	f, err := os.Create(planPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(planSections{requires, provides})
}

