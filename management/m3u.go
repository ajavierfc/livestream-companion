package management

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type M3UCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	ParentID     int    `json:"parent_id"`
}

type M3uChannel struct {
	Num               int      `json:"num"`
	Name              string   `json:"name"`
	StreamType        string   `json:"stream_type"`
	StreamID          int      `json:"stream_id"`
	StreamIcon        string   `json:"stream_icon"`
	EpgChannelID      string   `json:"epg_channel_id"`
	Added             string   `json:"added"`
	CustomSid         string   `json:"custom_sid"`
	TvArchive         int      `json:"tv_archive"`
	DirectSource      string   `json:"direct_source"`
	TvArchiveDuration int      `json:"tv_archive_duration"`
	CategoryID        string   `json:"category_id"`
	CategoryIds       []string `json:"category_ids"`
	Thumbnail         string   `json:"thumbnail"`
	StreamURL         string   `json:"stream_url"`
}

func getM3uData(url string, playlistID uint) (*bufio.Scanner, io.Closer, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	path, size, err := saveImportSnapshot(playlistID, "source.m3u", resp.Body)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("[playlist %d] Saved M3U source (%d bytes) to %s", playlistID, size, path)

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open saved M3U source: %w", err)
	}
	return bufio.NewScanner(file), file, nil
}

func M3uCategoryHandler(c *gin.Context) {
	playlistIDStr := c.Param("playlistID") // This gets the playlistID from the URL

	playlistIDUint64, err := strconv.ParseUint(playlistIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
		log.Print(err)
		return
	}
	playlistID := uint(playlistIDUint64)

	playlist, err := GetPlaylistByID(playlistID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get playlist by ID"})
		log.Print(err)
		return
	}

	categories := make(map[string]string)
	scanner, sourceFile, err := getM3uData(playlist.M3uURL, playlist.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get data from M3U URL"})
		log.Print(err)
		return
	}
	defer sourceFile.Close()

	categoryRegexp := regexp.MustCompile(`group-title="(.*?)"`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := categoryRegexp.FindStringSubmatch(line)

		if len(matches) > 1 {
			category := matches[1]
			if _, ok := categories[category]; !ok {
				id := strings.ReplaceAll(category, " ", "_")
				categories[category] = id
			}
		}
	}
	if err := scanner.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read M3U data"})
		log.Print(err)
		return
	}

	result := []M3UCategory{}
	for k, v := range categories {
		result = append(result, M3UCategory{CategoryID: v, CategoryName: k, ParentID: 0})
	}

	c.JSON(http.StatusOK, result)
}

func M3uChannelHandler(c *gin.Context) {
	playlistIDStr := c.Param("playlistID")

	playlistIDUint64, err := strconv.ParseUint(playlistIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
		log.Print(err)
		return
	}
	playlistID := uint(playlistIDUint64)

	playlist, err := GetPlaylistByID(playlistID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get playlist by ID"})
		log.Print(err)
		return
	}

	channels := make([]M3uChannel, 0)
	scanner, sourceFile, err := getM3uData(playlist.M3uURL, playlist.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get data from M3U URL"})
		log.Print(err)
		return
	}
	defer sourceFile.Close()

	channelRegexp := regexp.MustCompile(`#EXTINF:-1 tvg-name="(.*?)" tvg-logo="(.*?)" group-title="(.*?)"`)

	var streamURL string
	for scanner.Scan() {
		line := scanner.Text()
		channelMatches := channelRegexp.FindStringSubmatch(line)

		if len(channelMatches) > 1 {
			streamURL = ""
			if scanner.Scan() {
				streamURL = scanner.Text()
			}
			channels = append(channels, M3uChannel{
				Num:        len(channels) + 1,
				Name:       channelMatches[1],
				StreamType: "live",
				StreamID:   len(channels) + 1,
				StreamIcon: channelMatches[2],
				CategoryID: strings.ReplaceAll(channelMatches[3], " ", "_"),
				CategoryIds: []string{
					channelMatches[3],
				},
				StreamURL: streamURL,
			})
			if len(channels)%500 == 0 {
				log.Printf("[playlist %d] Parsed %d channels from M3U source", playlist.ID, len(channels))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read M3U data"})
		log.Print(err)
		return
	}
	log.Printf("[playlist %d] Parsed %d channels from M3U source", playlist.ID, len(channels))

	c.JSON(http.StatusOK, channels)
}
