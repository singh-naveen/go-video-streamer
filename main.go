package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"

	_ "github.com/lib/pq"
)

// Global variables
var db *sql.DB

func init() {
	// Database connection logic (Pehle se hi set hai)
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		db = nil
		return
	}

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
}

// Handler for the root path and health check
func homeHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := "Not Checked."
	if db != nil {
		// Ping the database to check connectivity
		err := db.Ping()
		if err == nil {
			dbStatus = "Connected."
		} else {
			dbStatus = fmt.Sprintf("Failed to connect: %v", err)
		}
	}

	fmt.Fprintf(w, "Hello Naveen ji ! Your Go Video Streamer server is running. Database Status: %s", dbStatus)
}

// NEW: Handler to upload and encode video
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Get the uploaded file
	file, header, err := r.FormFile("videoFile")
	if err != nil {
		http.Error(w, "Error retrieving the file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 2. Create a temporary file to save the uploaded video
	tempFile, err := os.CreateTemp("", "upload-*.mp4")
	if err != nil {
		http.Error(w, "Error creating temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name()) // Clean up the temp file

	// Copy the uploaded file to the temporary storage
	_, err = io.Copy(tempFile, file)
	if err != nil {
		http.Error(w, "Error saving file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Successfully uploaded temporary file: %s", tempFile.Name())

	// 3. Define output path and encoding command (VP9)
	outputFileName := "encoded_" + header.Filename + ".webm"
	outputPath := "/tmp/" + outputFileName

	// FFmpeg command to encode to VP9 (WebM format)
	// -i: input file
	// -c:v: video codec (libvpx-vp9)
	// -b:v: bitrate (low quality for fast test)
	// -y: overwrite output file if it exists
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
		http.Error(w, "Video encoding failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 5. Success response
	fmt.Fprintf(w, "Successfully encoded video! Output saved to: %s (Size: %d bytes)", outputPath, header.Size)
}

func main() {
	// Root and health check handler
	http.HandleFunc("/", homeHandler)
	// New handler for video upload
	http.HandleFunc("/upload", uploadHandler)

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

// Note: Testing this needs a client (like Postman or a simple HTML form) 
// to send a POST request with a file named 'videoFile'.
