package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

type Dependency struct {
	Dependencies []string `yaml:"dependencies"`
}

type Node struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Dependencies []string `json:"dependencies"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type FileInfo struct {
	Name  string    `json:"name"`
	Path  string    `json:"path"`
	IsDir bool      `json:"isDir"`
	Size  int64     `json:"size"`
	Type  string    `json:"type"`
}

type FilesResponse struct {
	Files []FileInfo `json:"files"`
	Path  string     `json:"path"`
}

var (
	rootPath string
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func main() {
	flag.StringVar(&rootPath, "path", ".", "Root path to scan for directories")
	flag.Parse()

	router := mux.NewRouter()
	
	router.HandleFunc("/api/graph", getGraphHandler).Methods("GET")
	router.HandleFunc("/api/files/{nodeId}", getFilesHandler).Methods("GET")
	router.HandleFunc("/ws/terminal/{nodeId}", terminalWebSocketHandler)
	
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("../web/dist/")))

	fmt.Printf("Server starting on :8080, scanning path: %s\n", rootPath)
	log.Fatal(http.ListenAndServe(":8080", corsMiddleware(router)))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func getGraphHandler(w http.ResponseWriter, r *http.Request) {
	graph := buildGraph(rootPath)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}

func getFilesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	subPath := r.URL.Query().Get("path")
	
	// Find the node's directory
	var nodePath string
	err := filepath.Walk(rootPath, func(dirPath string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		
		if filepath.Base(dirPath) == nodeID {
			relPath, _ := filepath.Rel(rootPath, dirPath)
			nodePath = relPath
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil || nodePath == "" {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	
	// Build the full path
	fullPath := filepath.Join(rootPath, nodePath)
	if subPath != "" {
		fullPath = filepath.Join(fullPath, subPath)
	}
	
	// Read directory contents
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, "Failed to read directory", http.StatusInternalServerError)
		return
	}
	
	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		fileType := "file"
		if entry.IsDir() {
			fileType = "folder"
		} else if filepath.Ext(entry.Name()) != "" {
			fileType = filepath.Ext(entry.Name())[1:] // Remove the dot
		}
		
		files = append(files, FileInfo{
			Name:  entry.Name(),
			Path:  filepath.Join(subPath, entry.Name()),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
			Type:  fileType,
		})
	}
	
	response := FilesResponse{
		Files: files,
		Path:  subPath,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func buildGraph(path string) Graph {
	var nodes []Node
	nodeMap := make(map[string]*Node)
	
	err := filepath.Walk(path, func(dirPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() {
			return nil
		}
		
		depFile := filepath.Join(dirPath, "dependencies.yaml")
		if _, err := os.Stat(depFile); os.IsNotExist(err) {
			return nil
		}
		
		relPath, _ := filepath.Rel(path, dirPath)
		nodeID := filepath.Base(dirPath)
		
		node := Node{
			ID:   nodeID,
			Name: nodeID,
			Path: relPath,
		}
		
		data, err := os.ReadFile(depFile)
		if err == nil {
			var dep Dependency
			if err := yaml.Unmarshal(data, &dep); err == nil {
				node.Dependencies = dep.Dependencies
			}
		}
		
		nodes = append(nodes, node)
		nodeMap[nodeID] = &node
		
		return nil
	})
	
	if err != nil {
		log.Printf("Error walking directory: %v", err)
	}
	
	var edges []Edge
	for _, node := range nodes {
		for _, dep := range node.Dependencies {
			edges = append(edges, Edge{
				Source: node.ID,
				Target: dep,
			})
		}
	}
	
	return Graph{
		Nodes: nodes,
		Edges: edges,
	}
}

func terminalWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	
	// Set read deadline to handle idle connections
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	go func() {
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
	
	// Send initial message
	initialMsg := fmt.Sprintf("Connected to %s terminal\r\n$ ", nodeID)
	conn.WriteMessage(websocket.TextMessage, []byte(initialMsg))
	
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			// Only log unexpected errors
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for node %s: %v", nodeID, err)
			}
			break
		}
		
		response := fmt.Sprintf("%s", string(message))
		if err := conn.WriteMessage(messageType, []byte(response)); err != nil {
			break
		}
		
		// Reset read deadline on successful message
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	}
}