
### Separate

NPM:

```toml
[[processes]]
name = "web"
command = "npm start"

[[layer]]
name = "nodejs"
expose = true
use = true

[layer.detect]
inline = """
jq -r .engines.node package.json > "$MD/version"
"""

[[layer]]
name = "npm-cache"
cache = true

[[layer]]
name = "modules"
use = true

[layer.detect]
inline = """
sha=$(md5sum package-lock.json | cut -d' ' -f1)
echo "$sha-${NODE_VERSION}" > "$MD/version"
"""

[[layer.detect.require]]
name = "nodejs"
version-as = "NODE_VERSION"

[layer.build]
write-app = true
inline = """
cd "$APP"
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules “$LAYER/"
ln -snf “$LAYER/node_modules” node_modules
"""

[[layer.build.require]] # special case: no-build + no-detect = build-only layer, no plan entry
name = "npm-cache"
write = true
path-as = "NPM_CACHE"
```

Node.js:
```toml
[[layer]]
name = "nodejs" 
cache = true

# downloads are cleaned up after
# all file vars from detect are present + shortcut for version
# parameters are available
# sha is checked if specified
[[layer.build.deps]]
name = "node"
version = "{{version}}"
uri = "https://nodejs.org/dist/{{version}}/node-{{version}}-linux-x64.tar.xz"


# VERSION and VERSIONS also accessible
# get-dep 
[layer.build]
inline = """
version=$(cat "$MD/version")
url=https://semver.io/node/resolve/${version:-*}
echo v$(wget -q -O - "$url") > "$MD/version"
tar -xJf "$(get-dep node)" --strip-components=1
"""
```

### Combined

```toml
[[processes]]
name = "web"
command = "npm start"

[[layer]]
name = "nodejs"
use = true
cache = true

[layer.detect]
inline = """
semver=$(jq -r .engines.node package.json)
url=https://semver.io/node/resolve/${semver:-*}
echo v$(wget -q -O - "$url") > "$MD/version"
"""

[[layer.build.deps]]
name = "node"
version = "{{version}}"
uri = "https://nodejs.org/dist/{{version}}/node-{{version}}-linux-x64.tar.xz"

[layer.build]
inline = """
tar -xJf "$(get-dep node)" --strip-components=1
"""

[[layer]]
name = "npm-cache"
cache = true

[[layer]]
name = "modules"
use = true

[layer.detect]
inline = """
sha=$(md5sum package-lock.json | cut -d' ' -f1)
echo "$sha-${NODE_VERSION}" > "$MD/version"
"""

[[layer.detect.require]]
name = "nodejs"
version-as = "NODE_VERSION"

[layer.build]
write-app = true
inline = """
cd "$APP"
npm ci --unsafe-perm --cache "$NPM_CACHE"
mv node_modules “$LAYER/"
ln -snf “$LAYER/node_modules” node_modules
"""

[[layer.build.require]]
name = "nodejs"

[[layer.build.require]]
name = "npm-cache"
write = true
path-as = "NPM_CACHE"
```