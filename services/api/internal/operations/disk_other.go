//go:build !linux

package operations

func diskSpace(string) (uint64, uint64, bool) { return 0, 0, false }
