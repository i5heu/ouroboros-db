package dashboard

import (
	"sync"
	"testing"
)

func TestUploadFlowTracker_Create(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()
	if flow == nil {
		t.Fatal("Create returned nil")
	}

	if flow.ID == "" {
		t.Error("Flow ID should not be empty")
	}

	if flow.Status != "started" {
		t.Errorf("Expected status 'started', got '%s'", flow.Status)
	}

	if flow.StartedAt == 0 {
		t.Error("StartedAt should be set")
	}

	if len(flow.Stages) != 5 {
		t.Errorf("Expected 5 stages, got %d", len(flow.Stages))
	}
}

func TestUploadFlowTracker_Get(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	retrieved, ok := tracker.Get(flow.ID)
	if !ok {
		t.Fatal("Get returned false for existing flow")
	}

	if retrieved.ID != flow.ID {
		t.Errorf("Retrieved flow ID mismatch")
	}

	// Non-existent flow
	_, ok = tracker.Get("nonexistent")
	if ok {
		t.Error("Get should return false for non-existent flow")
	}
}

func TestUploadFlowTracker_UpdateStatus(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	// Update to encrypting
	tracker.UpdateStatus(flow.ID, "encrypting")

	retrieved, _ := tracker.Get(flow.ID)
	if retrieved.Status != "encrypting" {
		t.Errorf("Expected status 'encrypting', got '%s'", retrieved.Status)
	}

	// Check stage status
	if retrieved.Stages[0].Status != "in_progress" {
		t.Errorf("First stage should be in_progress")
	}

	// Update to wal
	tracker.UpdateStatus(flow.ID, "wal")

	retrieved, _ = tracker.Get(flow.ID)
	if retrieved.Stages[0].Status != "complete" {
		t.Errorf("First stage should be complete")
	}
	if retrieved.Stages[1].Status != "in_progress" {
		t.Errorf("Second stage should be in_progress")
	}
}

func TestUploadFlowTracker_UpdateStatus_Complete(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	tracker.UpdateStatus(flow.ID, "complete")

	retrieved, _ := tracker.Get(flow.ID)
	if retrieved.Status != "complete" {
		t.Errorf("Expected status 'complete', got '%s'", retrieved.Status)
	}

	if retrieved.CompletedAt == 0 {
		t.Error("CompletedAt should be set")
	}

	// All stages should be complete
	for i, stage := range retrieved.Stages {
		if stage.Status != "complete" {
			t.Errorf("Stage %d should be complete", i)
		}
	}
}

func TestUploadFlowTracker_SetVertexHash(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	tracker.SetVertexHash(flow.ID, "vertex-abc123")

	retrieved, _ := tracker.Get(flow.ID)
	if retrieved.VertexHash != "vertex-abc123" {
		t.Errorf("Expected VertexHash 'vertex-abc123', got '%s'",
			retrieved.VertexHash)
	}
}

func TestUploadFlowTracker_SetBlockHash(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	tracker.SetBlockHash(flow.ID, "block-xyz789")

	retrieved, _ := tracker.Get(flow.ID)
	if retrieved.BlockHash != "block-xyz789" {
		t.Errorf("Expected BlockHash 'block-xyz789', got '%s'",
			retrieved.BlockHash)
	}
}

func TestUploadFlowTracker_SliceDistribution(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	// Add slice distributions
	tracker.AddSliceDistribution(flow.ID, SliceDistribution{
		SliceHash:  "slice-1",
		SliceIndex: 0,
		TargetNode: "node-A",
		Status:     "pending",
	})
	tracker.AddSliceDistribution(flow.ID, SliceDistribution{
		SliceHash:  "slice-2",
		SliceIndex: 1,
		TargetNode: "node-B",
		Status:     "pending",
	})

	retrieved, _ := tracker.Get(flow.ID)
	if len(retrieved.SliceDistribution) != 2 {
		t.Fatalf("Expected 2 slice distributions, got %d",
			len(retrieved.SliceDistribution))
	}

	// Update slice status
	tracker.UpdateSliceStatus(flow.ID, "slice-1", "confirmed")

	retrieved, _ = tracker.Get(flow.ID)
	if retrieved.SliceDistribution[0].Status != "confirmed" {
		t.Errorf("Expected slice-1 status 'confirmed', got '%s'",
			retrieved.SliceDistribution[0].Status)
	}
	if retrieved.SliceDistribution[1].Status != "pending" {
		t.Errorf("Expected slice-2 status 'pending', got '%s'",
			retrieved.SliceDistribution[1].Status)
	}
}

func TestUploadFlowTracker_SetError(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	tracker.SetError(flow.ID, "something went wrong")

	retrieved, _ := tracker.Get(flow.ID)
	if retrieved.Status != "failed" {
		t.Errorf("Expected status 'failed', got '%s'", retrieved.Status)
	}
	if retrieved.Error != "something went wrong" {
		t.Errorf("Expected error message, got '%s'", retrieved.Error)
	}
}

func TestUploadFlowTracker_Delete(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	flow := tracker.Create()

	tracker.Delete(flow.ID)

	_, ok := tracker.Get(flow.ID)
	if ok {
		t.Error("Flow should not exist after delete")
	}
}

func TestUploadFlowTracker_ConcurrentAccess(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	var wg sync.WaitGroup

	// Create multiple flows concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flow := tracker.Create()
			tracker.UpdateStatus(flow.ID, "encrypting")
			tracker.SetVertexHash(flow.ID, "hash-"+flow.ID)
			tracker.AddSliceDistribution(flow.ID, SliceDistribution{
				SliceHash: "slice",
				Status:    "pending",
			})
		}()
	}

	wg.Wait()
	// Should not panic or race
}

func TestUploadFlowTracker_UpdateNonExistent(t *testing.T) { // A
	tracker := NewUploadFlowTracker()

	// These should not panic
	tracker.UpdateStatus("nonexistent", "encrypting")
	tracker.SetVertexHash("nonexistent", "hash")
	tracker.SetBlockHash("nonexistent", "hash")
	tracker.AddSliceDistribution("nonexistent", SliceDistribution{})
	tracker.UpdateSliceStatus("nonexistent", "slice", "confirmed")
	tracker.SetError("nonexistent", "error")
	tracker.Delete("nonexistent")
}

func TestGenerateFlowID(t *testing.T) { // A
	ids := make(map[string]bool)

	// Generate multiple IDs and check uniqueness
	for i := 0; i < 100; i++ {
		id := generateFlowID()
		if id == "" {
			t.Error("Generated empty ID")
		}
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestSliceDistribution_JSON(t *testing.T) { // A
	dist := SliceDistribution{
		SliceHash:  "hash123",
		SliceIndex: 3,
		TargetNode: "node-X",
		Status:     "confirmed",
	}

	if dist.SliceIndex != 3 {
		t.Error("SliceIndex should be 3")
	}
}

func TestUploadFlowStage_Progression(t *testing.T) { // A
	tracker := NewUploadFlowTracker()
	flow := tracker.Create()

	stages := []string{"encrypting", "wal", "sealing", "distributing", "complete"}

	for _, stage := range stages {
		tracker.UpdateStatus(flow.ID, stage)
		retrieved, _ := tracker.Get(flow.ID)
		if retrieved.Status != stage {
			t.Errorf("Expected status '%s', got '%s'", stage, retrieved.Status)
		}
	}
}
