# packfile

Reproducible, unprivileged OCI image builds in TOML.

Parallel, offline, and metadata-rich.

Built on top of Cloud Native Buildpacks.

## Random Notes

- PROBLEM: default metadata for provide-only layers is never propagated, but double-propagating would mess up require+provide layers
- SOLUTION: propagate directly to build, but only when layer is provide-only?

env always loaded (clear-env = false)

- require gets: APP (ro), MD (rw) (wd: APP)
- provide.test gets: APP (rw), MD (rw) (wd: APP) | MD_AS (ro), PATH_AS (ro)
- provide gets: APP (rw), LAYER (rw), MD (rw) (wd: APP) | MD_AS (ro/rw), PATH_AS (ro/rw)

- provide-only: provide in build plan
- require-only: require in build plan
- require + provide: both require and provide in build plan
- build-only: both require and provide in build plan
- (no other combinations presented)

build is the same as provide but implies require

each step may override version & metadata

layers automatically required/provided, when they have relevant section

layers can have directly specified metadata on them in TOML, but code blocks override

later metadata wins when there are duplicate requires (no merge except expose/export)

a layer with no provide or provide.test has code 100 does not create an actual layer, but may require it.
export/expose flags on these layers override previous actual layer definitions

a layer with no require or require has code 100 does not require a layer, but may provide it

a missing version is always considered a mismatch

$MD accessible during require/provide.test/provide

eventually use two layers for cache/store+export?

get-dep version defaults to layer version

any layer with a provide can be referenced with "link"

a layer with no provide or require can be referenced with "link"

provide.test can be used to create inter-dependent layer rebuilding

write-app layers are always re-built and run serially

- export + store = always comes back, rebuilds w/o cache on version mismatch, link does not change behavior
- export + cache = always comes back, rebuilds w/ cache on version mismatch, link does not change behavior
- export + no-cache = never comes back, is not created if version matches, link forces creation
- expose + store =  always comes back, rebuilds w/o cache on version mismatch, link does not change behavior
- expose + cache =  always comes back, rebuilds w/ cache on version mismatch, link does not change behavior
- expose + no-cache = never comes back, always rebuilt, link does not change behavior
- export + expose + store = always comes back, rebuilds w/o cache on version mismatch, link does not change behavior
- export + expose + cache = always comes back, rebuilds w/ cache on version mismatch, link does not change behavior
- export + expose + no-cache = never comes back, always rebuilt, link does not change behavior

- when writing to a non-cache layer via a link, provide always re-runs if the linked layer was re-generated
- when writing to a cache layer via a link, provide may not re-run, even if the linked layer was re-generated

- the version from provide is always matched against the version from provide.test
- if other layers override the version, the new value is just used for the BOM

PROBLEM: double-write: if linked+write layer is restored, and current layer re-builds -> double writes