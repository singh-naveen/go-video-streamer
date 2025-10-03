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

// Video struct: Saare naye fields shamil hain
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
// 1. Database Setup & Migration (ALTER TABLE logic)
// ---------------------------

func initDB() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("FATAL: DATABASE_URL environment variable set nahi hai.")
	}

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("FATAL: Database connection fail ho gaya: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("FATAL: Database ping fail ho gaya: %v", err)
	}

	log.Println("SUCCESS: Database successfully connect ho gaya.")

	createTable() // Table create aur migrate karna
}

func createTable() {
	// Step 1: Pehle check karo ki 'videos' table exist karti hai ya nahi.
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

	// Step 2: Missing columns ko ALTER TABLE se add karna (Safe Migration).
	columnsToAdd := map[string]string{
		"title":       "VARCHAR(100) NOT NULL DEFAULT 'Untitled Video'",
		"description": "TEXT NOT NULL DEFAULT 'No description provided.'",
		"keywords":    "VARCHAR(500)",
		"privacy":     "VARCHAR(10) NOT NULL DEFAULT 'public'",
	}

	for colName, colDefinition := range columnsToAdd {
		checkQuery := `
		SELECT 1 
		FROM information_schema.columns 
		WHERE table_name='videos' AND column_name=$1;`
		
		var exists int
		err := db.QueryRow(checkQuery, colName).Scan(&exists)

		if err == sql.ErrNoRows {
			// Column exist nahi karta, toh add karna
			alterQuery := fmt.Sprintf("ALTER TABLE videos ADD COLUMN %s %s", colName, colDefinition)
			_, alterErr := db.Exec(alterQuery)
			
			if alterErr != nil {
				log.Printf("WARNING: Column '%s' add karne mein error: %v", colName, alterErr)
			} else {
				log.Printf("SUCCESS: Table 'videos' mein naya column '%s' successfully add kiya gaya.", colName)
			}
		} else if err != nil && err != sql.ErrNoRows {
			log.Printf("WARNING: Column check error: %v", err)
		}
	}
}

// ---------------------------
// 2. Handlers
// ---------------------------

// homeHandler: index.html serve karta hai
func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var dbStatus string
	if db != nil && db.Ping() == nil {
		dbStatus = "Connected"
	} else {
		dbStatus = "Not Checked"
	}

	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Template loading error: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		DBStatus string
	}{
		DBStatus: dbStatus,
	}

	tmpl.Execute(w, data)
}

