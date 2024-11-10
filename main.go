package main

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// Template data structure
type PageData struct {
	Title       string
	MovieName   string
	ContentType string
}

func main() {
	app := fiber.New()
	app.Use(logger.New()) // Logger for tracking requests

	// Route to serve the HTML player
	app.Get("/stream/:movie", func(c *fiber.Ctx) error {
		movieName := c.Params("movie")
		var movieFilePath string
		supportedExtensions := []string{"mp4", "mkv"}
		found := false

		// Locate file with supported extension
		for _, ext := range supportedExtensions {
			path := fmt.Sprintf("movies/%s.%s", movieName, ext)
			if _, err := os.Stat(path); err == nil {
				movieFilePath = path
				found = true
				break
			}
		}

		if !found {
			return c.Status(fiber.StatusNotFound).SendString("Movie not found.")
		}

		// Set the correct content type based on file extension
		ext := strings.ToLower(filepath.Ext(movieFilePath))
		var contentType string
		switch ext {
		case ".mp4":
			contentType = "video/mp4"
		case ".mkv":
			contentType = "video/x-matroska"
		default:
			return c.Status(fiber.StatusForbidden).SendString("Unsupported file format.")
		}

		// Parse and execute the HTML template
		tmpl, err := template.ParseFiles("index.html")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load HTML template.")
		}

		data := PageData{
			Title:       fmt.Sprintf("Streaming %s", movieName),
			MovieName:   movieName,
			ContentType: contentType,
		}

		// Render the template into the response
		var renderedPage strings.Builder
		if err := tmpl.Execute(&renderedPage, data); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to render HTML template.")
		}

		return c.Status(fiber.StatusOK).Type("html").SendString(renderedPage.String())
	})

	// Route for serving the video file with range support
	app.Get("/video/:movie", func(c *fiber.Ctx) error {
		movieName := c.Params("movie")
		var movieFilePath string
		supportedExtensions := []string{"mp4", "mkv"}
		found := false

		// Locate file path for video file
		for _, ext := range supportedExtensions {
			path := fmt.Sprintf("movies/%s.%s", movieName, ext)
			if _, err := os.Stat(path); err == nil {
				movieFilePath = path
				found = true
				break
			}
		}

		if !found {
			return c.Status(fiber.StatusNotFound).SendString("Movie not found.")
		}

		file, err := os.Open(movieFilePath)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Could not open video file.")
		}
		defer file.Close()

		// Get file size
		fileInfo, err := file.Stat()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Could not get file info.")
		}
		fileSize := fileInfo.Size()

		// Set headers for content type and range support
		ext := strings.ToLower(filepath.Ext(movieFilePath))
		switch ext {
		case ".mp4":
			c.Set("Content-Type", "video/mp4")
		case ".mkv":
			c.Set("Content-Type", "video/x-matroska")
		}
		c.Set("Accept-Ranges", "bytes")

		// Handle range requests
		rangeHeader := c.Get("Range")
		if rangeHeader == "" {
			// If no range is specified, send the first 2MB for fast starting
			c.Set("Content-Length", strconv.FormatInt(2*1024*1024, 10)) // 2 MB
			return c.SendFile(movieFilePath)
		}

		// Parse the range header (e.g., bytes=0-1048575)
		rangeParts := strings.Split(rangeHeader, "=")
		// Check if the first value is 'bytes', and the second value is a valid range
		if len(rangeParts) != 2 || rangeParts[0] != "bytes" {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid Range header.")
		}

		rangeValues := strings.Split(rangeParts[1], "-")
		// Now check if the first value is defined, and if the second is empty then set it to a large value which should correspond to the start and the file.
		if rangeValues[0] == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid Range header.")
		}
		start, err := strconv.ParseInt(rangeValues[0], 10, 64)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid start byte in Range header.")
		}
		end := start + 2*1024*1024 - 1 // 2 MB

		// Ensure the 'start' is within the file size
		if start < 0 || start >= fileSize {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid start byte in Range header.")
		}

		// Make sure the range does not exceed the file size
		if end >= fileSize {
			end = fileSize - 1
		}

		// Calculate the length of the data to be sent
		length := end - start + 1

		// Set headers for partial content
		c.Status(fiber.StatusPartialContent)
		c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		c.Set("Content-Length", strconv.FormatInt(length, 10))

		// Stream the requested byte range
		file.Seek(start, 0)
		buffer := make([]byte, 6144) // Read in 6KB chunks (adjustable)
		bytesSent := int64(0)

		for bytesSent < length {
			remaining := length - bytesSent
			readSize := int64(len(buffer))
			if remaining < readSize {
				readSize = remaining
			}

			n, err := file.Read(buffer[:readSize])
			if err != nil && err.Error() != "EOF" { // Handle error other than EOF
				log.Printf("Error reading file: %v", err)
				break
			}

			// Ensure that we don't break prematurely
			if n == 0 {
				break
			}

			// Write the data chunk to the response
			if _, err := c.Write(buffer[:n]); err != nil {
				log.Printf("Failed to send video content: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to send video content.")
			}

			bytesSent += int64(n)
		}

		return nil
	})

	// Start server on all network interfaces at port 3000
	log.Fatal(app.Listen("0.0.0.0:3000"))
}
