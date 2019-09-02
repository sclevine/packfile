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

use + cache = always comes back, rebuilds w/cache on version mismatch, require does not change behavior
use + no-cache = never comes back, is not created if version matches, require forces creation
expose + cache =  always comes back, rebuilds w/ cache on version mismatch, require does not change behavior
expose + no-cache = never comes back, always rebuilt, require does not change behavior
use + expose + no-cache = never comes back, always rebuilt, require does not change behavior
use + expose + cache = always comes back, rebuilds w/ cache on version mismatch, require does not change behavior

Idea:

cached layers are always re-built from scratch
cache is only restored if versions match
cache can be available inter-build if required + no version 

Another idea:

if required layers change, dependent layers must be rebuild?

Another idea:

if required layer versions change, dependent layers must be rebuilt?

Completely different idea:

Add another step before build for setting version?

## New Version

env always loaded (clear-env = false)

require gets: APP, MD (wd: APP)
provide.test gets: APP, MD (wd: APP)
provide.build gets: APP, LAYER, MD (wd: LAYER)

each step may override version & metadata

layers automatically required/provided, when they have relevant section

layers can have directly specified metadata on them in TOML, but code blocks override

later metadata wins when there are duplicate requires (no merge except cache/launch)

a layer with no provide or provide.test has code 100 does not create an actual layer, but may require it
launch/cache flags on these layers override previous actual layer definitions

a layer with no require or require has code 100 does not require a layer, but may provide it

a missing version is always considered a mismatch

$MD accessible during require/provide.test/provide.build

eventually use two layers for cache+launch?

get-dep version defaults to layer version

any layer with a provide can be referenced with "with"

a layer with no provide or require can be referenced with "with"

image + cache = always comes back, rebuilds w/cache on version mismatch, require does not change behavior
image + no-cache = never comes back, is not created if version matches, require forces creation
expose + cache =  always comes back, rebuilds w/ cache on version mismatch, require does not change behavior
expose + no-cache = never comes back, always rebuilt, require does not change behavior
image + expose + no-cache = never comes back, always rebuilt, require does not change behavior
image + expose + cache = always comes back, rebuilds w/ cache on version mismatch, require does not change behavior
