## Examples

### Separate

#### NPM
```toml
[config]
id = "sh.scl.npm"
version = "0.0.0"
name = "NPM Packfile"

[[processes]]
type = "web"
command = "npm start"

[[caches]]
name = "npm-cache"

[[layers]]
name = "nodejs"
expose = true
export = true

[layers.require]
inline = """
jq -r .engines.node package.json > "$MD/version"
"""

[[layers]]
name = "modules"
export = true

[[layers.build.env.launch]]
name = "NODE_PATH"
value = "{{.Layer}}/node_modules"

[layers.build.test]
inline = """
sha=$(md5sum package-lock.json | cut -d' ' -f1)
echo "$sha-$(node -v)" > "$MD/version"
"""

[layers.build.run]
inline = """
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules "$LAYER/"
"""

[[layers.build.links]]
name = "npm-cache"
path-as = "NPM_CACHE"
```

#### Node
```toml
[config]
id = "sh.scl.node-engine"
version = "0.0.0"
name = "Node Engine Packfile"

[[layers]]
name = "nodejs"
store = true

[layers.provide.test]
inline = """
version=$(cat "$MD/version")
url=https://semver.io/node/resolve/${version:-*}
echo v$(curl -sL "$url") > "$MD/version"
"""

[[layers.provide.deps]]
name = "node"
version = "{{.version}}"
uri = "https://nodejs.org/dist/{{.version}}/node-{{.version}}-linux-x64.tar.xz"

[layers.provide.run]
inline = """
tar -C "$LAYER" -xJf "$(get-dep node)" --strip-components=1
"""
```

### Combined

#### Node.js
```toml
[config]
id = "sh.scl.nodejs"
version = "0.0.0"
name = "Node.js Packfile"

[[processes]]
type = "web"
command = "npm start"

[[caches]]
name = "npm-cache"

[[layers]]
name = "nodejs"
export = true
store = true

[layers.build.test]
inline = """
version=$(jq -r .engines.node package.json)
url=https://semver.io/node/resolve/${version:-*}
echo v$(curl -sL "$url") > "$MD/version"
"""

[[layers.build.deps]]
name = "node"
version = "{{.version}}"
uri = "https://nodejs.org/dist/{{.version}}/node-{{.version}}-linux-x64.tar.xz"

[layers.build.run]
inline = """
tar -C "$LAYER" -xJf "$(get-dep node)" --strip-components=1
"""

[[layers]]
name = "modules"
export = true

[[layers.build.env.both]]
name = "NODE_PATH"
value = "{{.Layer}}/node_modules"

[layers.build.test]
match = ["package-lock.json"]

[layers.build.run]
inline = """
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules "$LAYER/"
"""

[[layers.build.links]]
name = "nodejs"
link-version = true

[[layers.build.links]]
name = "npm-cache"
path-as = "NPM_CACHE"
```

#### Simple TCP Server
```toml
[config]
id = "sh.scl.tcp"
version = "0.0.0"
name = "TCP Packfile"

[[processes]]
type = "web"
command = "run"

[[layers]]
name = "server"
export = true

[layers.build.run]
inline = """
mkdir "$LAYER/bin"
echo 'while true; do cat index.tcp | nc -l 8080; done' > "$LAYER/bin/run"
chmod +x "$LAYER/bin/run"
"""
```