// uploadHandler: File receive karta hai, DB mein entry karta hai aur encoding shuru karta hai
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed. Use POST.", http.StatusMethodNotAllowed)
		return
	}

	// 1. Form Data Parse karna (Video file aur metadata)
	// Max 32MB upload size. Render par temporary file size ka dhyan rakhna padega.
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, fmt.Sprintf("Form data parse error: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Metadata Extract karna
	title := r.FormValue("title")
	description := r.FormValue("description")
	keywords := r.FormValue("keywords")
	privacy := r.FormValue("privacy")
	
	if title == "" || description == "" || privacy == "" {
		http.Error(w, "Error: Title, Description, aur Privacy fields bharna zaroori hai.", http.StatusBadRequest)
		return
	}

	// 3. Video File Extract karna
	file, handler, err := r.FormFile("file") // UI code mein name="file" hai
	if err != nil {
		http.Error(w, fmt.Sprintf("File upload error: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	originalName := handler.Filename
	// Temp file path banane ke liye temp dir aur original name use karna
	tempFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("%d-%s", time.Now().UnixNano(), originalName))

	// File ko /tmp directory mein save karna
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Temp file create error: %v", err), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, file); err != nil {
		http.Error(w, fmt.Sprintf("File copy error: %v", err), http.StatusInternalServerError)
		return
	}

	// 4. Database mein 'processing' status ke saath entry karna
	var videoID int
	query := `
	INSERT INTO videos (title, description, keywords, privacy, original_name, encoded_path, status)
	VALUES ($1, $2, $3, $4, $5, $6, 'processing') RETURNING id`

	// QueryRow se insert karna aur returning ID ko seedha nikal lena
	err = db.QueryRow(query, title, description, keywords, privacy, originalName, "").Scan(&videoID)
	
	if err != nil {
		log.Printf("DB insert error: %v", err)
		// Uploaded temp file ko delete karna agar DB error ho
		os.Remove(tempFilePath) 
		http.Error(w, "Database mein video entry karne mein error aayi.", http.StatusInternalServerError)
		return
	}

	// 5. Encoding Go routine shuru karna
	go encodeVideo(videoID, tempFilePath)

	// 6. Response bhejkar client ko bataana
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	// Client ko JSON response dena taki UI aage ka process shuru kar sake
	fmt.Fprintf(w, `{"message": "Video upload successful. Encoding started.", "id": %d, "stream_link": "/stream/%d"}`, videoID, videoID)
}

// streamHandler: Encoded video ko stream karta hai
func streamHandler(w http.ResponseWriter, r *http.Request) {
	// URL se video ID nikalna
	pathID := r.URL.Path[len("/stream/"):]
	videoID, err := strconv.Atoi(pathID)
	if err != nil {
		http.Error(w, "Invalid video ID", http.StatusBadRequest)
		return
	}

	// Database se video information nikalna
	var encodedPath string
	var status string
	err = db.QueryRow("SELECT encoded_path, status FROM videos WHERE id = $1", videoID).Scan(&encodedPath, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Video not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Status check karna
	if status != "encoded" {
		http.Error(w, fmt.Sprintf("Video status is: %s. Not ready for streaming.", status), http.StatusServiceUnavailable)
		return
	}

	// File serve karna
	w.Header().Set("Content-Type", "video/webm")
	http.ServeFile(w, r, encodedPath)
}

// ---------------------------
// 3. Encoding Logic
// ---------------------------

// encodeVideo: FFmpeg se video ko VP9 (WebM) mein encode karta hai
func encodeVideo(videoID int, inputPath string) {
	log.Printf("Encoding started for ID %d: %s", videoID, inputPath)
	updateStatus(videoID, "encoding")
	
	// Input file hamesha delete hoga, chahe fail ho ya pass
	defer os.Remove(inputPath)

	// Output file path tay karna (Render par /tmp mein)
	encodedFileName := fmt.Sprintf("encoded_%d.webm", videoID)
	outputPath := filepath.Join(os.TempDir(), encodedFileName)

	// FFmpeg command: VP9 codec, 16:9 ratio maintain karna, Opus audio
	// Ye command 16:9 aspect ratio ko force nahi karta, balki use preserve karne ki koshish karta hai
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,           // Input file
		"-c:v", "libvpx-vp9",      // Video codec (VP9)
		"-b:v", "1M",              // Video bitrate
		"-vf", "scale=-2:720",     // Scale to 720p height, maintain aspect ratio
		"-c:a", "libopus",         // Audio codec (Opus)
		"-b:a", "128k",
		"-f", "webm",              // Output format (WebM)
		"-y",                      // Agar file pehle se ho toh overwrite karna
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("ERROR: Encoding failed for ID %d: %v\nOutput:\n%s", videoID, err, string(output))
		updateStatus(videoID, "failed")
		return
	}

	// SUCCESS
	log.Printf("SUCCESS: Encoding done for ID %d. Path: %s", videoID, outputPath)
	updateStatusAndPath(videoID, "encoded", outputPath)
}

// ---------------------------
// 4. DB Utility Functions
// ---------------------------

func updateStatus(id int, newStatus string) {
	_, err := db.Exec("UPDATE videos SET status = $1 WHERE id = $2", newStatus, id)
	if err != nil {
		log.Printf("Error updating status for ID %d to %s: %v", id, newStatus, err)
	}
}

func updateStatusAndPath(id int, newStatus, newPath string) {
	_, err := db.Exec("UPDATE videos SET status = $1, encoded_path = $2 WHERE id = $3", newStatus, newPath, id)
	if err != nil {
		log.Printf("Error updating status and path for ID %d: %v", id, err)
	}
}

// ---------------------------
// 5. Main Function
// ---------------------------

func main() {
	initDB()

	// FFmpeg ki availability check karna
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("WARNING: FFmpeg not found: %v", err)
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
		port = "8080" // Default port
	}

	log.Printf("Server port: %s par chal raha hai...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("FATAL: Server start karne mein error: %v", err)
	}
}
