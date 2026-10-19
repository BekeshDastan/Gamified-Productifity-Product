//go:build linux

package operations

import "syscall"

func diskSpace(directory string) (uint64, uint64, bool) {
	var stat syscall.Statfs_t
	if syscall.Statfs(directory, &stat) != nil {
		return 0, 0, false
	}
	return stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize), true
}
