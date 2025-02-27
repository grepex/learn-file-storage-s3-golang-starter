package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<30)

	// Extract video ID
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Authenticate user
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// Get video metadata
	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to fetch video metadata", err)
		return
	}

	// If user is not video owner, return status unauthorized
	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User does not have permissions to modify selected video", errors.New("unauthorized user"))
		return
	}

	// Parse uploaded video file from the form
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Get media type and validate it
	contentTypes := header.Header["Content-Type"]
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to parse content type header", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "file must be mp4", err)
		return
	}

	// Preprocess video with ffmpeg
	// processedFilePath, err := processVideoForFastStart("tubely_upload.mp4")
	// if err != nil {
	// 	respondWithError(w, http.StatusBadRequest, "Unable to preprocess video for fast start", err)
	// 	return
	// }
	// os.Remove()

	// Save uploaded file to temporary file on disk
	tempFile, err := os.CreateTemp("", "tubely_upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create temporary file for upload", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to copy file", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to reset file pointer", err)
		return
	}

	// Get aspect ratio for filename
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video aspect ratio", err)
		return
	}

	var orientation string

	if aspectRatio == "16:9" {
		orientation = "landscape/"
	} else if aspectRatio == "9:16" {
		orientation = "portrait/"
	} else {
		orientation = "other/"
	}

	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to generate random byte string for file name", err)
		return
	}

	randomBytesHex := hex.EncodeToString(randomBytes)

	fileKey := orientation + randomBytesHex + ".mp4"

	// Preprocess video with ffmpeg
	preprocessedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to preprocess video for fast start", err)
		return
	}

	// clean up temp file
	tempFile.Close()
	os.Remove(tempFile.Name())

	// open new file and defer cleanup
	preprocessedFile, err := os.Open(preprocessedFilePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to load preprocessed file", err)
		return
	}
	defer preprocessedFile.Close()
	defer os.Remove(preprocessedFilePath)

	params := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileKey,
		Body:        preprocessedFile,
		ContentType: &mediaType,
	}

	_, err = cfg.s3Client.PutObject(r.Context(), &params)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to store object in S3", err)
		return
	}

	// Update VideoURL

	// videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileKey)
	videoURL := cfg.s3Bucket + "," + fileKey
	metadata.VideoURL = &videoURL

	cfg.db.UpdateVideo(metadata)

	fmt.Println("Converting url to signed url...")
	metadata, err = cfg.dbVideoToSignedVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to convert video url to signed url", err)
	}

	respondWithJSON(w, http.StatusOK, metadata)
}
