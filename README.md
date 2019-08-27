# packfile

Reproducible, unprivileged OCI image builds in TOML. Parallel, offline, and metadata-rich. Built on top of Cloud Native Buildpacks.

## Random Notes

env always loaded (clear-env = false)

detect gets: APP, MD (wd: APP)
build gets: APP, LAYER, MD (wd: LAYER)

build may override version

layers automatically required/provided, when they have relevant section

layers can have directly specified metadata on them in TOML, detect overrides

later metadata wins when there are duplicate detects (no merge except cache/launch)

a layer with no build or build=100 does not provide or create an actual layer, may require it
launch/cache flags on these layers override previous actual layer definitions

a layer with no detect or detect=100 does not require a layer, but may provide it

version also trigger requirement for matching build section?

$MD accessible during detect/build

eventually use two layers for cache+launch?

get-dep version defaults to layer version

layer with no build or detect can be required, but just during build