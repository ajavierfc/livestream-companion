package management

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CategoryInfo struct {
	ID           string
	ExternalID   string
	CategoryName string
}

func ImportXtream(c *gin.Context, ID uint) {
	playlist, err := GetPlaylistByID(ID)
	if err != nil {
		log.Print(err)
		return
	}

	newPlaylist := playlist.ImportStatus == 0

	startTime := time.Now()
	playlist.ImportStatus = 1
	err = playlist.Update()
	if err != nil {
		log.Print(err)
		return
	}

	baseURL := playlist.Server
	username := playlist.Username
	password := playlist.Password

	scheme := "http"
	if forwardedProto := c.GetHeader("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	} else if c.Request.TLS != nil {
		scheme = "https"
	}

	var categoryListURL string
	if playlist.Type == "m3u" {
		categoryListURL = fmt.Sprintf("%s://%s/api/m3u/categories/%d", scheme, c.Request.Host, ID)
	} else {
		categoryListURL = fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_live_categories", baseURL, username, password)
	}

	categoryListResp, err := http.Get(categoryListURL)
	if err != nil {
		log.Print(err)
		playlist.ImportStatus = -1
		playlist.Update()
		return
	}
	defer categoryListResp.Body.Close()

	categoriesBody, err := io.ReadAll(categoryListResp.Body)
	if err != nil {
		log.Print(err)
		playlist.ImportStatus = -1
		playlist.Update()
		return
	}

	var categoriesResponse []Category
	if err := json.Unmarshal(categoriesBody, &categoriesResponse); err != nil {
		log.Print(err)
		playlist.ImportStatus = -1
		playlist.Update()
		return
	}

	var streamListURL string
	if playlist.Type == "m3u" {
		streamListURL = fmt.Sprintf("%s://%s/api/m3u/channels/%d", scheme, c.Request.Host, ID)
	} else {
		streamListURL = fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_live_streams", baseURL, username, password)
	}

	streamListResp, err := http.Get(streamListURL)
	if err != nil {
		log.Print(err)
		playlist.ImportStatus = -1
		playlist.Update()
		return
	}
	defer streamListResp.Body.Close()

	streamsBody, err := io.ReadAll(streamListResp.Body)
	if err != nil {
		log.Print(err)
		playlist.ImportStatus = -1
		playlist.Update()
		return
	}

	var channelsResponse []Channel
	if err := json.Unmarshal(streamsBody, &channelsResponse); err != nil {
		log.Print(err)
		playlist.ImportStatus = -1
		playlist.Update()
		return
	}

	var categoriesToCreate []Category
	var categoriesToUpdate []Category
	var channelsToCreate []Channel
	var channelsToUpdate []Channel
	var hdhrChannelNum = 1000
	var batchSize = 500

	for i, category := range categoriesResponse {
		var dbCategory Category
		var err error
		if !newPlaylist {
			err = DB.Model(Category{}).Where("external_id = ? and playlist_id = ?", category.ExternalID, playlist.ID).First(&dbCategory).Error
		}
		if newPlaylist || err == gorm.ErrRecordNotFound {
			dbCategory.ExternalID = category.ExternalID
			dbCategory.CategoryName = category.CategoryName
			dbCategory.Num = i
			dbCategory.PlaylistID = playlist.ID
			dbCategory.Active = false
			categoriesToCreate = append(categoriesToCreate, dbCategory)
			if len(categoriesToCreate) == batchSize {
				DB.CreateInBatches(categoriesToCreate, batchSize)
				categoriesToCreate = categoriesToCreate[:0]
			}
		} else {
			dbCategory.CategoryName = category.CategoryName
			dbCategory.Num = i
			categoriesToUpdate = append(categoriesToUpdate, dbCategory)
			if len(categoriesToUpdate) == batchSize {
				for _, category := range categoriesToUpdate {
					DB.Save(&category)
				}
				categoriesToUpdate = categoriesToUpdate[:0]
			}
		}

	}

	// Handle remaining categories
	DB.CreateInBatches(categoriesToCreate, len(categoriesToCreate))
	for _, category := range categoriesToUpdate {
		DB.Save(&category)
	}

	// Delete all channels with StreanID = "-1" (Categories fake channels)
	DB.Where("stream_id = ?", -1).Delete(&Channel{})

	// Get all categories from the database
	var categories []Category
	result := DB.Find(&categories)
	if result.Error != nil {
		log.Fatal(result.Error)
	}

	for _, channel := range channelsResponse {
		var category Category
		var dbChannel Channel
		var err error = DB.Model(Category{}).Where("external_id = ? and playlist_id = ?", channel.ExternalCategoryID, playlist.ID).First(&category).Error
		if err == gorm.ErrRecordNotFound {
			log.Print(err)
			continue
		}
		if !newPlaylist {
			err = DB.Model(Channel{}).Where("stream_id = ? and external_category_id = ?", channel.StreamID, channel.ExternalCategoryID).First(&dbChannel).Error
		}
		if newPlaylist || err == gorm.ErrRecordNotFound {
			dbChannel.Num = channel.Num
			dbChannel.Name = channel.Name
			dbChannel.CategoryID = category.ID
			dbChannel.ExternalCategoryID = channel.ExternalCategoryID
			dbChannel.StreamID = channel.StreamID
			if playlist.Type == "m3u" {
				dbChannel.StreamURL = channel.StreamURL
			} else {
				dbChannel.StreamURL = fmt.Sprintf("%s/live/%s/%s/%d.m3u8", baseURL, username, password, channel.StreamID)
			}
			dbChannel.EpgChannelID = channel.EpgChannelID
			dbChannel.HDHRChannelNum = hdhrChannelNum
			dbChannel.StreamIcon = channel.StreamIcon
			dbChannel.Active = true
			channelsToCreate = append(channelsToCreate, dbChannel)
			if len(channelsToCreate) == batchSize {
				DB.CreateInBatches(channelsToCreate, batchSize)
				channelsToCreate = channelsToCreate[:0]
			}
			hdhrChannelNum++
		} else {
			dbChannel.Num = channel.Num
			dbChannel.Name = channel.Name
			dbChannel.CategoryID = category.ID
			dbChannel.ExternalCategoryID = channel.ExternalCategoryID
			dbChannel.StreamID = channel.StreamID
			if playlist.Type == "m3u" {
				dbChannel.StreamURL = channel.StreamURL
			} else {
				dbChannel.StreamURL = fmt.Sprintf("%s/live/%s/%s/%d.m3u8", baseURL, username, password, channel.StreamID)
			}
			dbChannel.EpgChannelID = channel.EpgChannelID
			dbChannel.HDHRChannelNum = hdhrChannelNum
			dbChannel.StreamIcon = channel.StreamIcon
			channelsToUpdate = append(channelsToUpdate, dbChannel)
			if len(channelsToUpdate) == batchSize {
				for _, channel := range channelsToUpdate {
					DB.Save(&channel)
				}
				channelsToUpdate = channelsToUpdate[:0]
			}
			hdhrChannelNum++
		}

	}

	// Handle remaining channels
	DB.CreateInBatches(channelsToCreate, len(channelsToCreate))
	for _, channel := range channelsToUpdate {
		DB.Save(&channel)
	}

	DB.Delete(&Category{}, "playlist_id = ? and updated_at < ?", ID, startTime)
	DB.Where("category_id IN (SELECT id FROM categories WHERE playlist_id = ?) AND updated_at < ?", ID, startTime).Delete(&Channel{})

	UpdateHDHRChannelNumForAllChannels()
	playlist.ImportStatus = 2
	err = playlist.Update()
	if err != nil {
		log.Print(err)
		return
	}

	err = UpdatePlaylistEPG(*playlist)
	if err != nil {
		log.Print(err)
		return
	}

	log.Printf("Imported playlist %d in %s", playlist.ID, time.Since(startTime))
}
