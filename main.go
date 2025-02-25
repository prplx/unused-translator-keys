package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	translatorDirName   = "translator"
	translationFilePart = "translations/translation.en.json"
	excludedFile        = "translationImports.ts"
)

type Key struct {
	Name     string `json:"name"`
	FilePath string `json:"filePath"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: <program> <project_root_path>")
	}
	rootPath := os.Args[1]

	// 1. Gather all keys from translation.en.json files in translator directories.
	allKeys := gatherKeys(rootPath)
	if len(allKeys) == 0 {
		fmt.Println("Ключи не найдены")
		return
	}

	// 2. Initialize a map to track key usage.
	//    Key is the translation name, value is the Key structure.
	//    A separate usedMap stores whether the key was found in the project.
	keysMap := make(map[string]Key, len(allKeys))
	usedMap := make(map[string]bool, len(allKeys))
	for _, key := range allKeys {
		keysMap[key.Name] = key
		usedMap[key.Name] = false
	}

	// 3. Traverse through .ts and .tsx files in the project (excluding, for example, translationImports.ts)
	//    and search for key occurrences in each file.
	var mu sync.Mutex
	fileCh := make(chan string, 100)
	var wg sync.WaitGroup

	numWorkers := runtime.NumCPU()
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range fileCh {
				processFile(filePath, usedMap, &mu)
			}
		}()
	}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Handle only files with .ts and .tsx extensions
		if !d.IsDir() {
			ext := filepath.Ext(path)
			if (ext == ".ts" || ext == ".tsx") && !strings.HasSuffix(path, excludedFile) {
				fileCh <- path
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	close(fileCh)
	wg.Wait()

	// 4. Build a list of unused keys.
	var unusedKeys []Key
	for keyName, used := range usedMap {
		if !used {
			unusedKeys = append(unusedKeys, keysMap[keyName])
		}
	}

	// 5. Save all keys and unused keys to JSON files.
	writeJSON("all_keys.json", allKeys)
	writeJSON("unused_keys.json", unusedKeys)

	fmt.Printf("Total keys: %d\nUnused keys: %d\n", len(allKeys), len(unusedKeys))
}

// gatherKeys looks for translation.en.json files in translator directories and extracts keys from them.
func gatherKeys(rootPath string) []Key {
	var keys []Key

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// If the directory is a translator directory, look for translation.en.json file.
		if d.IsDir() && d.Name() == translatorDirName {
			jsonPath := filepath.Join(path, filepath.FromSlash(translationFilePart))
			if _, err := os.Stat(jsonPath); err == nil {
				file, err := os.Open(jsonPath)
				if err != nil {
					log.Printf("Ошибка открытия файла %s: %v", jsonPath, err)
					return nil
				}
				defer file.Close()

				data, err := io.ReadAll(file)
				if err != nil {
					log.Printf("Ошибка чтения файла %s: %v", jsonPath, err)
					return nil
				}

				var translations map[string]any
				if err := json.Unmarshal(data, &translations); err != nil {
					log.Printf("Ошибка разбора JSON файла %s: %v", jsonPath, err)
					return nil
				}

				for k := range translations {
					// Пропускаем ключи, содержащие "_plural"
					if !strings.Contains(k, "_plural") {
						keys = append(keys, Key{
							Name:     k,
							FilePath: jsonPath,
						})
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return keys
}

// processFile reads the file content and checks if any of the keys are present
func processFile(filePath string, usedMap map[string]bool, mu *sync.Mutex) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Ошибка чтения файла %s: %v", filePath, err)
		return
	}
	content := string(data)

	// Copy the keys to check to a separate slice to avoid locking the map for a long time.
	mu.Lock()
	var keysToCheck []string
	for k, used := range usedMap {
		if !used {
			keysToCheck = append(keysToCheck, k)
		}
	}
	mu.Unlock()

	// If all keys are used, return.
	if len(keysToCheck) == 0 {
		return
	}

	// For each key, check if it is present in the file content.
	for _, key := range keysToCheck {
		if strings.Contains(content, key) {
			mu.Lock()
			usedMap[key] = true
			mu.Unlock()
		}
	}
}

// writeJSON saves the data to a JSON file with the given filename.
func writeJSON(filename string, data any) {
	JSON, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		log.Printf("Ошибка маршалинге JSON для %s: %v", filename, err)
		return
	}
	if err := os.WriteFile(filename, JSON, 0644); err != nil {
		log.Printf("Ошибка записи файла %s: %v", filename, err)
	}
}
