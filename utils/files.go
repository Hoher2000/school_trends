package utils

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func ProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func downloadImageWithTimeout(url, filepath string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		os.Remove(filepath)
		return fmt.Errorf("stat: %w", err)
	}
	if info.Size() < 1024 {
		os.Remove(filepath)
		return fmt.Errorf("file too small (%d bytes)", info.Size())
	}
	return nil
}

func DownloadImagesConcurrently(urls []string, prefix string, timeout time.Duration, maxConcurrent int) []string {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []string
		sem     = make(chan struct{}, maxConcurrent)
	)
	for i, imgURL := range urls {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			destFile := filepath.Join("backgrounds", fmt.Sprintf("%s_%d.jpg", prefix, idx))
			if err := downloadImageWithTimeout(url, destFile, timeout); err != nil {
				log.Printf("Не удалось скачать %s: %v", url, err)
				return
			}
			mu.Lock()
			results = append(results, destFile)
			mu.Unlock()
			log.Printf("Скачано (%d/%d): %s", len(results), len(urls), destFile)
		}(i, imgURL)
	}
	wg.Wait()
	return results
}
