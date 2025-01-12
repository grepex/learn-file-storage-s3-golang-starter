package main

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20

	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// get mediatype from file's content-type header
	contentTypes := header.Header["Content-Type"]
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to parse content type header", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "file must be either jpeg or png format", err)
		return
	}

	// get video metadata from db
	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to fetch video metadata", err)
		return
	}

	// check fi user id lines up
	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User does not have permissions to modify selected video", errors.New("unauthorized user"))
		return
	}

	// save thumbnail to global map
	// tn := thumbnail{
	// 	data:      data,
	// 	mediaType: mediaType,
	// }

	// videoThumbnails[metadata.ID] = tn

	// update db so video record has a new thumbnail url

	// create base64 encoded image
	// b64Data := base64.StdEncoding.EncodeToString(data)
	//
	// dataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, b64Data)

	extensions, err := mime.ExtensionsByType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to determine file extension", err)
	}
	extension := extensions[0]

	fileName := fmt.Sprintf("%s.%s", metadata.ID, extension)
	filePath := filepath.Join(cfg.assetsRoot, fileName)
	createdFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "error creating file for thumbnail in filesystem", err)
	}

	_, err = io.Copy(createdFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error writing uploaded file to filesystem", err)
	}
	tnURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)

	videoParams := database.CreateVideoParams{
		Title:       metadata.Title,
		Description: metadata.Description,
		UserID:      metadata.UserID,
	}

	video := database.Video{
		ID:                metadata.ID,
		CreateVideoParams: videoParams,
		ThumbnailURL:      &tnURL,
	}

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to push thumbnail to db", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
