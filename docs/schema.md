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

[[layer]]
cache = false
expose = false
use = false
version = "<default version>"

[layer.metadata]
# default values

# all field values below can be templated
[[layer.build.deps]]
name = "<dep name>"
version = "<dep version>"
uri = "<dep uri>"

[layer.build]
write-app = false
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"
func = "<go code>"

[[layer.build.require]]
name = "<layer name reference>"
write = false
path-as = "<env var name for path>"
version-as = "<env var name for version>"
metadata-as = "<env var name for metadata path?>"

[[layer.build.env]]
name = "<name>"
value = "<value>"

[[layer.env]]
name = "<name>"
value = "<value>"

[layer.detect]
shell = "/usr/bin/env bash"
inline = "<script>"
path = "<path to script>"
func = "<go code>"

[[layer.detect.require]]
name = "<layer name reference>"
version-as = "<env var name for version>"
metadata-as = "<env var name for metadata path?>"

[[layer.launch.profile]]
inline = "<script>"
path = "<path to script>"

[[layer.launch.env]]
name = "<name>"
value = "<value>"

```