package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/cnb"
	"github.com/sclevine/packfile/deps"
	"github.com/sclevine/packfile/metadata"
)

func main() {
	if len(os.Args) == 0 {
		log.Fatal("Error: command name missing")
	}
	command := os.Args[0]
	switch filepath.Base(command) {
	case "detect":
		if len(os.Args) != 3 {
			log.Fatal("Error: detect requires two arguments")
		}
		pf, ctxDir := findPackfile(command)
		if err := cnb.Detect(&pf, ctxDir, os.Args[1], os.Args[2]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	case "build":
		if len(os.Args) != 4 {
			log.Fatal("Error: build requires three arguments")
		}
		pf, ctxDir := findPackfile(command)
		dotBinDir := filepath.Join(filepath.Dir(filepath.Dir(command)), ".bin")
		pathEnv := dotBinDir + string(os.PathListSeparator) + os.Getenv("PATH")
		if err := os.Setenv("PATH", pathEnv); err != nil {
			log.Fatalf("Error: %s", err)
		}
		if err := cnb.Build(&pf, ctxDir, os.Args[1], os.Args[2], os.Args[3]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	case "get-dep":
		if n := len(os.Args); n != 2 && n != 3 {
			log.Fatal("Usage: get-dep <name> [<version>]")
		}
		name := os.Args[1]
		var version string
		if len(os.Args) == 3 {
			version = os.Args[2]
		}
		var config packfile.ConfigTOML
		if _, err := toml.DecodeFile(os.Getenv("PF_CONFIG_PATH"), &config); err != nil {
			log.Fatalf("Error: %s", err)
		}
		client := deps.Client{
			ContextDir: config.ContextDir,
			StoreDir:   config.StoreDir,
			Metadata:   metadata.NewFS(config.MetadataDir),
			Deps:       config.Deps,
		}
		path, err := client.GetFile(name, version)
		if err != nil {
			log.Fatalf("Error: %s", err)
		}
		fmt.Println(path)
	default:
		var in, out, pf string
		flag.StringVar(&in, "i", "", "input path to directory")
		flag.StringVar(&out, "o", "", "output path to buildpack tgz")
		flag.StringVar(&pf, "p", "", "path to pf binary")
		flag.Parse()
		if out == "" {
			flag.Usage()
			log.Fatal("Error: -o must be specified")
		}
		if err := writeBuildpack(out, in, pf); err != nil {
			log.Fatalf("Error: %s", err)
		}
	}
}

func findPackfile(command string) (pf packfile.Packfile, dir string) {
	dir = filepath.Dir(filepath.Dir(command))
	if _, err := os.Stat(filepath.Join(dir, "packfile.toml")); os.IsNotExist(err) {
		dir = "."
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "packfile.toml"), &pf); err != nil {
		log.Fatalf("Error: %s", err)
	}
	return pf, dir
}

type buildpackTOML struct {
	API       string        `toml:"api"`
	Buildpack buildpackInfo `toml:"buildpack"`
	Stacks    []packfile.Stack   `toml:"stacks"`
}

type buildpackInfo struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	Name    string `toml:"name"`
}

var packfileBuildpack = buildpackTOML{
	API: "0.2",
	Buildpack: buildpackInfo{
		ID:      "sh.scl.packfile",
		Version: "0.0.1",
		Name:    "Packfile Buildpack",
	},
	Stacks: []packfile.Stack{
		{ID: "io.buildpacks.stacks.bionic"},
		{ID: "org.cloudfoundry.stacks.cflinuxfs3"},
		{ID: "org.cloudfoundry.stacks.tiny"},
	},
}

func writeBuildpack(dst, src, path string) error {
	tempDir, err := ioutil.TempDir("", "packfile")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	bpTOML := packfileBuildpack
	var pf packfile.Packfile
	if _, err := toml.DecodeFile(filepath.Join(src, "packfile.toml"), &pf); err == nil {
		bpTOML = buildpackTOML{
			API: "0.2",
			Buildpack: buildpackInfo{
				ID:      pf.Config.ID,
				Version: pf.Config.Version,
				Name:    pf.Config.Name,
			},
			Stacks: pf.Stacks,
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := writeTOML(filepath.Join(tempDir, "buildpack.toml"), bpTOML); err != nil {
		return err
	}
	if path != "" {
		if err := copyFile(filepath.Join(tempDir, "pf"), path); err != nil {
			return err
		}
	} else {
		pf, err := getLinuxPF()
		if err != nil {
			return err
		}
		defer pf.Close()
		if err := writeFile(filepath.Join(tempDir, "pf"), pf, 0777); err != nil {
			return err
		}
	}

	binDir := filepath.Join(tempDir, "bin")
	if err := os.Mkdir(binDir, 0777); err != nil {
		return err
	}
	dotBinDir := filepath.Join(tempDir, ".bin")
	if err := os.Mkdir(dotBinDir, 0777); err != nil {
		return err
	}
	pfLink := filepath.Join("..", "pf")
	if err := os.Symlink(pfLink, filepath.Join(binDir, "build")); err != nil {
		return err
	}
	if err := os.Symlink(pfLink, filepath.Join(binDir, "detect")); err != nil {
		return err
	}
	if err := os.Symlink(pfLink, filepath.Join(dotBinDir, "get-dep")); err != nil {
		return err
	}
	args := []string{"-czf", dst}
	if src != "" {
		args = append(args, "-C", src, ".")
	}
	args = append(args, "-C", tempDir, "./pf", "./bin", "./.bin", "./buildpack.toml")
	return exec.Command("tar", args...).Run()
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeFile(path string, in io.Reader, perm os.FileMode) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeTOML(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(v); err != nil {
		return err
	}
	return f.Close()
}
