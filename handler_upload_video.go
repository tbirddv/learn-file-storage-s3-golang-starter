package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"github.com/tbirddv/learn-file-storage-s3-golang-starter/internal/database"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
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

	var video database.Video
	video, err = cfg.getVideofromDBandConfirmAccess(videoID, userID)
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

	const maxMemory = 10 << 30 // 1 GB

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse form", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get video", err)
		return
	}
	defer file.Close()

	fileType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if fileType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid content type", nil)
		return
	}
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error Uploading Video", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	tempFilePath := tempFile.Name()
	fmt.Println("Saving video to", tempFilePath)

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error Uploading Video", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error Getting Video Aspect Ratio", err)
		return
	}

	processedFilePath, err := processVideoForFastStart(tempFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error Processing Video", err)
		return
	}

	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error Opening Processed Video", err)
		return
	}
	defer os.Remove(processedFilePath) // Clean up processed file after upload
	defer processedFile.Close()

	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random bytes", err)
		return
	}
	var key string
	switch aspectRatio {
	case "16:9":
		key = fmt.Sprintf("landscape/%s.mp4", base64.RawURLEncoding.EncodeToString(randomBytes))
	case "9:16":
		key = fmt.Sprintf("portrait/%s.mp4", base64.RawURLEncoding.EncodeToString(randomBytes))
	default:
		key = fmt.Sprintf("other/%s.mp4", base64.RawURLEncoding.EncodeToString(randomBytes))
	}

	fmt.Println("uploading mp4 for video", videoID, "by user", userID)

	_, err = cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        processedFile,
		ContentType: aws.String(fileType),
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error Uploading Video", err)
		return
	}

	distroURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, key)
	video.VideoURL = &distroURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error Updating Video", err)
		return
	}

	respondWithJSON(w, http.StatusCreated, video)
}
