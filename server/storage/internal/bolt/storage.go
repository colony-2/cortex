package bolt

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/colony-2/colony2/server/core/pkg/core"
)

const (
	positionsBucket   = "positions"
	containerIDBucket = "container_ids"
)

// Storage implements core.Storage using BoltDB
type Storage struct {
	db       *bolt.DB
	mu       sync.RWMutex
	readOnly bool
}

// New creates a new BoltDB storage implementation
func New(dbPath string, readOnly bool) (core.Storage, error) {
	dbFile := filepath.Join(dbPath, ".colony2.db")

	db, err := bolt.Open(dbFile, 0600, &bolt.Options{
		ReadOnly: readOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &Storage{
		db:       db,
		readOnly: readOnly,
	}

	// Initialize buckets if not read-only
	if !readOnly {
		err = db.Update(func(tx *bolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists([]byte(positionsBucket))
			if err != nil {
				return err
			}
			_, err = tx.CreateBucketIfNotExists([]byte(containerIDBucket))
			return err
		})
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create buckets: %w", err)
		}
	}

	return s, nil
}

// SavePosition saves a position to the database
func (s *Storage) SavePosition(ctx context.Context, pos core.Position) error {
	if s.readOnly {
		return fmt.Errorf("storage is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(positionsBucket))
		if b == nil {
			return fmt.Errorf("positions bucket not found")
		}

		data, err := json.Marshal(pos)
		if err != nil {
			return fmt.Errorf("failed to marshal position: %w", err)
		}

		return b.Put([]byte(pos.CellID), data)
	})
}

// GetPositions retrieves all positions from the database
func (s *Storage) GetPositions(ctx context.Context) ([]core.Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var positions []core.Position

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(positionsBucket))
		if b == nil {
			return nil // No positions yet
		}

		return b.ForEach(func(k, v []byte) error {
			var pos core.Position
			if err := json.Unmarshal(v, &pos); err != nil {
				return fmt.Errorf("failed to unmarshal position: %w", err)
			}
			positions = append(positions, pos)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return positions, nil
}

// DeletePosition removes a position from the database
func (s *Storage) DeletePosition(ctx context.Context, cellID string) error {
	if s.readOnly {
		return fmt.Errorf("storage is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(positionsBucket))
		if b == nil {
			return fmt.Errorf("positions bucket not found")
		}

		return b.Delete([]byte(cellID))
	})
}

// SaveContainerID saves a container ID for a cell
func (s *Storage) SaveContainerID(ctx context.Context, cellID, containerID string) error {
	if s.readOnly {
		return fmt.Errorf("storage is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(containerIDBucket))
		if b == nil {
			return fmt.Errorf("container ID bucket not found")
		}

		return b.Put([]byte(cellID), []byte(containerID))
	})
}

// GetContainerID retrieves a container ID for a cell
func (s *Storage) GetContainerID(ctx context.Context, cellID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var containerID string

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(containerIDBucket))
		if b == nil {
			return fmt.Errorf("container ID bucket not found")
		}

		data := b.Get([]byte(cellID))
		if data == nil {
			return fmt.Errorf("container ID not found for cell %s", cellID)
		}

		containerID = string(data)
		return nil
	})

	if err != nil {
		return "", err
	}

	return containerID, nil
}

// DeleteContainerID removes a container ID for a cell
func (s *Storage) DeleteContainerID(ctx context.Context, cellID string) error {
	if s.readOnly {
		return fmt.Errorf("storage is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(containerIDBucket))
		if b == nil {
			return fmt.Errorf("container ID bucket not found")
		}

		return b.Delete([]byte(cellID))
	})
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}
