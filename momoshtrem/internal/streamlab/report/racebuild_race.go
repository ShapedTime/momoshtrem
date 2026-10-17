//go:build race

package report

// raceBuild is true in a binary built with -race: the race build tag is
// set by the toolchain itself, so it holds for go build, go run and test
// binaries alike, whatever their build settings record.
const raceBuild = true
