package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
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

// Default port can be overridden during build with:
// -ldflags "-X main.defaultPort=8081"
var defaultPort = "8080"

var (
	rootPath string
	port     string
	newDB    bool
	storage  *PositionStorage
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

var rootCmd = &cobra.Command{
	Use:   "vibethis [path]",
	Short: "A graph visualizer for directory dependencies",
	Long: `vibethis is a tool that visualizes directory dependencies in your project.
It scans for dependencies.yaml files and creates an interactive graph visualization.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runServer,
}

func init() {
	rootCmd.Flags().StringVarP(&port, "port", "p", defaultPort, "Port to listen on")
	rootCmd.Flags().BoolVarP(&newDB, "new", "n", false, "Create a new database if it doesn't exist")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Set root path from argument or default to current directory
	if len(args) > 0 {
		rootPath = args[0]
	} else {
		rootPath = "."
	}

	// Check if database exists
	dbPath := filepath.Join(rootPath, ".vibestate.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) && !newDB {
		return fmt.Errorf("no .vibestate.db found in %s. Use -n/--new flag to create a new database", rootPath)
	}

	// Initialize storage in the directory being served
	var err error
	storage, err = NewPositionStorage(rootPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %v", err)
	}
	defer storage.Close()

	// Create server instance
	server := NewServer(rootPath, storage)

	router := mux.NewRouter()
	
	router.HandleFunc("/api/graph", getGraphHandler).Methods("GET")
	router.HandleFunc("/api/files/{nodeId}", getFilesHandler).Methods("GET")
	router.HandleFunc("/api/positions", getPositionsHandler).Methods("GET")
	router.HandleFunc("/api/positions", savePositionsHandler).Methods("POST")
	router.HandleFunc("/ws/terminal/{nodeId}", terminalWebSocketHandler)
	
	// Container management endpoints
	router.HandleFunc("/api/nodes/{nodeId}/container/status", getContainerStatusHandler).Methods("GET")
	router.HandleFunc("/api/nodes/{nodeId}/container/create", createContainerHandler).Methods("POST")
	router.HandleFunc("/api/nodes/{nodeId}/container/start", startContainerHandler).Methods("POST")
	router.HandleFunc("/api/nodes/{nodeId}/container/restart", restartContainerHandler).Methods("POST")
	router.HandleFunc("/api/nodes/{nodeId}/container/reset", resetContainerHandler).Methods("POST")
	
	// File management endpoints for devcontainer.json
	router.HandleFunc("/api/nodes/{nodeId}/files/{filePath:.*}", getNodeFileHandler).Methods("GET")
	router.HandleFunc("/api/nodes/{nodeId}/files/{filePath:.*}", putNodeFileHandler).Methods("PUT")
	
	// Dependencies management endpoint
	router.HandleFunc("/api/nodes/{nodeId}/dependencies", updateDependenciesHandler).Methods("PUT")
	
	// Git management endpoints
	router.HandleFunc("/api/nodes/{nodeId}/git/status", server.handleGitStatus).Methods("GET")
	router.HandleFunc("/api/nodes/{nodeId}/git/diff", server.handleGitDiff).Methods("GET")
	router.HandleFunc("/api/nodes/{nodeId}/git/commit", server.handleGitCommit).Methods("POST")
	router.HandleFunc("/api/nodes/{nodeId}/git/history", server.handleGitHistory).Methods("GET")
	
	router.PathPrefix("/").Handler(getFrontendHandler())

	fmt.Printf("Server starting on :%s, scanning path: %s\n", port, rootPath)
	return http.ListenAndServe(":"+port, corsMiddleware(router))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func getGraphHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("GET /api/graph - rootPath: %s", rootPath)
	graph := buildGraph(rootPath)
	
	log.Printf("Built graph with %d nodes and %d edges", len(graph.Nodes), len(graph.Edges))
	if len(graph.Nodes) == 0 {
		log.Printf("Warning: No nodes found. Check if dependencies.yaml files exist in %s", rootPath)
	}
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(graph); err != nil {
		log.Printf("Error encoding graph response: %v", err)
		http.Error(w, "Failed to encode graph", http.StatusInternalServerError)
		return
	}
}

func getFilesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	subPath := r.URL.Query().Get("path")
	
	log.Printf("GET /api/files/%s - subPath: %s", nodeID, subPath)
	
	// Find the node's directory
	var nodePath string
	err := filepath.Walk(rootPath, func(dirPath string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		
		if filepath.Base(dirPath) == nodeID {
			relPath, _ := filepath.Rel(rootPath, dirPath)
			nodePath = relPath
			log.Printf("Found node %s at path: %s", nodeID, relPath)
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil || nodePath == "" {
		log.Printf("Node %s not found in %s", nodeID, rootPath)
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	
	// Build the full path
	fullPath := filepath.Join(rootPath, nodePath)
	if subPath != "" {
		fullPath = filepath.Join(fullPath, subPath)
	}
	
	log.Printf("Reading directory: %s", fullPath)
	
	// Read directory contents
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		log.Printf("Failed to read directory %s: %v", fullPath, err)
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

func getPositionsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("GET /api/positions - rootPath: %s", rootPath)
	positionsMap, err := storage.GetPositions(rootPath)
	if err != nil {
		log.Printf("Error getting positions: %v", err)
		http.Error(w, "Failed to get positions", http.StatusInternalServerError)
		return
	}
	
	// Convert map to array
	var positions []NodePosition
	for _, pos := range positionsMap {
		positions = append(positions, pos)
	}
	
	log.Printf("Returning %d positions", len(positions))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(positions)
}

func savePositionsHandler(w http.ResponseWriter, r *http.Request) {
	var positions []NodePosition
	if err := json.NewDecoder(r.Body).Decode(&positions); err != nil {
		log.Printf("Error decoding positions: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	log.Printf("POST /api/positions - Saving %d positions for rootPath: %s", len(positions), rootPath)
	if err := storage.SavePositions(rootPath, positions); err != nil {
		log.Printf("Error saving positions: %v", err)
		http.Error(w, "Failed to save positions", http.StatusInternalServerError)
		return
	}
	
	log.Printf("Successfully saved %d positions", len(positions))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func updateDependenciesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	
	var request struct {
		Dependencies []string `json:"dependencies"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("Error decoding dependencies request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Find the node's directory
	var nodePath string
	err := filepath.Walk(rootPath, func(dirPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() {
			return nil
		}
		
		if filepath.Base(dirPath) == nodeID {
			nodePath = dirPath
			return filepath.SkipDir
		}
		
		return nil
	})
	
	if err != nil || nodePath == "" {
		log.Printf("Node %s not found", nodeID)
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	
	// Create the dependency structure
	dep := Dependency{
		Dependencies: request.Dependencies,
	}
	
	// Marshal to YAML
	data, err := yaml.Marshal(&dep)
	if err != nil {
		log.Printf("Error marshaling dependencies: %v", err)
		http.Error(w, "Failed to encode dependencies", http.StatusInternalServerError)
		return
	}
	
	// Write to dependencies.yaml file
	depFile := filepath.Join(nodePath, "dependencies.yaml")
	if err := os.WriteFile(depFile, data, 0644); err != nil {
		log.Printf("Error writing dependencies file: %v", err)
		http.Error(w, "Failed to save dependencies", http.StatusInternalServerError)
		return
	}
	
	log.Printf("Updated dependencies for node %s: %v", nodeID, request.Dependencies)
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
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
