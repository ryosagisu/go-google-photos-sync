package main

import (
	"flag"
	"fmt"
	"google-photo-sync/configs"
	"google-photo-sync/internal"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/api/photoslibrary/v1"
)

var (
	client *http.Client
)

const (
	SyncImage = "SyncImage"
	ListAlbum = "ListAlbum"
)

func main() {
	log.Println("Starting go-google-photo-sync")
	configFlag := flag.String("config", "/configs/config.yml", "path to config.yml")
	commandFlag := flag.String("command", SyncImage, "command to execute")
	flag.Parse()

	cfg := configs.ReadConfig(*configFlag)
	svc := internal.GetService(cfg)

	client = &http.Client{
		Timeout: 5 * time.Minute,
	}

	switch *commandFlag {
	case SyncImage:
		syncImage(cfg, svc)
	case ListAlbum:
		listAlbum(svc)
	}
	log.Println("Good bye...")
}

func listLocalImages(cfg *configs.Config) map[string]bool {
	imageFiles := make(map[string]bool)
	files, err := ioutil.ReadDir(cfg.OutputPath)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		if file.IsDir() || file.Size() == 0 {
			continue
		}

		filePath := fmt.Sprintf("%s/%s", cfg.OutputPath, file.Name())
		if !isValidImage(filePath) {
			continue
		}
		imageFiles[strings.TrimSuffix(file.Name(), ".jpg")] = true
	}
	return imageFiles
}

func isValidImage(filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("failed to open image: %v\n", err)
		return false
	}
	defer f.Close()

	buff := make([]byte, 512)
	if _, err := f.Read(buff); err != nil {
		log.Printf("failed to read image: %v\n", err)
		return false
	}

	return http.DetectContentType(buff) == "image/jpeg"
}

func syncImage(cfg *configs.Config, svc *photoslibrary.Service) {
	localImages := listLocalImages(cfg)
	var pageToken string
	log.Println("Downloading images...")
	for {
		req := &photoslibrary.SearchMediaItemsRequest{
			PageSize:  100,
			AlbumId:   cfg.AlbumId,
			PageToken: pageToken,
		}

		items, err := svc.MediaItems.Search(req).Do()
		if err != nil {
			log.Fatalf("failed to search media: %v", err)
		}

		pageToken = items.NextPageToken
		for _, item := range items.MediaItems {
			fileName := getImageName(item.Id, cfg.OutputPath)
			if localImages[item.Id] {
				localImages[item.Id] = false
			}

			err = downloadImage(fileName, item.BaseUrl)
			if err != nil {
				log.Printf("Failed to download: %v\n", err)
			}
		}

		if pageToken == "" {
			break
		}
	}

	log.Println("Delete missing images...")
	for k, v := range localImages {
		if v {
			deleteLocalFile(k, cfg.OutputPath)
		}
	}
}

func deleteLocalFile(filename, outputPath string) {
	path := getImageName(filename, outputPath)
	err := os.Remove(path)
	if err != nil {
		log.Printf("failed to delete local files: %v\n", err)
		return
	}
	log.Printf("%s deleted\n", filename)
}

func getImageName(id, outputPath string) string {
	return fmt.Sprintf("%s/%s.jpg", outputPath, id)
}

// Skip download if file exist
func downloadImage(fileName, baseUrl string) error {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		url := fmt.Sprintf("%v=d", baseUrl)
		output, err := os.Create(fileName)
		if err != nil {
			return err
		}
		defer output.Close()

		response, err := client.Get(url)
		if err != nil {
			return err
		}
		defer response.Body.Close()

		n, err := io.Copy(output, response.Body)
		if err != nil {
			return err
		}
		log.Printf("Downloaded '%v' (%v)", fileName, uint64(n))
	}
	return nil
}

func listAlbum(svc *photoslibrary.Service) {
	albums, err := svc.Albums.List().Do()
	if err != nil {
		log.Fatalf("failed to list album: %v", err)
	}

	for _, album := range albums.Albums {
		log.Println(album.Id, album.Title)
	}
}
