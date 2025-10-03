package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq" // PostgreSQL driver for database
)

func main() {
	// DATABASE_URL environment variable se fetch karte hain
	dbURL := os.Getenv("DATABASE_URL")

	// Database connection status check
	dbStatus := "Not Checked"

	if dbURL != "" {
		// sql.Open se database handle milta hai
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			log.Printf("Error connecting to database: %v", err)
			dbStatus = fmt.Sprintf("Connection Failed: %v", err)
		} else {
			// Connection band karna zaroori hai
			defer db.Close()

			// Ping se check karte hain ki database se communication ho raha hai ya nahi
			err = db.Ping()
			if err != nil {
				log.Printf("Error performing database ping: %v", err)
				dbStatus = fmt.Sprintf("Ping Failed: %v", err)
			} else {
				log.Println("Successfully connected to the PostgreSQL database!")
				dbStatus = "Connected"
			}
		}
	} else {
		log.Println("WARNING: DATABASE_URL is not set. Database check skipped.")
	}

	// PORT environment variable set karte hain (Render 10000 use karta hai)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Local/Codespace testing ke liye
	}

	// Basic Home route handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain") // Simple text response
		fmt.Fprintf(w, "Hello Naveen! Your Go Video Streamer server is running.\nDatabase Status: %s.", dbStatus)
	})

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
