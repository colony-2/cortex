package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Server holds the application state and relationships
type Server struct {
	storage  *PositionStorage
	rootPath string
}

// NewServer creates a new server instance
func NewServer(rootPath string, storage *PositionStorage) *Server {
	return &Server{
		storage:  storage,
		rootPath: rootPath,
	}
}

// findNodePath finds the full path for a node by its ID
func (s *Server) findNodePath(nodeID string) (string, error) {
	var nodePath string
	err := filepath.Walk(s.rootPath, func(dirPath string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		
		if filepath.Base(dirPath) == nodeID {
			nodePath = dirPath
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil {
		return "", err
	}
	
	if nodePath == "" {
		return "", fmt.Errorf("node not found")
	}
	
	return nodePath, nil
}