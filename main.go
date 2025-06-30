package main

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

const targetSize = 990 * 1000 // 990KB for safety margin

func main() {
	fmt.Println("Image Compressor - Starting...")
	fmt.Printf("Target size: %d KB (%.2f MB)\n", targetSize/1000, float64(targetSize)/(1000*1000))
	
	// Get the directory where the binary is located
	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error getting executable path: %v\n", err)
		fmt.Println("Press Enter to exit...")
		fmt.Scanln()
		return
	}
	
	dir := filepath.Dir(execPath)
	fmt.Printf("Processing images in: %s\n", dir)
	
	// Create compressed directory
	compressedDir := filepath.Join(dir, "compressed")
	if err := os.MkdirAll(compressedDir, 0755); err != nil {
		fmt.Printf("Error creating compressed directory: %v\n", err)
		fmt.Println("Press Enter to exit...")
		fmt.Scanln()
		return
	}
	fmt.Printf("Output directory: %s\n\n", compressedDir)
	
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		fmt.Println("Press Enter to exit...")
		fmt.Scanln()
		return
	}
	
	processedCount := 0
	skippedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		ext := strings.ToLower(filepath.Ext(file.Name()))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" && ext != ".webp" && ext != ".heic" && ext != ".heif" {
			continue
		}
		
		filePath := filepath.Join(dir, file.Name())
		info, err := os.Stat(filePath)
		if err != nil {
			fmt.Printf("Error getting file info for %s: %v\n", file.Name(), err)
			continue
		}
		
		fmt.Printf("Processing %s (%.2f MB)... ", file.Name(), float64(info.Size())/(1000*1000))
		
		outputPath := filepath.Join(compressedDir, file.Name())
		
		if info.Size() <= targetSize {
			// Copy file as-is if already under target size
			if err := copyFile(filePath, outputPath); err != nil {
				fmt.Printf("ERROR copying: %v\n", err)
			} else {
				fmt.Printf("COPIED (already under target)\n")
				skippedCount++
			}
			continue
		}
		
		if err := compressImage(filePath, outputPath); err != nil {
			fmt.Printf("ERROR: %v\n", err)
		} else {
			// Verify the compressed file is actually under 1MB
			newInfo, err := os.Stat(outputPath)
			if err != nil {
				fmt.Printf("ERROR reading output: %v\n", err)
			} else if newInfo.Size() > targetSize {
				// Still too large, try more aggressive compression
				fmt.Printf("still %.2f MB, re-compressing... ", float64(newInfo.Size())/(1000*1000))
				if err := recompressImage(outputPath); err != nil {
					fmt.Printf("FAILED: %v\n", err)
					// Remove the failed file
					os.Remove(outputPath)
				} else {
					finalInfo, _ := os.Stat(outputPath)
					if finalInfo != nil && finalInfo.Size() <= targetSize {
						fmt.Printf("DONE (%.2f MB)\n", float64(finalInfo.Size())/(1000*1000))
						processedCount++
					} else {
						fmt.Printf("FAILED: Could not compress below 990KB\n")
						os.Remove(outputPath)
					}
				}
			} else {
				fmt.Printf("DONE (%.2f MB)\n", float64(newInfo.Size())/(1000*1000))
				processedCount++
			}
		}
	}
	
	fmt.Printf("\nCompleted! Compressed %d images, copied %d images.\n", processedCount, skippedCount)
	fmt.Printf("All output saved to: %s\n", compressedDir)
	fmt.Println("Press Enter to exit...")
	fmt.Scanln()
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

func compressImage(srcPath, dstPath string) error {
	ext := strings.ToLower(filepath.Ext(srcPath))
	
	// Handle HEIC/HEIF files separately
	if ext == ".heic" || ext == ".heif" {
		return compressHEIC(srcPath, dstPath)
	}
	
	// Read the original image
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Decode the image
	img, format, err := image.Decode(file)
	if err != nil {
		return err
	}
	file.Close()
	
	// Compress based on format
	switch format {
	case "jpeg":
		return compressJPEG(dstPath, img)
	case "png":
		return compressPNG(srcPath, dstPath, img)
	case "gif":
		return compressGIF(srcPath, dstPath, img)
	default:
		// For unsupported formats, try to save as JPEG
		jpegPath := strings.TrimSuffix(dstPath, filepath.Ext(dstPath)) + ".jpg"
		return compressJPEG(jpegPath, img)
	}
}

