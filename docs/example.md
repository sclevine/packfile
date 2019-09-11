
### Separate

#### NPM
```toml
[[processes]]
name = "web"
command = "npm start"

[[layers]]
name = "nodejs"
expose = true
export = true

[layers.require]
inline = """
jq -r .engines.node package.json > "$MD/version"
"""

# special case: no-provide + no-require = empty dir w/ no plan entry
[[layers]]
name = "npm-cache"
recover = true

[[layers]]
name = "modules"
export = true

[layers.build.test]
inline = """
sha=$(md5sum package-lock.json | cut -d' ' -f1)
echo "$sha-$(node -v)" > "$MD/version"
"""

[layers.build]
write-app = true
inline = """
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules “$LAYER/"
mkdir "$LAYER/env"
echo "$LAYER/node_modules" > "$LAYER/env/NODE_PATH"
"""

[[layers.build.links]]
name = "npm-cache"
cache = true
path-as = "NPM_CACHE"
```

#### Node
```toml
[[layers]]
name = "nodejs" 
recover = true

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
version = "{{version}}"
uri = "https://nodejs.org/dist/{{version}}/node-{{version}}-linux-x64.tar.xz"

[layers.provide]
inline = """
tar -C "$LAYER" -xJf "$(get-dep node)" --strip-components=1
"""
```

### Combined

#### Node.js
```toml
[[processes]]
name = "web"
command = "npm start"

[[layers]]
name = "nodejs"
export = true
recover = true

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
version = "{{version}}"
uri = "https://nodejs.org/dist/{{version}}/node-{{version}}-linux-x64.tar.xz"

[layers.provide]
inline = """
tar -C "$LAYER" -xJf "$(get-dep node)" --strip-components=1
"""

[[layers]]
name = "npm-cache"
recover = true

[[layers]]
name = "modules"
export = true

[layers.build.test]
inline = """
sha=$(md5sum package-lock.json | cut -d' ' -f1)
echo "$sha-${NODE_VERSION}" > "$MD/version"
"""

[layers.build]
write-app = true
inline = """
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules “$LAYER/"
mkdir "$LAYER/env"
echo "$LAYER/node_modules" > "$LAYER/env/NODE_PATH"
"""

[[layers.build.links]]
name = "nodejs"
version-as = "NODE_VERSION"

[[layers.build.links]]
name = "npm-cache"
cache = true
path-as = "NPM_CACHE"
```

#### Simple TCP Server
```toml
[[processes]]
name = "web"
command = "run"

[[layers]]
name = "server"
export = true

[layers.build]
inline = """
mkdir bin
echo 'while true; do cat index.tcp | nc -l 8080; done' > bin/run
chmod +x bin/run
"""
```

