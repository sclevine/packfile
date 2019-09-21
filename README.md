# packfile

Reproducible, unprivileged OCI image builds in TOML.

Parallel, offline, and metadata-rich.

Built on top of [Cloud Native Buildpacks](https://buildpacks.io).

## Random Notes

- PROBLEM: default metadata for provide-only layers is never propagated, but double-propagating would mess up require+provide layers
- SOLUTION: propagate directly to build, but only when layer is provide-only?

env always loaded (clear-env = false)

- require gets: APP (ro), MD (rw) (wd: APP)
- provide.test gets: APP (rw), MD (rw) (wd: APP) | Link: MD_AS (ro), PATH_AS (ro) | Cache: PATH_AS (rw)
- provide gets: APP (rw), LAYER (rw), MD (rw) (wd: APP) | Link: MD_AS (ro), PATH_AS (ro) | Cache: PATH_AS (rw)

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

provide.test can be used to create custom inter-dependent layer rebuilding

provide.test is never skipped

write-app layers are always re-built and run serially

PROBLEM: some layers need other layers during provide.test, but it forces unnecessary rebuilds in export chains
SOLUTION: flag to add layer path to provide.test

PROBLEM: easier to send BOM metadata to linked layers, but not always available when layer is not regenerated
SOLUTION: replace metadata with saved metadata

PROBLEM: deadlock occurs when layer must lookup whether layers that link to it need to be rebuilt
PROBLEM: future layer lookup doesn't work for indirect rebuilds a -> b -> c

[drop] metadata changes during provide are only accessible in BOM, not in linked layers (unless for-test?)

- export + store = always comes back, rebuilds w/o cache on version mismatch, link does not change behavior
- export = never comes back, is not created if version matches, link can force creation
- expose + store =  always comes back, rebuilds w/o cache on version mismatch, link does not change behavior
- expose = never comes back, always rebuilt, link does not change behavior
- export + expose + store = always comes back, rebuilds w/o cache on version mismatch, link does not change behavior
- export + expose = never comes back, always rebuilt, link does not change behavior

- the version from provide.test is always matched against the version from provide.test