func compressJPEG(dstPath string, img image.Image) error {
	quality := 95
	
	// Try different quality levels
	for quality > 10 {
		var buffer bytes.Buffer
		err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: quality})
		if err != nil {
			return err
		}
		
		if buffer.Len() <= targetSize {
			// Found a good quality level
			return os.WriteFile(dstPath, buffer.Bytes(), 0644)
		}
		
		// Adjust quality based on how far we are from target
		ratio := float64(buffer.Len()) / float64(targetSize)
		if ratio > 2 {
			quality -= 20
		} else if ratio > 1.5 {
			quality -= 10
		} else {
			quality -= 5
		}
		
		if quality < 10 {
			quality = 10
		}
	}
	
	// If we can't get it small enough, use quality 10
	var buffer bytes.Buffer
	err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 10})
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, buffer.Bytes(), 0644)
}

func compressPNG(srcPath, dstPath string, img image.Image) error {
	// First try PNG with best compression
	var buffer bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	err := encoder.Encode(&buffer, img)
	if err != nil {
		return err
	}
	
	if buffer.Len() <= targetSize {
		return os.WriteFile(dstPath, buffer.Bytes(), 0644)
	}
	
	// If PNG is still too large, convert to JPEG
	jpegPath := strings.TrimSuffix(dstPath, filepath.Ext(dstPath)) + ".jpg"
	fmt.Printf("(converting to JPEG) ")
	return compressJPEG(jpegPath, img)
}

func compressGIF(srcPath, dstPath string, img image.Image) error {
	// For GIF, try to re-encode with default settings
	var buffer bytes.Buffer
	err := gif.Encode(&buffer, img, nil)
	if err != nil {
		return err
	}
	
	if buffer.Len() <= targetSize {
		return os.WriteFile(dstPath, buffer.Bytes(), 0644)
	}
	
	// If GIF is still too large, convert to JPEG
	jpegPath := strings.TrimSuffix(dstPath, filepath.Ext(dstPath)) + ".jpg"
	fmt.Printf("(converting to JPEG) ")
	return compressJPEG(jpegPath, img)
}

func compressHEIC(srcPath, dstPath string) error {
	// Since Go doesn't have native HEIC support, we'll show a message
	// In a production app, you'd use a tool like ImageMagick or libheif
	fmt.Printf("\nNote: HEIC format requires external tools for conversion.\n")
	fmt.Printf("To compress HEIC files, please convert them to JPEG first using:\n")
	fmt.Printf("  - macOS: Preview app or Photos app\n")
	fmt.Printf("  - Windows: HEIF Image Extensions from Microsoft Store\n")
	fmt.Printf("  - Command line: ImageMagick or libheif tools\n")
	return fmt.Errorf("HEIC compression not supported without external tools")
}

func recompressImage(filePath string) error {
	// Read the file to determine its format
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Decode the image
	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}
	file.Close()
	
	// Force JPEG compression with very low quality
	var buffer bytes.Buffer
	err = jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 5})
	if err != nil {
		return err
	}
	
	// If still too large, try scaling down the image
	if buffer.Len() > targetSize {
		// Scale down by 50%
		bounds := img.Bounds()
		newWidth := bounds.Dx() / 2
		newHeight := bounds.Dy() / 2
		
		// Create a scaled version (simple nearest neighbor for now)
		scaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		for y := 0; y < newHeight; y++ {
			for x := 0; x < newWidth; x++ {
				scaled.Set(x, y, img.At(x*2, y*2))
			}
		}
		
		// Try encoding the scaled image
		buffer.Reset()
		err = jpeg.Encode(&buffer, scaled, &jpeg.Options{Quality: 10})
		if err != nil {
			return err
		}
	}
	
	return os.WriteFile(filePath, buffer.Bytes(), 0644)
}