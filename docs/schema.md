## Schema

```toml
[config]
id = "<id for compilation>"
version = "<version for compilation>"
name = "<name for compilation>"
shell = "/usr/bin/env bash"

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

[layers.provide]
lock-app = false

[[layers.provide.links]]
name = "<layer/cache name reference>"
path-as = "<env var name for path>"
version-as = "<env var name for version>"
metadata-as = "<env var name for metadata path>"
link-content = false # always rebuild on change
link-version = false # rebuild on version change

[layers.provide.test]
full-env = false # provide links paths/layers for test + respect lock-app
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"
match = ["<file path glob>"] # uses recursive checksum of app dir files as version

# all deps fields can be go-templated with metadata
[[layers.provide.deps]]
name = "<dep name>"
version = "<dep version>"
uri = "<dep uri>"
sha = "<dep sha checksum>" # verified on download

[layers.provide.deps.metadata]
# additional metadata

[layers.provide.run]
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"

[[layers.provide.env.both]]
name = "<name>"
value = "<value>" # interpolated with .Layer and .App
op = "<operation>" # default: override
delim = "<delimiter>"

[[layers.provide.env.launch]]
# same as [[layers.provide.env.both]], just launch-time

[[layers.provide.env.build]]
# same as [[layers.provide.env.both]], just build-time

[[layers.provide.profile]]
inline = "<script>"
path = "<path to script>"

[[layers.build]]
# same as [[layers.provide]]

[[slices]]
paths = []
```