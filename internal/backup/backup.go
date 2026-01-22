// Package backup provides backup implementations for OuroborosDB.
package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/i5heu/ouroboros-db/pkg/backup"
)

// DefaultBackupManager implements the BackupManager interface.
type DefaultBackupManager struct {
	status   backup.BackupStatus
	schedule backup.BackupSchedule
}

// NewBackupManager creates a new DefaultBackupManager instance.
func NewBackupManager() *DefaultBackupManager {
	return &DefaultBackupManager{}
}

// BackupData creates a backup of all local data.
func (m *DefaultBackupManager) BackupData(
	ctx context.Context,
	writer io.Writer,
) error {
	m.status.BackupInProgress = true
	m.status.Progress = 0

	defer func() {
		m.status.BackupInProgress = false
	}()

	// Implementation will write all data to the writer
	// This is a placeholder for the actual implementation

	m.status.Progress = 100

	return nil
}

// RestoreData restores data from a backup.
func (m *DefaultBackupManager) RestoreData(
	ctx context.Context,
	reader io.Reader,
) error {
	// Implementation will read and restore all data from the reader
	// This is a placeholder for the actual implementation
	return fmt.Errorf("backup: restore not yet implemented")
}

// GetBackupStatus returns the current backup status.
func (m *DefaultBackupManager) GetBackupStatus(
	ctx context.Context,
) (backup.BackupStatus, error) {
	return m.status, nil
}

// ScheduleBackup schedules a recurring backup.
func (m *DefaultBackupManager) ScheduleBackup(
	ctx context.Context,
	schedule backup.BackupSchedule,
) error {
	m.schedule = schedule
	m.status.ScheduledBackupEnabled = schedule.Enabled
	return nil
}

// CancelScheduledBackup cancels a scheduled backup.
func (m *DefaultBackupManager) CancelScheduledBackup(
	ctx context.Context,
) error {
	m.schedule.Enabled = false
	m.status.ScheduledBackupEnabled = false
	return nil
}

// Ensure DefaultBackupManager implements the BackupManager interface.
var _ backup.BackupManager = (*DefaultBackupManager)(nil)
