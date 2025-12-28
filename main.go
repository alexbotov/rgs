package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	fmt.Println("🎰 RGS - Remote Gaming Server")
	fmt.Println("Starting server on :8080...")

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"RGS","version":"0.1.0","description":"Remote Gaming Server"}`))
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

