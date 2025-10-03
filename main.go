package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	// Removed "strconv" (from previous fix)
	
	_ "github.com/lib/pq"
)

// Global variables
var db *sql.DB

func init() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		db = nil
		return
	}

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	// -------------------------------------------------------------
	// NEW: Table creation logic - Yeh code table check aur create karega
	// -------------------------------------------------------------

	// Ping the DB to ensure connection is live before running DDL
	err = db.Ping()
	if err != nil {
		log.Printf("Warning: Database ping failed in init: %v. Server will start but DB status will be bad.", err)
		return
	}
	
	// SQL command to create the table IF IT DOES NOT EXIST
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS videos (
			id SERIAL PRIMARY KEY,
			original_filename VARCHAR(255) NOT NULL,
			encoded_path VARCHAR(255) NOT NULL,
			status VARCHAR(50) DEFAULT 'encoded',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
	`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Error creating videos table: %v", err)
	}

	log.Println("Database connection successful and 'videos' table ensured.")
	// -------------------------------------------------------------
}

// ... (homeHandler and uploadHandler remain the same as the previous update) ...

// **VERY IMPORTANT:**
// uploadHandler must be the one with the DB insertion logic:
/*
func uploadHandler(w http.ResponseWriter, r *http.Request) {
    // ... (File upload, encoding setup) ...
    // ... (Existing code for FFmpeg command setup remains the same) ...
	
    // 4. Run the encoding command
    // ...

	// 5. Success: Save metadata to database (THE DB INSERTION PART)
	if db != nil {
		_, err = db.Exec("INSERT INTO videos (original_filename, encoded_path, status) VALUES ($1, $2, $3)", 
			header.Filename, outputPath, "encoded")
		
		// ... (Error checking logic) ...
	}
    // ...
}
*/

func main() {
    // ... (Rest of main function remains the same) ...
}
