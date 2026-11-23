package registry

// DefaultRegistry is the container registry namespace used for
// arrstart-owned images (the wizard/bootstrap image).
//
// It is kept in a single place so that a namespace change only requires
// editing this constant (ADR-0010: Registry, image name, and namespace).
//
// The install one-liner references ${DefaultRegistry}:<version> together
// with a versioned release asset URL. Tags are pinned to repo versions
// and are never "latest" in the one-liner.
const DefaultRegistry = "ghcr.io/bruceowenga/arrstart"
