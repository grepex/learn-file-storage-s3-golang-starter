package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

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

	// TODO: implement the upload here
	const maxMemory = 10 << 20

	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// get mediatype from file's content-type header
	mediaType := header.Header["Content-Type"][0]

	// read image data into a byte slice
	data, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read file", err)
	}

	// get video metadata from db
	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to fetch video metadata", err)
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
	// tnURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, metadata.ID)

	// create base64 encoded image
	b64Data := base64.StdEncoding.EncodeToString(data)

	dataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, b64Data)

	videoParams := database.CreateVideoParams{
		Title:       metadata.Title,
		Description: metadata.Description,
		UserID:      metadata.UserID,
	}

	video := database.Video{
		ID:                metadata.ID,
		CreateVideoParams: videoParams,
		ThumbnailURL:      &dataURL,
	}

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to push thumbnail to db", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
