package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	tDirName     = "translator"
	tJsonPath    = "/master/translation.en.json"
	tImportsFile = "translationImports.ts"
)

type Key struct {
	Name     string
	FilePath string
}

func main() {
	args := os.Args
	allKeys := []Key{}
	presumablyUnusedKeys := []Key{}
	unUsedKeys := []Key{}

	err := filepath.WalkDir(args[1], func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() && d.Name() == tDirName {
			jsonFilePath := path + tJsonPath

			if _, err := os.Stat(jsonFilePath); err == nil {
				jsonFile, err := os.Open(jsonFilePath)
				if err != nil {
					log.Fatal(err)
				}

				defer jsonFile.Close()

				byteValue, _ := io.ReadAll(jsonFile)

				var result map[string]interface{}
				json.Unmarshal([]byte(byteValue), &result)

				for k := range result {
					if !strings.Contains(k, "_plural") {
						allKeys = append(allKeys, Key{k, jsonFilePath})
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(allKeys))

	for i := range allKeys {
		go checkKeyUsage(allKeys[i], strings.ReplaceAll(allKeys[i].FilePath, fmt.Sprintf("/%s%s", tDirName, tJsonPath), ""), &presumablyUnusedKeys, &wg)
	}

	wg.Wait()

	if len(presumablyUnusedKeys) > 0 {
		keysInPackages := []Key{}
		wg := sync.WaitGroup{}

		for _, key := range presumablyUnusedKeys {
			if strings.Contains(key.FilePath, "packages") {
				keysInPackages = append(keysInPackages, key)
			}
		}

		wg.Add(len(keysInPackages))

		for _, key := range keysInPackages {
			go checkKeyUsage(key, args[1], &unUsedKeys, &wg)
		}

		wg.Wait()
	}

	JSON, _ := json.MarshalIndent(allKeys, "", "\t")
	_ = os.WriteFile("all_keys.json", JSON, 0644)

	JSON, _ = json.MarshalIndent(unUsedKeys, "", "\t")
	_ = os.WriteFile("unused_keys.json", JSON, 0644)

	formatted := fmt.Sprintf("Total keys: %d\nUnused keys: %d", len(allKeys), len(unUsedKeys))
	fmt.Println(formatted)
}

func checkKeyUsage(key Key, pathToLookUp string, unUsedKeys *[]Key, wg *sync.WaitGroup) {
	defer wg.Done()

	isUsed := false
	err := filepath.WalkDir(pathToLookUp, func(path string, d fs.DirEntry, err error) error {
		fileExt := filepath.Ext(path)

		if (fileExt == ".ts" || fileExt == ".tsx") && d.Name() != tImportsFile {
			f, err := os.Open(path)
			if err != nil {
				log.Fatal(err)
			}

			defer f.Close()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				if strings.Contains(scanner.Text(), key.Name) {
					isUsed = true
					break
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	if !isUsed {
		*unUsedKeys = append(*unUsedKeys, key)
	}
}
