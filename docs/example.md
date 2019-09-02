
### Separate

NPM:

```toml
[[processes]]
name = "web"
command = "npm start"

[[layers]]
name = "nodejs"
expose = true
image = true

[layers.require]
inline = """
jq -r .engines.node package.json > "$MD/version"
"""

[[layers]]
name = "npm-cache"
cache = true

[[layers]]
name = "modules"
image = true

[layers.require]

[layers.provide.test]
inline = """
sha=$(md5sum package-lock.json | cut -d' ' -f1)
echo "$sha-$(node -v)" > "$MD/version"
"""

[layers.provide]
write-app = true
inline = """
cd "$APP"
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules “$LAYER/"
ln -snf “$LAYER/node_modules” node_modules
"""

[[layers.provide.use]] # special case: no-build + no-detect = build-only layer, no plan entry
name = "npm-cache"
write = true
path-as = "NPM_CACHE"
```

Node.js:
```toml
[[layers]]
name = "nodejs" 
cache = true

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
tar -xJf "$(get-dep node)" --strip-components=1
"""
```

### Combined

```toml
[[processes]]
name = "web"
command = "npm start"

[[layers]]
name = "nodejs"
image = true
cache = true

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
tar -xJf "$(get-dep node)" --strip-components=1
"""

[[layers]]
name = "npm-cache"
cache = true

[[layers]]
name = "modules"
image = true

[layers.require]

[layers.provide.test]
inline = """
sha=$(md5sum package-lock.json | cut -d' ' -f1)
echo "$sha-${NODE_VERSION}" > "$MD/version"
"""

[layers.provide]
write-app = true
inline = """
cd "$APP"
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules “$LAYER/"
ln -snf “$LAYER/node_modules” node_modules
"""

[[layers.provide.use]]
name = "nodejs"
version-as = "NODE_VERSION"

[[layers.provide.use]]
name = "npm-cache"
write = true
path-as = "NPM_CACHE"
```