package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type PexelsPhotoResponse struct {
	Photos []struct {
		Src struct {
			Large2x string `json:"large2x"`
		} `json:"src"`
		Photographer string `json:"photographer"`
	} `json:"photos"`
}

// FetchStockImages ищет изображения на Pexels и скачивает до 5 штук.
func FetchStockImages(apiKey, query string) ([]string, error) {
	u, _ := url.Parse("https://api.pexels.com/v1/search")
	q := u.Query()
	q.Set("query", query)
	q.Set("per_page", "5")
	q.Set("orientation", "portrait")
	u.RawQuery = q.Encode()

	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("Authorization", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels images: %w", err)
	}
	defer resp.Body.Close()

	var pexResp PexelsPhotoResponse
	if err := json.NewDecoder(resp.Body).Decode(&pexResp); err != nil {
		return nil, fmt.Errorf("pexels image decode: %w", err)
	}

	var files []string
	os.MkdirAll("backgrounds", 0755)
	for i, photo := range pexResp.Photos {
		imgResp, err := http.Get(photo.Src.Large2x)
		if err != nil {
			continue
		}
		defer imgResp.Body.Close()
		filename := filepath.Join("backgrounds", fmt.Sprintf("image_%s_%d.jpg", sanitizeFilename(query), i))
		file, err := os.Create(filename)
		if err != nil {
			continue
		}
		io.Copy(file, imgResp.Body)
		file.Close()
		files = append(files, filename)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("нет изображений")
	}
	return files, nil
}
