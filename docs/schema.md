```toml
[config]
id = "<id for compilation>"
version = "<version for compilation>"
name = "<name for compilation>"
shell = "/usr/bin/env bash"
serial = false

[[processes]]
type = "<command name>"
command = "<command value>"
args = ["command arg"]
direct = false

[[caches]]
name = "<cache name>"

[caches.setup]
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"

[[layers]]
name = "<layer name>"
expose = false
export = false
store = false
version = "<default version>"

[layers.metadata]
# default values

[layers.require]
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"

[[layers.provide.links]]
name = "<layer/cache name reference>"
path-as = "<env var name for path>"
version-as = "<env var name for version>"
metadata-as = "<env var name for metadata path>"
link-content = false # always rebuild on change
link-version = false # rebuild on version change

[layers.provide.test]
full-env = false # provide links paths/layers for test + respect write-app
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"

# all deps fields can be go-templated
[[layers.provide.deps]]
name = "<dep name>"
version = "<dep version>"
uri = "<dep uri>"
sha = "<dep sha checksum>"

[layers.provide.deps.metadata]
# additional metadata

[layers.provide]
write-app = false
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"

[[layers.provide.env.launch]]
name = "<name>"
value = "<value>" # interpolated with .Layer and .App
op = "<operation>" # default: override
delim = "<delimiter>"

[[layers.provide.env.build]]
# same as [[layers.provide.env.launch]]

[[layers.provide.profile]]
inline = "<script>"
path = "<path to script>"

[[layers.build]]
# same as [[layers.provide]]

[[slices]]
paths = []
```