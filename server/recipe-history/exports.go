package recipehistory

// Export public types and constants for easier access

// Re-export key interfaces and types that consumers will need
type History = Client

// Ensure interface compatibility
var _ = &Client{}