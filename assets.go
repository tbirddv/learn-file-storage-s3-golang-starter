package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"

	"github.com/google/uuid"

	"github.com/tbirddv/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/tbirddv/learn-file-storage-s3-golang-starter/internal/database"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func (cfg apiConfig) getLoggedInUserID(r *http.Request) (uuid.UUID, error) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		return uuid.Nil, errors.New("couldn't find JWT")
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		return uuid.Nil, errors.New("couldn't validate JWT")
	}
	return userID, nil
}

func (cfg apiConfig) getVideofromDBandConfirmAccess(videoID uuid.UUID, userID uuid.UUID) (database.Video, error) {
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		return database.Video{}, errors.New("couldn't find video")
	}

	if video.UserID != userID {
		return database.Video{}, errors.New("you don't have permission to access this video")
	}
	return video, nil
}

type ffProbeStream struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	ffprobe := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var output bytes.Buffer
	ffprobe.Stdout = &output
	err := ffprobe.Run()
	if err != nil {
		return "", err
	}

	var ffprobeData ffProbeStream
	err = json.Unmarshal(output.Bytes(), &ffprobeData)
	if err != nil {
		return "", err
	}

	if len(ffprobeData.Streams) == 0 {
		return "", errors.New("no video streams found")
	}

	width := ffprobeData.Streams[0].Width
	height := ffprobeData.Streams[0].Height
	if int(width/16) == int(height/9) {
		return "16:9", nil
	} else if int(width/9) == int(height/16) {
		return "9:16", nil
	} else {
		return "other", nil
	}
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"
	ffmpeg := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	err := ffmpeg.Run()
	if err != nil {
		return "", err
	}
	return outputFilePath, nil
}
