package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/pf"
)

var BuildID string

var buildpack = &packfile.Packfile{
	API: "0.2",
	Config: packfile.Config{
		ID:      "sh.scl.node-engine",
		Version: "0.0.0",
		Name:    "Node Engine Packfile",
	},
	Layers: []packfile.Layer{
		{
			Name:  "nodejs",
			Store: true,
			Provide: &packfile.Provide{
				Run: nodeLayer{},
				Test: &packfile.Test{
					Run: nodeLayer{},
				},
				Deps: []packfile.Dep{
					{
						Name:    "node",
						Version: "{{.version}}",
						URI:     "https://nodejs.org/dist/{{.version}}/node-{{.version}}-linux-x64.tar.xz",
					},
				},
			},
		},
	},
}

type nodeLayer struct{}

func (nodeLayer) Provide(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata, deps []packfile.Dep) error {
	dl, err := pf.NewDownloader(md, deps)
	if err != nil {
		return err
	}
	defer dl.Close()
	path, err := dl.GetFile("node", "")
	if err != nil {
		return err
	}

	cmd := exec.Command("tar", "-C", env["LAYER"], "-xJf", path, "--strip-components=1")
	cmd.Stdout = st.Stdout()
	cmd.Stderr = st.Stderr()
	return cmd.Run()
}

func (nodeLayer) Test(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata) error {
	version, err := md.Read("version")
	if xerrors.Is(err, metadata.ErrNotExist) {
		version = "*"
	} else if err != nil {
		return err
	}
	resp, err := http.Get("https://semver.io/node/resolve/" + version)
	if err != nil {
		return err
	}
	vbytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return md.Write("v"+string(vbytes), "version")
}

func (nodeLayer) Version() string {
	return BuildID
}

func main() {
	if err := pf.Run(buildpack); err != nil {
		log.Fatalf("Error: %s", err)
	}
}
