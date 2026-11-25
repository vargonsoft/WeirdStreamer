package media

// folderDevice is unknown on Windows (no per-disk load readings there either): jobs judge by the
// busiest disk, as for a file whose disk can't be told.
func folderDevice(dir string) (uint64, bool) { return 0, false }

// wholeDiskOf: Windows has no /sys/dev/block.
func wholeDiskOf(dev uint64) string { return "" }
