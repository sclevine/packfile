package cnb

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/exec"
	"github.com/sclevine/packfile/layers"
	"github.com/sclevine/packfile/link"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type buildPlan struct {
	Entries []link.Require `toml:"entries"`
}

func (b buildPlan) get(name string) []link.Require {
	var out []link.Require
	for _, e := range b.Entries {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

type launchTOML struct {
	Processes []packfile.Process `toml:"processes"`
	Slices    []packfile.Slice   `toml:"slices"`
}

type buildStore struct {
	Metadata struct {
		BuildID string `toml:"build-id"`
	} `toml:"metadata"`
}

func Build(pf *packfile.Packfile, ctxDir, layersDir, platformDir, planPath string) error {
	if pf.Config.ID != "" && pf.Config.Version != "" {
		var name string
		if n := pf.Config.Name; n != "" {
			name = " - " + n
		}
		fmt.Printf("Executing buildpack: %s@%s%s\n", pf.Config.ID, pf.Config.Version, name)
	}

	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	shell := packfile.DefaultShell
	if s := pf.Config.Shell; s != "" {
		shell = s
	}
	storePath := filepath.Join(layersDir, "store.toml")
	var store buildStore
	if _, err := toml.DecodeFile(storePath, &store); os.IsNotExist(err) {
		store = buildStore{}
	} else if err != nil {
		return err
	}
	lastBuildID := store.Metadata.BuildID
	store.Metadata.BuildID = uuid.New().String()
	var plan buildPlan
	if _, err := toml.DecodeFile(planPath, &plan); err != nil {
		return err
	}
	var streamLayers []layers.StreamLayer
	layerNames := map[string]struct{}{}
	for i := range pf.Caches {
		cache := &pf.Caches[i]
		layerNames[cache.Name] = struct{}{}
		cacheLayer := &layers.Cache{
			Streamer: sync.NewStreamer(),
			Share: link.Share{
				LayerDir: filepath.Join(layersDir, pf.Caches[i].Name),
			},
			Cache:  cache,
			AppDir: appDir,
		}
		if setup := cache.Setup; setup != nil {
			if setup.Runner != nil {
				cacheLayer.SetupRunner = setup.Runner
			} else {
				cacheLayer.SetupRunner = &exec.Exec{
					Exec: shellOverride(setup.Exec, shell),
					Name: cache.Name,
					CtxDir: ctxDir,
				}
			}
		}
		streamLayers = append(streamLayers, cacheLayer)
	}
	for i := range pf.Layers {
		layer := &pf.Layers[i]
		layerNames[layer.Name] = struct{}{}
		if layer.Provide != nil && layer.Build != nil {
			return xerrors.Errorf("layer '%s' has both provide and build sections", layer.Name)
		}
		if layer.Build == nil && layer.Provide == nil {
			continue
		}
		mdDir, err := ioutil.TempDir("", "packfile.md."+layer.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		layerDir := filepath.Join(layersDir, layer.Name)
		buildLayer := &layers.Build{
			Streamer: sync.NewStreamer(),
			Share: link.Share{
				LayerDir: layerDir,
			},
			Layer:       layer,
			Requires:    plan.get(layer.Name),
			AppDir:      appDir,
			BuildID:     store.Metadata.BuildID,
			LastBuildID: lastBuildID,
		}
		if test := layer.FindProvide().Test; test != nil {
			if test.Runner != nil {
				buildLayer.TestRunner = test.Runner
			} else {
				buildLayer.TestRunner = &exec.Exec{
					Exec: shellOverride(test.Exec, shell),
					Name: layer.Name,
					CtxDir: ctxDir,
				}
			}
		}
		if run := layer.FindProvide().Run; run != nil {
			if run.Runner != nil {
				buildLayer.Metadata = metadata.NewMemory()
				buildLayer.ProvideRunner = run.Runner
			} else {
				buildLayer.Metadata = metadata.NewFS(mdDir)
				buildLayer.ProvideRunner = &exec.Exec{
					Exec:   shellOverride(run.Exec, shell),
					Name:   layer.Name,
					CtxDir: ctxDir,
				}
			}
		}
		streamLayers = append(streamLayers, buildLayer)
	}
	if err := eachDir(layersDir, func(name string) error {
		if _, ok := layerNames[name]; !ok {
			if err := os.RemoveAll(filepath.Join(layersDir, name)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	linkLayers := toLinkLayers(streamLayers)
	syncLayers := link.Layers(linkLayers)
	for i := range syncLayers {
		go func(i int) {
			defer streamLayers[i].Close()
			syncLayers[i].Run()
		}(i)
	}
	for i := range streamLayers {
		streamLayers[i].Stream(os.Stdout, os.Stderr)
	}
	for i := range syncLayers {
		syncLayers[i].Wait()
	}
	if err := writeTOML(launchTOML{
		Processes: pf.Processes,
		Slices:    pf.Slices,
	}, filepath.Join(layersDir, "launch.toml")); err != nil {
		return err
	}
	requires, err := link.Requires(linkLayers)
	if err != nil {
		return err
	}
	if err := writeTOML(buildPlan{requires}, planPath); err != nil {
		return err
	}
	return writeTOML(store, storePath)
}
