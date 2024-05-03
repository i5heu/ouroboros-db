package keyValStore

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"text/tabwriter"

	"github.com/google/fscrypt/filesystem"
)

// getDiskUsageStats gets the disk usage statistics of the given path
func getDiskUsageStats(path string) (disk syscall.Statfs_t, err error) {
	err = syscall.Statfs(path, &disk)
	return
}

// calculateDirectorySize calculates the total size of files within a directory
func calculateDirectorySize(path string) (size int64, err error) {
	err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return
}

func getDeviceAndMountPoint(path string) (device, mountPoint string, err error) {
	// Finding the mount information for the given path

	mnt, err := filesystem.FindMount(path)
	if err != nil {
		return "", "", fmt.Errorf("unable to find mount for path %s: %v", path, err)
	}

	return mnt.Device, mnt.Path, nil
}

// displayDiskUsage displays the disk usage information in a table format
func displayDiskUsage(paths []string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "Path\tDevice\tMount Point\tTotal Space (GB)\tUsed Space (GB)\tFree Space (GB)\tUsage by DB (GB)")

	for _, path := range paths {
		disk, err := getDiskUsageStats(path)
		if err != nil {
			return err
		}

		device, mountPoint, err := getDeviceAndMountPoint(path)
		if err != nil {
			return err
		}

		totalSpace := float64(disk.Blocks*uint64(disk.Bsize)) / 1e9
		freeSpace := float64(disk.Bfree*uint64(disk.Bsize)) / 1e9
		usedSpace := totalSpace - freeSpace

		pathSize, err := calculateDirectorySize(path)
		if err != nil {
			return err
		}
		pathUsage := float64(pathSize) / 1e9

		fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%.2f\t%.2f\t%.2f\n", path, device, mountPoint, totalSpace, usedSpace, freeSpace, pathUsage)
	}

	return nil
}
