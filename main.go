package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings" // Required for streamHandler
	"strconv" // Required for parsing ID in streamHandler
	
	_ "github.com/lib/pq" // PostgreSQL driver
)

// Global variables
var db *sql.DB

func init() {
	// Database connection setup
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		db = nil
		return
	}

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error opening database connection: %v", err)
	}

	// -------------------------------------------------------------
	// 1. Automatic Table Creation Logic (To fix Shell access issues)
	// -------------------------------------------------------------
	err = db.Ping()
	if err != nil {
		log.Printf("Warning: Database ping failed in init: %v. Server will start but DB status will be bad.", err)
		return
	}
	
	// SQL to create the 'videos' table if it doesn't already exist
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
}

// Handler for the root path and health check
func homeHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := "Not Checked."
	if db != nil {
		err := db.Ping()
		if err == nil {
			dbStatus = "Connected."
		} else {
			dbStatus = fmt.Sprintf("Failed to connect: %v", err)
		}
	}

	fmt.Fprintf(w, "Hello Naveen ji! Your Go Video Streamer server is running. Database Status: %s", dbStatus)
}

// -------------------------------------------------------------
// 2. Handler to upload and encode video
// -------------------------------------------------------------
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed. Use POST.", http.StatusMethodNotAllowed)
		return
	}

	// 1. Get the uploaded file (max 100MB)
	r.ParseMultipartForm(100 << 20) // 100MB limit
	file, header, err := r.FormFile("videoFile")
	if err != nil {
		http.Error(w, "Error retrieving the file. Ensure 'videoFile' is used: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 2. Save the uploaded file to a temporary location
	tempFile, err := os.CreateTemp("", "upload-*.mp4")
	if err != nil {
		http.Error(w, "Error creating temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name()) // Clean up the temp file after use

	_, err = io.Copy(tempFile, file)
	if err != nil {
		http.Error(w, "Error saving file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Successfully uploaded temporary file: %s", tempFile.Name())

	// 3. Define output path and encoding command (VP9)
	outputFileName := fmt.Sprintf("encoded_%d.webm", os.Getpid()) // Use PID for unique name in /tmp
	outputPath := "/tmp/" + outputFileName

	cmd := exec.Command("ffmpeg", 
		"-i", tempFile.Name(), 
		"-c:v", "libvpx-vp9", 
		"-b:v", "1M",
		"-y", 
		outputPath,
	)

	// 4. Run the encoding command
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg output: %s", string(output))
		http.Error(w, "Video encoding failed. Is FFmpeg installed? "+err.Error(), http.StatusInternalServerError)
		return
	}
	
	// 5. Success: Save metadata to database
	var videoID int
	if db != nil {
		// Use QueryRow and Scan to get the new ID back
		err = db.QueryRow("INSERT INTO videos (original_filename, encoded_path, status) VALUES ($1, $2, $3) RETURNING id", 
			header.Filename, outputPath, "encoded").Scan(&videoID)
		
		if err != nil {
			log.Printf("Database Insert Failed: %v", err)
			http.Error(w, "Encoding successful, but DB save failed.", http.StatusInternalServerError)
			return
		}
	} else {
		log.Println("Warning: Database connection is nil. Metadata not saved.")
	}

	// 6. Final success response (Tells user the ID for streaming)
	fmt.Fprintf(w, "Successfully encoded video! ID for streaming: %d. File Path: %s. (Note: File is temporary)", videoID, outputPath)
}


// -------------------------------------------------------------
// 3. Handler to stream the encoded video
// -------------------------------------------------------------

// Utility function to safely split the URL path
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	// Trim the leading slash and split
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Get video ID from the URL path (e.g., /stream/1)
	parts := splitPath(r.URL.Path) 
    
    // Check if parts has at least 3 elements (to get the ID: [ "stream", "1" ])
    if len(parts) < 2 { // We only need "stream" and "1", so 2 elements
        http.Error(w, "Video ID missing in URL.", http.StatusBadRequest)
        return
    }

	// The ID is the last element
	videoIDStr := parts[len(parts)-1]
	videoID, err := strconv.Atoi(videoIDStr)

    if err != nil || videoID == 0 {
		http.Error(w, "Invalid Video ID.", http.StatusBadRequest)
		return
    }
	
	// 2. Fetch the encoded file path from the database
	var encodedPath string
	if db != nil {
		err := db.QueryRow("SELECT encoded_path FROM videos WHERE id = $1", videoID).Scan(&encodedPath)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Video not found.", http.StatusNotFound)
				return
			}
			log.Printf("DB Query Error: %v", err)
			http.Error(w, "Database error retrieving video path.", http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "Database not connected.", http.StatusInternalServerError)
		return
	}

	// 3. Serve the file (Efficiently streams the content)
	// Important: The Content-Type must be 'video/webm' for VP9 streams
	w.Header().Set("Content-Type", "video/webm")
	
	// http.ServeFile handles range requests (which is crucial for streaming)
	http.ServeFile(w, r, encodedPath)
}

func main() {
	// Root and health check handler
	http.HandleFunc("/", homeHandler)
	// Handler for video upload (POST)
	http.HandleFunc("/upload", uploadHandler)
	// Handler for video streaming (GET) - Trailing slash is important for routing /stream/1
	http.HandleFunc("/stream/", streamHandler) 

	// Get PORT from environment variable (set to 10000 on Render)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port for local testing
	}

	log.Printf("Server starting on port %s...", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
