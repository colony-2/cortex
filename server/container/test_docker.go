package main

import (
    "context"
    "fmt"
    "time"
    "github.com/docker/docker/client"
)

func main() {
    // Try default first
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        fmt.Printf("Error with FromEnv: %v\n", err)
        // Try Docker Desktop socket
        cli, err = client.NewClientWithOpts(
            client.WithHost("unix:///Users/jnadeau/.docker/run/docker.sock"),
            client.WithAPIVersionNegotiation(),
        )
        if err != nil {
            fmt.Printf("Error creating client with socket: %v\n", err)
            return
        }
    }
    defer cli.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    _, err = cli.Ping(ctx)
    if err != nil {
        fmt.Printf("Error pinging Docker: %v\n", err)
        return
    }
    fmt.Println("Docker is available!")
}