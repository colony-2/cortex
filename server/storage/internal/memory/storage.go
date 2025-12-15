package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/colony-2/colony2/server/core/pkg/core"
)

// Storage implements core.Storage using in-memory maps
type Storage struct {
	mu           sync.RWMutex
	positions    map[string]core.Position
	containerIDs map[string]string
}

// New creates a new in-memory storage implementation
func New() core.Storage {
	return &Storage{
		positions:    make(map[string]core.Position),
		containerIDs: make(map[string]string),
	}
}

// SavePosition saves a position in memory
func (s *Storage) SavePosition(ctx context.Context, pos core.Position) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.positions[pos.CellID] = pos
	return nil
}

// GetPositions retrieves all positions from memory
func (s *Storage) GetPositions(ctx context.Context) ([]core.Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	positions := make([]core.Position, 0, len(s.positions))
	for _, pos := range s.positions {
		positions = append(positions, pos)
	}

	return positions, nil
}

// DeletePosition removes a position from memory
func (s *Storage) DeletePosition(ctx context.Context, cellID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.positions, cellID)
	return nil
}

// SaveContainerID saves a container ID in memory
func (s *Storage) SaveContainerID(ctx context.Context, cellID, containerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.containerIDs[cellID] = containerID
	return nil
}

// GetContainerID retrieves a container ID from memory
func (s *Storage) GetContainerID(ctx context.Context, cellID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	containerID, exists := s.containerIDs[cellID]
	if !exists {
		return "", fmt.Errorf("container ID not found for cell %s", cellID)
	}

	return containerID, nil
}

// DeleteContainerID removes a container ID from memory
func (s *Storage) DeleteContainerID(ctx context.Context, cellID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.containerIDs, cellID)
	return nil
}

// Close is a no-op for in-memory storage
func (s *Storage) Close() error {
	return nil
}
