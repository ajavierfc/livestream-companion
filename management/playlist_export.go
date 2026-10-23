package management

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ExportPlaylistM3U8Handler(c *gin.Context) {
	playlistIDParam := strings.TrimSuffix(c.Param("playlistID"), ".m3u8")
	playlistID, err := strconv.ParseUint(playlistIDParam, 10, 32)
	if err != nil || playlistID == 0 {
		c.String(http.StatusBadRequest, "Invalid playlist ID")
		return
	}

	var playlist Playlist
	if err := DB.Select("id").First(&playlist, uint(playlistID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(http.StatusNotFound, "Playlist not found")
		} else {
			c.String(http.StatusInternalServerError, "Unable to load playlist")
		}
		return
	}

	var channels []Channel
	if err := DB.Joins("JOIN categories ON categories.id = channels.category_id").
		Where("categories.playlist_id = ? AND categories.active = ? AND channels.name NOT LIKE ?",
			uint(playlistID), true, "###%").
		Preload("Category").
		Order("categories.num ASC, channels.hdhr_channel_num ASC, channels.id ASC").
		Find(&channels).Error; err != nil {
		c.String(http.StatusInternalServerError, "Unable to load playlist channels")
		return
	}

	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	if forwardedProto := c.GetHeader("X-Forwarded-Proto"); forwardedProto == "http" || forwardedProto == "https" {
		scheme = forwardedProto
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, c.Request.Host)

	var playlistBody strings.Builder
	playlistBody.WriteString("#EXTM3U\n")
	for _, channel := range channels {
		name := cleanM3UValue(channel.Name)
		categoryName := cleanM3UValue(channel.Category.CategoryName)
		logo := cleanM3UValue(channel.StreamIcon)
		fmt.Fprintf(&playlistBody, "#EXTINF:-1 tvg-id=\"%s\" tvg-name=\"%s\" tvg-logo=\"%s\" group-title=\"%s\",%s\n",
			cleanM3UValue(channel.EpgChannelID), name, logo, categoryName, name)
		fmt.Fprintf(&playlistBody, "%s/hls/%d.ts\n", baseURL, channel.ID)
	}

	c.Header("Content-Type", "application/vnd.apple.mpegurl; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"playlist-%d.m3u8\"", playlistID))
	c.String(http.StatusOK, playlistBody.String())
}

func cleanM3UValue(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "\"", "'")
}
