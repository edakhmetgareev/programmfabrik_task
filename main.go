package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
)

type TagInfo struct {
	XMLName xml.Name `xml:"taginfo"`
	Tables  []Table  `xml:"table"`
}

type Table struct {
	Name string `xml:"name,attr"`
	Tags []Tag  `xml:"tag"`
}

type Tag struct {
	ID          string `xml:"id,attr"`
	Name        string `xml:"name,attr"`
	Type        string `xml:"type,attr"`
	Writable    string `xml:"writable,attr"`
	Description struct {
		EN string `xml:"en"`
		DE string `xml:"de"`
		ES string `xml:"es"`
		IT string `xml:"it"`
	} `xml:"desc"`
}

func main() {
	fmt.Println("Starting server on port 8080")
	http.HandleFunc("/tags", handleTags)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Printf("Error serving server: %v", err)
		os.Exit(1)
	}
}

func getExifTags(ctx context.Context) (*TagInfo, error) {
	cmd := exec.CommandContext(ctx, "exiftool", "-listx")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var tagInfo TagInfo
	err = xml.Unmarshal(output, &tagInfo)
	if err != nil {
		return nil, err
	}

	return &tagInfo, nil
}

func getTagData(tag Tag, tableName string) map[string]interface{} {
	tagPath := fmt.Sprintf("%s:%s", tableName, tag.Name)
	tagDescription := map[string]string{
		"en": tag.Description.EN,
		"de": tag.Description.DE,
		"es": tag.Description.ES,
		"it": tag.Description.IT,
	}

	tagData := map[string]interface{}{
		"writable":    tag.Writable == "true",
		"path":        tagPath,
		"group":       tableName,
		"description": tagDescription,
		"type":        tag.Type,
	}

	return tagData
}

func handleTags(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// Cancel exiftool command if client disconnects
		<-r.Context().Done()
		cancel()
	}()

	tagInfo, err := getExifTags(ctx)
	if err != nil {
		http.Error(w, "Error while getting tags", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)

	var chanSize int64
	for _, table := range tagInfo.Tables {
		chanSize += int64(len(table.Tags))
	}
	tagChan := make(chan map[string]interface{}, chanSize)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := encoder.Encode(map[string][]map[string]interface{}{"tags": getTagDataSlice(tagChan)}); err != nil {
			fmt.Printf("Error encoding JSON: %v \n", err)
		}
	}()

	// Send tag data to the channel
	for _, table := range tagInfo.Tables {
		for _, tag := range table.Tags {
			tagData := getTagData(tag, table.Name)
			tagChan <- tagData
		}
	}
	close(tagChan)

	wg.Wait()
}

func getTagDataSlice(tagChan <-chan map[string]interface{}) []map[string]interface{} {
	var tags []map[string]interface{}
	for tagData := range tagChan {
		tags = append(tags, tagData)
	}
	return tags
}
