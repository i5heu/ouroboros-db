package keyValStore

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/google/fscrypt/filesystem"
	"github.com/sirupsen/logrus"
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

// displayDiskUsage displays the disk usage information using structured logging
func displayDiskUsage(paths []string) error {
	log.Info("Displaying disk usage information for paths")

	for _, path := range paths {
		disk, err := getDiskUsageStats(path)
		if err != nil {
			log.WithFields(logrus.Fields{
				"path": path,
			}).Errorf("Error retrieving disk usage stats: %v", err)
			return err
		}

		device, mountPoint, err := getDeviceAndMountPoint(path)
		if err != nil {
			log.WithFields(logrus.Fields{
				"path": path,
			}).Errorf("Error finding device and mount point: %v", err)
			return err
		}

		totalSpace := float64(disk.Blocks*uint64(disk.Bsize)) / 1e9
		freeSpace := float64(disk.Bfree*uint64(disk.Bsize)) / 1e9
		usedSpace := totalSpace - freeSpace

		pathSize, err := calculateDirectorySize(path)
		if err != nil {
			log.WithFields(logrus.Fields{
				"path": path,
			}).Errorf("Error calculating directory size: %v", err)
			return err
		}
		pathUsage := float64(pathSize) / 1e9

		log.WithFields(logrus.Fields{
			"Path":        path,
			"Device":      device,
			"Mount Point": mountPoint,
			"Total (GB)":  fmt.Sprintf("%.2f", totalSpace),
			"Used (GB)":   fmt.Sprintf("%.2f", usedSpace),
			"Free (GB)":   fmt.Sprintf("%.2f", freeSpace),
			"Usage by DB": fmt.Sprintf("%.2f", pathUsage),
		}).Info("Disk Usage")
	}

	return nil
}
