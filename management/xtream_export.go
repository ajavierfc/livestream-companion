package management

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"livestream-companion/stream"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type xtreamLiveCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	ParentID     int    `json:"parent_id"`
}

type xtreamLiveStream struct {
	Num          int      `json:"num"`
	Name         string   `json:"name"`
	StreamType   string   `json:"stream_type"`
	StreamID     int      `json:"stream_id"`
	StreamIcon   string   `json:"stream_icon"`
	EpgChannelID string   `json:"epg_channel_id"`
	Added        string   `json:"added"`
	DirectSource string   `json:"direct_source"`
	CategoryID   string   `json:"category_id"`
	CategoryIDs  []string `json:"category_ids"`
}

func xtreamPlaylistID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("playlistID"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
		return 0, false
	}
	return uint(id), true
}

func XtreamPlayerAPIHandler(c *gin.Context) {
	playlistID, ok := xtreamPlaylistID(c)
	if !ok {
		return
	}
	var playlist Playlist
	if err := DB.Select("id").First(&playlist, playlistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load playlist"})
		}
		return
	}

	switch strings.ToLower(c.Query("action")) {
	case "", "get_user_info":
		scheme := requestScheme(c)
		c.JSON(http.StatusOK, gin.H{
			"user_info": gin.H{
				"username":               c.Query("username"),
				"password":               c.Query("password"),
				"message":                "",
				"auth":                   1,
				"status":                 "Active",
				"exp_date":               nil,
				"is_trial":               "0",
				"active_cons":            "0",
				"created_at":             "",
				"max_connections":        "0",
				"allowed_output_formats": []string{"ts", "m3u8"},
			},
			"server_info": gin.H{
				"url":             c.Request.Host,
				"server_protocol": scheme,
				"timezone":        "UTC",
			},
		})
	case "get_live_categories":
		var categories []Category
		if err := DB.Where("playlist_id = ? AND active = ?", playlistID, true).
			Order("num ASC, id ASC").Find(&categories).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load playlist categories"})
			return
		}
		result := make([]xtreamLiveCategory, 0, len(categories))
		for _, category := range categories {
			result = append(result, xtreamLiveCategory{
				CategoryID:   strconv.FormatUint(uint64(category.ID), 10),
				CategoryName: category.CategoryName,
				ParentID:     0,
			})
		}
		c.JSON(http.StatusOK, result)
	case "get_live_streams":
		var categoryID *uint
		if rawCategoryID := c.Query("category_id"); rawCategoryID != "" {
			parsed, err := strconv.ParseUint(rawCategoryID, 10, 32)
			if err != nil || parsed == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
				return
			}
			id := uint(parsed)
			categoryID = &id
		}
		var channels []Channel
		query := DB.Joins("JOIN categories ON categories.id = channels.category_id").
			Where("categories.playlist_id = ? AND categories.active = ? AND channels.name NOT LIKE ?",
				playlistID, true, "###%")
		if categoryID != nil {
			query = query.Where("categories.id = ?", *categoryID)
		}
		if err := query.Preload("Category").
			Order("categories.num ASC, channels.num ASC, channels.id ASC").
			Find(&channels).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load playlist channels"})
			return
		}
		username := url.PathEscape(c.Query("username"))
		password := url.PathEscape(c.Query("password"))
		result := make([]xtreamLiveStream, 0, len(channels))
		for index, channel := range channels {
			categoryID := strconv.FormatUint(uint64(channel.CategoryID), 10)
			directSource := ""
			if username != "" && password != "" {
				directSource = fmt.Sprintf("/live/%s/%s/%d.ts", username, password, channel.ID)
			}
			result = append(result, xtreamLiveStream{
				Num:          index + 1,
				Name:         channel.Name,
				StreamType:   "live",
				StreamID:     channel.ID,
				StreamIcon:   channel.StreamIcon,
				EpgChannelID: channel.EpgChannelID,
				Added:        strconv.FormatInt(channel.CreatedAt.Unix(), 10),
				DirectSource: directSource,
				CategoryID:   categoryID,
				CategoryIDs:  []string{categoryID},
			})
		}
		c.JSON(http.StatusOK, result)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported Xtream API action"})
	}
}

func XtreamLiveStreamHandler(c *gin.Context) {
	serveXtreamLiveStream(c, c.Param("streamPath"), nil)
}

func XtreamPlaylistLiveStreamHandler(c *gin.Context) {
	playlistID, ok := xtreamPlaylistID(c)
	if !ok {
		return
	}
	serveXtreamLiveStream(c, c.Param("streamPath"), &playlistID)
}

func serveXtreamLiveStream(c *gin.Context, streamPath string, playlistID *uint) {
	streamIDPart := strings.Trim(streamPath, "/")
	if slash := strings.LastIndexByte(streamIDPart, '/'); slash >= 0 {
		streamIDPart = streamIDPart[slash+1:]
	}
	extension := ""
	if dot := strings.LastIndexByte(streamIDPart, '.'); dot >= 0 {
		extension = strings.ToLower(streamIDPart[dot:])
		streamIDPart = streamIDPart[:dot]
	}
	streamID, err := strconv.Atoi(streamIDPart)
	if err != nil || streamID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid stream ID"})
		return
	}

	query := DB.Joins("JOIN categories ON categories.id = channels.category_id").
		Where("channels.id = ? AND categories.active = ?", streamID, true)
	if playlistID != nil {
		query = query.Where("categories.playlist_id = ?", *playlistID)
	}
	var channel Channel
	if err := query.First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Stream not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load stream"})
		}
		return
	}

	switch extension {
	case "", ".ts":
		stream.HandleTS(c, channel.StreamURL, strconv.Itoa(channel.ID), "false")
	case ".m3u8":
		stream.HandleM3U8(c, channel.StreamURL, strconv.Itoa(channel.ID))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported stream format"})
	}
}

func requestScheme(c *gin.Context) string {
	if forwardedProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); strings.EqualFold(forwardedProto, "http") ||
		strings.EqualFold(forwardedProto, "https") {
		return strings.ToLower(forwardedProto)
	}
	if c.Request.TLS != nil {
		return "https"
	}
	return "http"
}
