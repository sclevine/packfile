# packfile

Packfile is an abstraction for writing modular Cloud Native Buildpacks.
It enables you to efficiently build OCI (Docker) images using declarative TOML, YAML, or Go.

Features:
- Can be used to build modular [buildpacks](https://buildpacks.io).
- Intelligently determines what layers need to be rebuilt, and only rebuilds those layers.
- Builds OCI image layers in parallel.
- Builds OCI images that are fully reproducible.
- Builds OCI images with swappable base images (compatible with `pack rebase`, so no containers required).
- Adds detailed metadata about OCI image contents.

Built on top of [Cloud Native Buildpacks](https://buildpacks.io).

**NOTE: Packfile is currently an untested proof-of-concept.**

## Documentation
[See here.](./docs)

## Usage

The `pf` binary can be used:
- To convert a directory containing `packfile.toml` or  `packfile.yaml` into a buildpack (with `-i`).
- To create a buildpack that will run `packfile.toml` or `packfile.toml` in an app directory (without `-i`).
- To create a buildpack from a compiled packfile binary and asset directory (with both `-p` and `-i <asset-dir>`).
- To create a buildpack from a compiled packfile binary and metadata (with both `-p` and `-i <packfile>`).
- On Linux as a buildpack that runs `packfile.toml` or `packfile.yaml` (when symlinked to `bin/build` and `bin/detect`).

## Build

```bash
./bin/tools
./bin/build
```

Buildpacks:
- `out/pf.tgz` can be used to build `testdata/app`.
- `testout/node-toml.tgz` is a Node.js engine buildpack built from `testdata/node-toml`.
- `testout/node-yaml.tgz` is a Node.js engine buildpack built from `testdata/node-yaml`.
- `testout/node-go.tgz` is a Node.js engine buildpack built from `testdata/node-go`.
- `testout/npm-toml.tgz` is an NPM buildpack built from `testdata/npm-toml`.
- `testout/npm-yaml.tgz` is an NPM buildpack built from `testdata/npm-yaml`.
- `testout/npm-go.tgz` is an NPM buildpack built from `testdata/npm-go`.

## Test

```bash
./bin/test
```
