package main

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

type NodePosition struct {
	NodeID string  `json:"nodeId"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

type PositionStorage struct {
	db *bolt.DB
}

const (
	bucketName = "node_positions"
	dbFileName = ".vibestate.db"
)

// NewPositionStorage creates a new position storage instance
func NewPositionStorage(dataDir string) (*PositionStorage, error) {
	dbPath := filepath.Join(dataDir, dbFileName)
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create bucket if it doesn't exist
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	return &PositionStorage{db: db}, nil
}

// SavePosition saves a node's position
func (ps *PositionStorage) SavePosition(graphPath string, position NodePosition) error {
	return ps.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		
		// Create a key combining graph path and node ID
		key := fmt.Sprintf("%s:%s", graphPath, position.NodeID)
		
		// Marshal position to JSON
		data, err := json.Marshal(position)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(key), data)
	})
}

// SavePositions saves multiple node positions at once
func (ps *PositionStorage) SavePositions(graphPath string, positions []NodePosition) error {
	return ps.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		
		for _, position := range positions {
			key := fmt.Sprintf("%s:%s", graphPath, position.NodeID)
			data, err := json.Marshal(position)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(key), data); err != nil {
				return err
			}
		}
		
		return nil
	})
}

// GetPositions retrieves all saved positions for a graph
func (ps *PositionStorage) GetPositions(graphPath string) (map[string]NodePosition, error) {
	positions := make(map[string]NodePosition)
	
	err := ps.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		
		prefix := []byte(graphPath + ":")
		c := b.Cursor()
		
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix); k, v = c.Next() {
			// Check if key starts with our prefix
			if string(k[:len(prefix)]) != string(prefix) {
				break
			}
			
			var position NodePosition
			if err := json.Unmarshal(v, &position); err != nil {
				log.Printf("Failed to unmarshal position: %v", err)
				continue
			}
			
			positions[position.NodeID] = position
		}
		
		return nil
	})
	
	return positions, err
}

// Close closes the database
func (ps *PositionStorage) Close() error {
	return ps.db.Close()
}