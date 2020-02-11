package cnb

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/layers"
	"github.com/sclevine/packfile/sync"
)

type buildPlan struct {
	Entries []layers.Require `toml:"entries"`
}

func (b buildPlan) get(name string) []layers.Require {
	var out []layers.Require
	for _, e := range b.Entries {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

type launchTOML struct {
	Processes []packfile.Process `toml:"processes"`
}

func Build(pf *packfile.Packfile, layersDir, platformDir, planPath string) error {
	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	shell := packfile.DefaultShell
	if s := pf.Config.Shell; s != "" {
		shell = s
	}
	var plan buildPlan
	if _, err := toml.DecodeFile(planPath, &plan); err != nil {
		return err
	}
	var linkLayers []layers.LinkLayer
	for i := range pf.Caches {
		linkLayers = append(linkLayers, &layers.Cache{
			Streamer: sync.NewStreamer(),
			LinkShare: layers.LinkShare{
				LayerDir: filepath.Join(layersDir, pf.Caches[i].Name),
			},
			Cache:  &pf.Caches[i],
			Shell:  shell,
			AppDir: appDir,
		})
	}
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		if lp.Provide != nil && lp.Build != nil {
			return xerrors.Errorf("layer '%s' has both provide and build sections", lp.Name)
		}
		// TODO: don't allow provide() to return nil, add test() with same idea?
		if lp.Build == nil && lp.Provide == nil && lp.Require != nil {
			continue
		}
		// TODO: move metadata dir into individual layer Init/Cleanup methods
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		layerDir := filepath.Join(layersDir, lp.Name)
		linkLayers = append(linkLayers, &layers.Build{
			Streamer: sync.NewStreamer(),
			LinkShare: layers.LinkShare{
				MetadataDir: mdDir,
				LayerDir:    layerDir,
			},
			Layer:    lp,
			Requires: plan.get(lp.Name),
			Shell:    shell,
			AppDir:   appDir,
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
	if err := writeTOML(launchTOML{
		Processes: pf.Processes,
	}, filepath.Join(layersDir, "launch.toml")); err != nil {
		return err
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
	return toml.NewEncoder(f).Encode(buildPlan{requires})
}

