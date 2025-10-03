package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	_ "github.com/lib/pq"
)

// Database connection
var db *sql.DB

// Video struct mein naye metadata fields shamil kiye gaye hain
type Video struct {
	ID             int
	Title          string
	Description    string
	Keywords       string
	Privacy        string
	OriginalName   string
	EncodedPath    string
	Status         string
	CreatedAt      time.Time
}

// ---------------------------
// 1. Database Setup
// ---------------------------

func initDB() {
	// Database connection string environment variable se padhna
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("FATAL: DATABASE_URL environment variable set nahi hai.")
	}

	var err error
	// Database se connect karna
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("FATAL: Database connection fail ho gaya: %v", err)
	}

	// Connection ko check karna
	err = db.Ping()
	if err != nil {
		log.Fatalf("FATAL: Database ping fail ho gaya: %v", err)
	}

	log.Println("SUCCESS: Database successfully connect ho gaya.")

	// Table banaana aur migrate karna
	createTable()
}

// createTable function ko safe migration ke liye update kiya gaya hai
func createTable() {
	// Step 1: Pehle check karo ki 'videos' table exist karti hai ya nahi. Agar nahi karti, toh nayi banao.
	initialQuery := `
	CREATE TABLE IF NOT EXISTS videos (
		id SERIAL PRIMARY KEY,
		original_name VARCHAR(255) NOT NULL,
		encoded_path VARCHAR(255) NOT NULL,
		status VARCHAR(50) DEFAULT 'encoded',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := db.Exec(initialQuery)
	if err != nil {
		log.Fatalf("FATAL: Initial Table creation fail ho gaya: %v", err)
	}
	log.Println("SUCCESS: 'videos' table check/create ho gayi.")
	
	// Step 2: Columns ki availability check karna aur missing columns ko add karna.
	// Isse hum migration karte hain aur purana data safe rehta hai.
	
	columnsToAdd := map[string]string{
		"title": "VARCHAR(100) NOT NULL DEFAULT 'Untitled Video'",
		"description": "TEXT NOT NULL DEFAULT 'No description provided.'",
		"keywords": "VARCHAR(500)",
		"privacy": "VARCHAR(10) NOT NULL DEFAULT 'public'",
	}

	for colName, colDefinition := range columnsToAdd {
		// PostgreSQL information_schema se column exist karta hai ya nahi check karna
		checkQuery := `
		SELECT 1 
		FROM information_schema.columns 
		WHERE table_name='videos' AND column_name=$1;`
		
		var exists int
		err := db.QueryRow(checkQuery, colName).Scan(&exists)

		if err == sql.ErrNoRows {
			// Column exist nahi karta, toh use ALTER TABLE se add karna
			alterQuery := fmt.Sprintf("ALTER TABLE videos ADD COLUMN %s %s", colName, colDefinition)
			_, alterErr := db.Exec(alterQuery)
			
			if alterErr != nil {
				log.Printf("WARNING: Column '%s' add karne mein error: %v", colName, alterErr)
			} else {
				log.Printf("SUCCESS: Table 'videos' mein naya column '%s' successfully add kiya gaya.", colName)
			}
		} else if err != nil {
			log.Printf("WARNING: Column check error: %v", err)
		}
	}
}

// updateStatus aur updateStatusAndPath functions, aur Video struct, aur handers wahi rahenge
// ... (The rest of the main.go code remains the same as provided in the previous response)

// ---------------------------
// 2. Handlers
// ---------------------------

// homeHandler naye index.html ko serve karta hai
func homeHandler(w http.ResponseWriter, r *http.Request) {
// ... (Wahi code)
}

// uploadHandler ab metadata ko bhi process aur save karta hai
func uploadHandler(w http.ResponseWriter, r *http.Request) {
// ... (Wahi code)
}

// streamHandler ab database se path nikal kar video serve karta hai
func streamHandler(w http.ResponseWriter, r *http.Request) {
// ... (Wahi code)
}

// ---------------------------
// 3. DB Utility Functions
// ---------------------------

func updateStatus(id int, newStatus string) {
// ... (Wahi code)
}

func updateStatusAndPath(id int, newStatus, newPath string) {
// ... (Wahi code)
}


// ---------------------------
// 4. Main Function
// ---------------------------

func main() {
	// Database initialize karna
	initDB()
	
	// FFmpeg ki availability check karna
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("WARNING: FFmpeg found nahi hua: %v", err)
		log.Println("Agar aap Render par hain, toh render.yaml mein 'apt' section check karein.")
	} else {
		log.Println("SUCCESS: FFmpeg found.")
	}

	// Handlers define karna
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/stream/", streamHandler)

	// Port define karna
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port for local testing
	}

	log.Printf("Server port: %s par chal raha hai...", port)
	// Server start karna
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("FATAL: Server start karne mein error: %v", err)
	}
}
