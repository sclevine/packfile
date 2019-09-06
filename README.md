# packfile

Reproducible, unprivileged OCI image builds in TOML.

Parallel, offline, and metadata-rich.

Built on top of Cloud Native Buildpacks.

## Random Notes

env always loaded (clear-env = false)

require gets: APP, MD (wd: APP)
provide.test gets: APP, MD (wd: APP)
provide gets: APP, LAYER, MD (wd: APP)

build is the same as provide but implies require

each step may override version & metadata

layers automatically required/provided, when they have relevant section

layers can have directly specified metadata on them in TOML, but code blocks override

later metadata wins when there are duplicate requires (no merge except cache/image)

a layer with no provide or provide.test has code 100 does not create an actual layer, but may require it
image/cache flags on these layers override previous actual layer definitions

a layer with no require or require has code 100 does not require a layer, but may provide it

a missing version is always considered a mismatch

$MD accessible during require/provide.test/provide

eventually use two layers for cache+image?

get-dep version defaults to layer version

any layer with a provide can be referenced with "use"

a layer with no provide or require can be referenced with "use"

provide.test can be used to create inter-dependent layer rebuilding

image + cache = always comes back, rebuilds w/cache on version mismatch, require does not change behavior
image + no-cache = never comes back, is not created if version matches, require forces creation
expose + cache =  always comes back, rebuilds w/ cache on version mismatch, require does not change behavior
expose + no-cache = never comes back, always rebuilt, require does not change behavior
image + expose + no-cache = never comes back, always rebuilt, require does not change behavior
image + expose + cache = always comes back, rebuilds w/ cache on version mismatch, require does not change behavior
