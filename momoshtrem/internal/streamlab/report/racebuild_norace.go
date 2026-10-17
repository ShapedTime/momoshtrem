//go:build !race

package report

// raceBuild is false in a binary built without -race (racebuild_race.go).
const raceBuild = false
