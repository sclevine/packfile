
### Separate

#### NPM
```toml
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

[layers.build]
inline = """
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules “$LAYER/"
"""

[[layers.build.links]]
name = "npm-cache"
path-as = "NPM_CACHE"
```

#### Node
```toml
[[layers]]
name = "nodejs" 
store = true

[layers.provide.test]
inline = """
version=$(cat "$MD/version")
url=https://semver.io/node/resolve/${version:-*}
echo v$(wget -q -O - "$url") > "$MD/version"
"""

# downloads are cleaned up after
# all file vars from detect are present + shortcut for version
# parameters are available
# sha is checked if specified
[[layers.provide.deps]]
name = "node"
version = "{{.version}}"
uri = "https://nodejs.org/dist/{{.version}}/node-{{.version}}-linux-x64.tar.xz"

[layers.provide]
inline = """
tar -C "$LAYER" -xJf "$(get-dep node)" --strip-components=1
"""
```

### Combined

#### Node.js
```toml
[[processes]]
type = "web"
command = "npm start"

[[layers]]
name = "nodejs"
export = true
store = true

[layers.require]
inline = """
jq -r .engines.node package.json > "$MD/version"
"""

[layers.provide.test]
inline = """
version=$(cat "$MD/version")
url=https://semver.io/node/resolve/${version:-*}
echo v$(wget -q -O - "$url") > "$MD/version"
"""

[[layers.provide.deps]]
name = "node"
version = "{{.version}}"
uri = "https://nodejs.org/dist/{{.version}}/node-{{.version}}-linux-x64.tar.xz"

[layers.provide]
inline = """
tar -C "$LAYER" -xJf "$(get-dep node)" --strip-components=1
"""

[[caches]]
name = "npm-cache"

[[layers]]
name = "modules"
export = true

[[layers.build.env.launch]]
name = "NODE_PATH"
value = "{{.Layer}}/node_modules"

[layers.build.test]
inline = """
md5sum package-lock.json | cut -d' ' -f1 > "$MD/version"
"""

[layers.build]
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
[[processes]]
type = "web"
command = "run"

[[layers]]
name = "server"
export = true

[layers.build]
inline = """
mkdir "$LAYER/bin"
echo 'while true; do cat index.tcp | nc -l 8080; done' > "$LAYER/bin/run"
chmod +x "$LAYER/bin/run"
"""
```

