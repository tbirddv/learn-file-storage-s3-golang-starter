package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	userID, err := cfg.getLoggedInUserID(r)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error(), err)
		return
	}

	video, err := cfg.getVideofromDBandConfirmAccess(videoID, userID)
	if err != nil {
		if err.Error() == "couldn't find video" {
			respondWithError(w, http.StatusNotFound, err.Error(), err)
			return
		}
		if err.Error() == "you don't have permission to access this video" {
			respondWithError(w, http.StatusUnauthorized, err.Error(), err)
			return
		}
	}

	const maxMemory = 10 << 20 // 10 MB

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse form", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get thumbnail", err)
		return
	}
	defer file.Close()

	fileType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if !strings.HasPrefix(fileType, "image/") {
		respondWithError(w, http.StatusBadRequest, "Invalid content type", nil)
		return
	}
	fileExtension := fileType[strings.LastIndex(fileType, "/")+1:]
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random bytes", err)
		return
	}
	randomString := base64.RawURLEncoding.EncodeToString(randomBytes)

	localfileaddress := filepath.Join(cfg.assetsRoot, randomString+"."+fileExtension)

	localfile, err := os.Create(localfileaddress)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save thumbnail", err)
		return
	}
	defer localfile.Close()

	io.Copy(localfile, file)

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)
	fmt.Println("Saving thumbnail to", localfileaddress)

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, randomString, fileExtension)
	fmt.Println("Thumbnail URL:", thumbnailURL)
	video.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
