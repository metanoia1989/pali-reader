# Sourced by build scripts. Keeps the Go module cache inside the workspace so
# the sandboxed toolchain can write to it, and routes module fetches through a
# mirror that is reachable from here.
#
# GOTOOLCHAIN=local: the module proxies here are slow/blocked for toolchain
# downloads, and the host Go (1.22) is what we target anyway. If a dependency
# ever demands a newer Go, bump the version explicitly instead of letting the
# toolchain switch happen implicitly.
export GOPATH="${PALI_GOPATH:-/Users/metanoia/WorkSpace/pali-reading/.gopath}"
export GOMODCACHE="$GOPATH/pkg/mod"
export GOCACHE="${PALI_GOCACHE:-/Users/metanoia/WorkSpace/pali-reading/.gocache}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct}"
export GOTOOLCHAIN=local
export GOFLAGS=-mod=mod
