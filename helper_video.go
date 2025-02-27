package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

type Stream struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type FFProbeOutput struct {
	Streams []Stream `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	fmt.Println("Initiate aspect ratio retrieval")
	fmt.Println("Forming command...")
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var out bytes.Buffer

	cmd.Stdout = &out

	fmt.Println("Running command...")
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error when running command")
		return "", err
	}

	var output FFProbeOutput

	if err := json.Unmarshal(out.Bytes(), &output); err != nil {
		return "", err
	}

	fmt.Printf("Width: %d\n", output.Streams[0].Width)
	fmt.Printf("Height: %d\n", output.Streams[0].Height)
	fmt.Println("Calculating ratio...")

	ratio := float64(output.Streams[0].Width) / float64(output.Streams[0].Height)
	fmt.Printf("Ratio: %.3f\n", ratio)

	if ratio > 1.7 && ratio < 1.8 {
		return "16:9", nil
	} else if ratio > 0.5 && ratio < 0.6 {
		return "9:16", nil
	} else {
		return "other", nil
	}
}

func processVideoForFastStart(filepath string) (string, error) {
	outputFilePath := filepath + ".processing"
	fmt.Println("Output filepath: " + outputFilePath)

	fmt.Println("Forming preprocess command...")
	cmd := exec.Command("ffmpeg", "-i", filepath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	fmt.Println("Running preprocess command...")
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error when running preprocess command")
		return "", err
	}
	fmt.Println("Sucessfully ran preprocess command...")

	return outputFilePath, nil

}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s3Client)

	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	request, err := presignClient.PresignGetObject(context.TODO(), params, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}

	return request.URL, err
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	fmt.Println("Starting dbVideoToSignedVideo...")
	fmt.Println("Bucketkey: " + *video.VideoURL)
	bucketKey := strings.Split(*video.VideoURL, ",")

	bucket := bucketKey[0]
	key := bucketKey[1]
	fmt.Printf("Separated bucket and key: %s, %s\n", bucket, key)

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 30*time.Minute)
	if err != nil {
		fmt.Println("Error when generating presigned url")
		return database.Video{}, err
	}
	fmt.Printf("Presigned url generated: %s\n", presignedURL)

	video.VideoURL = &presignedURL
	fmt.Printf("Assigned presigned url to video object url")

	return video, nil
}
