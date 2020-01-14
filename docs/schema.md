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
link-contents = false # always rebuild on change
link-version = false # rebuild on version change

[layers.provide.test]
write-app = false
use-links = false # provide the path/layer for test
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"

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

[[layers.provide.env.launch]]
name = "<name>"
value = "<value>"

[[layers.provide.env.build]]
name = "<name>"
value = "<value>"

[[layers.provide.profile]]
inline = "<script>"
path = "<path to script>"

[[layers.build]]
# same as [[layers.provide]]

[[slices]]
paths = []
```