// Package backup defines interfaces for backup operations in OuroborosDB.
package backup

import (
	"context"
	"io"
)

// BackupManager handles backup and restore operations for a node.
type BackupManager interface {
	// BackupData creates a backup of all local data.
	BackupData(ctx context.Context, writer io.Writer) error

	// RestoreData restores data from a backup.
	RestoreData(ctx context.Context, reader io.Reader) error

	// GetBackupStatus returns the current backup status.
	GetBackupStatus(ctx context.Context) (BackupStatus, error)

	// ScheduleBackup schedules a recurring backup.
	ScheduleBackup(ctx context.Context, schedule BackupSchedule) error

	// CancelScheduledBackup cancels a scheduled backup.
	CancelScheduledBackup(ctx context.Context) error
}

// BackupStatus represents the status of backup operations.
type BackupStatus struct {
	// LastBackup is the Unix timestamp of the last successful backup.
	LastBackup int64

	// LastBackupSize is the size of the last backup in bytes.
	LastBackupSize int64

	// BackupInProgress indicates if a backup is currently running.
	BackupInProgress bool

	// Progress is the percentage complete (0-100) if a backup is in progress.
	Progress float64

	// ScheduledBackupEnabled indicates if scheduled backups are enabled.
	ScheduledBackupEnabled bool

	// NextScheduledBackup is the Unix timestamp of the next scheduled backup.
	NextScheduledBackup int64
}

// BackupSchedule defines when backups should occur.
type BackupSchedule struct {
	// Enabled indicates if scheduled backups are enabled.
	Enabled bool

	// IntervalHours is the interval between backups in hours.
	IntervalHours int

	// RetainCount is the number of backups to retain.
	RetainCount int

	// BackupPath is the path where backups should be stored.
	BackupPath string
}
