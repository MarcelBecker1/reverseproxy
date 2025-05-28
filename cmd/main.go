package main

import (
	"fmt"
	"log"

	"github.com/MarcelBecker1/reverseproxy/internal/proxy"
)

func main() {
	fmt.Println("Starting Server")
	server := proxy.New(&proxy.Config{
		Host: "localhost",
		Port: 8080,
	})

	if err := server.Start(); err != nil {
		log.Fatal("Failed to Start server:", err)
	}
}
