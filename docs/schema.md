```toml

[config]
id = "<id for compilation>"
version = "<version for compilation>"
name = "<name for compilation>"
shell = "/usr/bin/env bash"
serial = false

[[processes]]
name = "<command name>"
command = "<command value>"
args = ["command arg"]
direct = false

[[layers]]
name = "<layer name>"
expose = false
export = false
recover = false
version = "<default version>"

[layers.metadata]
# default values

[layers.require]
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"
func = "<go code>"

[[layers.provide.links]]
name = "<layer name reference>"
write = false
cache = false
path-as = "<env var name for path>"
version-as = "<env var name for version>"
metadata-as = "<env var name for metadata path>"

[layers.provide.test]
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"
func = "<go code>"

# all deps fields can be go-templated
[[layers.provide.deps]]
name = "<dep name>"
version = "<dep version>"
uri = "<dep uri>"

[layers.provide]
write-app = false
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"
func = "<go code>"

[[layers.provide.env]]
name = "<name>"
value = "<value>"

[[layers.build]]
# same as [[layers.provide]]

[[layers.launch.profile]]
inline = "<script>"
path = "<path to script>"

[[layers.launch.env]]
name = "<name>"
value = "<value>"
```