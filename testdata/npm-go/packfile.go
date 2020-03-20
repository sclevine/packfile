package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/pf"
)

var BuildID string

var buildpack = &packfile.Packfile{
	API: "0.2",
	Config: packfile.Config{
		ID:      "sh.scl.npm",
		Version: "0.0.0",
		Name:    "NPM Packfile",
	},
	Caches: []packfile.Cache{
		{Name: "npm-cache"},
	},
	Layers: []packfile.Layer{
		{
			Name:   "nodejs",
			Expose: true,
			Export: true,
			Require: &packfile.Require{
				Run: nodeLayer{},
			},
		},
		{
			Name:   "modules",
			Expose: true,
			Build: &packfile.Provide{
				Run: modulesLayer{},
				Test: &packfile.Test{
					Run: modulesLayer{},
				},
				Env: packfile.Envs{
					Build: []packfile.Env{
						{Name: "NODE_PATH", Value: "{{.Layer}}/node_modules"},
					},
					Launch: []packfile.Env{
						{Name: "NODE_PATH", Value: "{{.Layer}}/node_modules"},
					},
				},
				Links: []packfile.Link{
					{Name: "npm-cache", PathEnv: "NPM_CACHE"},
				},
			},
		},
	},
}

type nodeLayer struct{}

func (nodeLayer) Require(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata) error {
	f, err := os.Open("package.json")
	if err != nil {
		return err
	}
	defer f.Close()
	var packageJSON struct {
		Engines struct {
			Node string
		}
	}
	if err := json.NewDecoder(f).Decode(&packageJSON); err != nil {
		return err
	}
	return md.Write(packageJSON.Engines.Node, "version")
}

type modulesLayer struct{}

func (modulesLayer) Test(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata) error {
	h := sha256.New()
	f, err := os.Open("package-lock.json")
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err !=  nil  {
		return err
	}
	nodeVersion, err := exec.Command("node", "-v").Output()
	if err != nil {
		return err
	}
	return md.Write(fmt.Sprintf("%x-%s", h.Sum(nil), nodeVersion), "version")
}

func (modulesLayer) Provide(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata, deps []packfile.Dep) error {
	cmd := exec.Command("npm", "ci", "--unsafe-perm", "--cache", env["NPM_CACHE"])
	cmd.Stdout = st.Stdout()
	cmd.Stderr = st.Stderr()
	if err := cmd.Run(); err != nil {
		return err
	}
	mv := exec.Command("mv", "node_modules", filepath.Join(env["LAYER"], "node_modules"))
	mv.Stdout = st.Stdout()
	mv.Stderr = st.Stderr()
	return mv.Run()
}

func (modulesLayer) Version() string {
	return BuildID
}

func main() {
	if err := pf.Run(buildpack); err != nil {
		log.Fatalf("Error: %s", err)
	}
}
