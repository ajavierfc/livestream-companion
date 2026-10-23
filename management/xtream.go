package management

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type CategoryInfo struct {
	ID           string
	ExternalID   string
	CategoryName string
}

func handleError(playlist *Playlist, err error) bool {
	if err != nil {
		markImportFailed(playlist, err)
		return true
	}
	return false
}

func markImportFailed(playlist *Playlist, cause error) {
	log.Print(cause)
	playlist.ImportStatus = -1
	playlist.ImportMessage = "Error: " + cause.Error()
	if err := playlist.Update(); err != nil {
		log.Printf("Failed to update playlist %d import status: %v", playlist.ID, err)
	}
}

func reportImportProgress(playlist *Playlist, progress, total int, message string) error {
	playlist.ImportProgress = progress
	playlist.ImportTotal = total
	playlist.ImportMessage = message
	log.Printf("[playlist %d] %s", playlist.ID, message)
	if err := playlist.Update(); err != nil {
		return fmt.Errorf("update playlist import progress: %w", err)
	}
	return nil
}

func ImportXtream(c *gin.Context, ID uint) {
	log.Printf("Importing playlist %d", ID)

	playlist, err := GetPlaylistByID(ID)
	if err != nil {
		log.Print(err)
		return
	}

	newPlaylist := playlist.ImportStatus == 0

	startTime := time.Now()
	playlist.ImportStatus = 1
	playlist.ImportProgress = 0
	playlist.ImportTotal = 0
	if err := reportImportProgress(playlist, 0, 0, "Starting playlist import"); err != nil {
		markImportFailed(playlist, err)
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

	if err := reportImportProgress(playlist, 0, 0, "Checking playlist server information"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	var xtreamInfo XtreamInfo
	xtreamInfo, err = GetXtreamServerInfo(playlist)
	if handleError(playlist, err) {
		return
	}
	if err := reportImportProgress(playlist, 0, 0, "Playlist server information received"); err != nil {
		markImportFailed(playlist, err)
		return
	}

	playlist.ExpiresAt = xtreamInfo.UserInfo.ExpirationDate
	playlist.Expired = IsDateBeforeCurrent(xtreamInfo.UserInfo.ExpirationDate)

	streamFormat := "m3u8"
	if len(xtreamInfo.UserInfo.AllowedOutputFormats) > 0 {
		allowedFormats := "," + strings.Join(xtreamInfo.UserInfo.AllowedOutputFormats, ",") + ","
		if strings.Contains(allowedFormats, ",ts,") {
			streamFormat = "ts"
		} else if !strings.Contains(allowedFormats, ",m3u8,") {
			streamFormat = xtreamInfo.UserInfo.AllowedOutputFormats[0]
		}
	}

	var categoryListURL string
	if playlist.Type == "m3u" {
		categoryListURL = fmt.Sprintf("%s://%s/api/m3u/categories/%d", scheme, c.Request.Host, ID)
	} else {
		categoryListURL = fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_live_categories", baseURL, username, password)
	}

	if err := reportImportProgress(playlist, 0, 0, "Downloading playlist categories"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	categoryListResp, err := http.Get(categoryListURL)
	if err != nil {
		markImportFailed(playlist, fmt.Errorf("download playlist categories: %w", err))
		return
	}
	defer categoryListResp.Body.Close()

	categoriesBody, err := io.ReadAll(categoryListResp.Body)
	if err != nil {
		markImportFailed(playlist, fmt.Errorf("read playlist categories: %w", err))
		return
	}
	categoriesPath, categoriesSize, err := saveImportSnapshot(ID, "categories.json", bytes.NewReader(categoriesBody))
	if err != nil {
		markImportFailed(playlist, err)
		return
	}
	log.Printf("[playlist %d] Saved categories response (%d bytes) to %s", ID, categoriesSize, categoriesPath)

	var categoriesResponse []Category
	if err := json.Unmarshal(categoriesBody, &categoriesResponse); err != nil {
		markImportFailed(playlist, fmt.Errorf("decode playlist categories: %w", err))
		return
	}
	if err := reportImportProgress(playlist, 0, 0, fmt.Sprintf("Received %d categories", len(categoriesResponse))); err != nil {
		markImportFailed(playlist, err)
		return
	}

	var streamListURL string
	if playlist.Type == "m3u" {
		streamListURL = fmt.Sprintf("%s://%s/api/m3u/channels/%d", scheme, c.Request.Host, ID)
	} else {
		streamListURL = fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_live_streams", baseURL, username, password)
	}

	if err := reportImportProgress(playlist, 0, 0, "Downloading channel list"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	streamListResp, err := http.Get(streamListURL)
	if err != nil {
		markImportFailed(playlist, fmt.Errorf("download channel list: %w", err))
		return
	}
	defer streamListResp.Body.Close()

	streamsBody, err := io.ReadAll(streamListResp.Body)
	if err != nil {
		markImportFailed(playlist, fmt.Errorf("read channel list: %w", err))
		return
	}
	channelsPath, channelsSize, err := saveImportSnapshot(ID, "channels.json", bytes.NewReader(streamsBody))
	if err != nil {
		markImportFailed(playlist, err)
		return
	}
	log.Printf("[playlist %d] Saved channel response (%d bytes) to %s", ID, channelsSize, channelsPath)

	var channelsResponse []Channel
	if err := json.Unmarshal(streamsBody, &channelsResponse); err != nil {
		markImportFailed(playlist, fmt.Errorf("decode channel list: %w", err))
		return
	}
	if err := reportImportProgress(playlist, 0, len(channelsResponse), fmt.Sprintf("Decoded %d channels; preparing categories", len(channelsResponse))); err != nil {
		markImportFailed(playlist, err)
		return
	}

	const batchSize = 500
	if err := reportImportProgress(playlist, 0, len(channelsResponse), "Loading existing playlist categories"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	categoriesByExternalID := make(map[string]Category, len(categoriesResponse))
	if !newPlaylist {
		var existingCategories []Category
		if err := DB.Where("playlist_id = ?", playlist.ID).Find(&existingCategories).Error; err != nil {
			markImportFailed(playlist, err)
			return
		}
		for _, category := range existingCategories {
			if _, exists := categoriesByExternalID[category.ExternalID]; !exists {
				categoriesByExternalID[category.ExternalID] = category
			}
		}
	}

	var categoriesToCreate []Category
	var categoriesToUpdate []Category
	for i, category := range categoriesResponse {
		dbCategory, exists := categoriesByExternalID[category.ExternalID]
		if !exists {
			dbCategory = Category{
				ExternalID:   category.ExternalID,
				CategoryName: category.CategoryName,
				Num:          i,
				PlaylistID:   playlist.ID,
				Active:       false,
			}
			categoriesToCreate = append(categoriesToCreate, dbCategory)
			if len(categoriesToCreate) == batchSize {
				if err := DB.CreateInBatches(&categoriesToCreate, batchSize).Error; err != nil {
					markImportFailed(playlist, err)
					return
				}
				categoriesToCreate = categoriesToCreate[:0]
			}
			continue
		}

		dbCategory.CategoryName = category.CategoryName
		dbCategory.Num = i
		categoriesToUpdate = append(categoriesToUpdate, dbCategory)
		if len(categoriesToUpdate) == batchSize {
			if err := saveCategoriesInBatches(categoriesToUpdate, batchSize); err != nil {
				markImportFailed(playlist, err)
				return
			}
			categoriesToUpdate = categoriesToUpdate[:0]
		}
	}

	if len(categoriesToCreate) > 0 {
		if err := DB.CreateInBatches(&categoriesToCreate, batchSize).Error; err != nil {
			markImportFailed(playlist, err)
			return
		}
	}
	if err := saveCategoriesInBatches(categoriesToUpdate, batchSize); err != nil {
		markImportFailed(playlist, err)
		return
	}
	if err := reportImportProgress(playlist, 0, len(channelsResponse), fmt.Sprintf("Synchronized %d categories", len(categoriesResponse))); err != nil {
		markImportFailed(playlist, err)
		return
	}

	var categories []Category
	if err := reportImportProgress(playlist, 0, len(channelsResponse), "Loading category identifiers"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	if err := DB.Where("playlist_id = ?", playlist.ID).Find(&categories).Error; err != nil {
		markImportFailed(playlist, err)
		return
	}
	categoryIDByExternalID := make(map[string]uint, len(categories))
	for _, category := range categories {
		if _, exists := categoryIDByExternalID[category.ExternalID]; !exists {
			categoryIDByExternalID[category.ExternalID] = category.ID
		}
	}

	// Delete all channels with StreanID = "-1" (Categories fake channels)
	if err := DB.Where("stream_id = ?", -1).Delete(&Channel{}).Error; err != nil {
		markImportFailed(playlist, err)
		return
	}

	if err := reportImportProgress(playlist, 0, len(channelsResponse), "Loading existing playlist channels"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	existingChannelsByKey := make(map[channelImportKey]Channel, len(channelsResponse))
	if !newPlaylist {
		var existingChannels []Channel
		if err := DB.Joins("JOIN categories ON categories.id = channels.category_id").
			Where("categories.playlist_id = ?", playlist.ID).
			Find(&existingChannels).Error; err != nil {
			markImportFailed(playlist, err)
			return
		}
		for _, channel := range existingChannels {
			key := channelImportKey{categoryID: channel.CategoryID, streamID: channel.StreamID}
			if _, exists := existingChannelsByKey[key]; !exists {
				existingChannelsByKey[key] = channel
			}
		}
		log.Printf("[playlist %d] Loaded %d existing channels", playlist.ID, len(existingChannels))
	}

	if err := reportImportProgress(playlist, 0, len(channelsResponse), fmt.Sprintf("Importing %d channels", len(channelsResponse))); err != nil {
		markImportFailed(playlist, err)
		return
	}
	var channelsToCreate []Channel
	var channelsToUpdate []Channel
	hdhrChannelNum := 1000
	processedChannels := 0
	createdChannels := 0
	updatedChannels := 0
	skippedChannels := 0
	reportChannelBatch := func() error {
		if processedChannels%batchSize != 0 && processedChannels != len(channelsResponse) {
			return nil
		}
		if err := flushChannelImportBatches(&channelsToCreate, &channelsToUpdate, batchSize); err != nil {
			return err
		}
		message := fmt.Sprintf(
			"Imported %d/%d channels (created %d, updated %d, skipped %d)",
			processedChannels, len(channelsResponse), createdChannels, updatedChannels, skippedChannels,
		)
		return reportImportProgress(playlist, processedChannels, len(channelsResponse), message)
	}
	for _, channel := range channelsResponse {
		processedChannels++
		categoryID, exists := categoryIDByExternalID[channel.ExternalCategoryID]
		if !exists {
			skippedChannels++
			if err := reportChannelBatch(); err != nil {
				markImportFailed(playlist, err)
				return
			}
			continue
		}

		key := channelImportKey{categoryID: categoryID, streamID: channel.StreamID}
		dbChannel, exists := existingChannelsByKey[key]
		dbChannel.Num = channel.Num
		dbChannel.Name = channel.Name
		dbChannel.CategoryID = categoryID
		dbChannel.ExternalCategoryID = channel.ExternalCategoryID
		dbChannel.StreamID = channel.StreamID
		if playlist.Type == "m3u" {
			dbChannel.StreamURL = channel.StreamURL
		} else {
			dbChannel.StreamURL = fmt.Sprintf("%s/live/%s/%s/%d.%s", baseURL, username, password, channel.StreamID, streamFormat)
		}
		dbChannel.EpgChannelID = channel.EpgChannelID
		dbChannel.HDHRChannelNum = hdhrChannelNum
		dbChannel.StreamIcon = channel.StreamIcon
		if !exists {
			dbChannel.Active = true
			channelsToCreate = append(channelsToCreate, dbChannel)
			createdChannels++
		} else {
			channelsToUpdate = append(channelsToUpdate, dbChannel)
			updatedChannels++
		}
		hdhrChannelNum++

		if err := reportChannelBatch(); err != nil {
			markImportFailed(playlist, err)
			return
		}
	}

	if err := flushChannelImportBatches(&channelsToCreate, &channelsToUpdate, batchSize); err != nil {
		markImportFailed(playlist, err)
		return
	}

	if err := reportImportProgress(playlist, len(channelsResponse), len(channelsResponse), "Removing categories no longer in the playlist"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	if err := DB.Delete(&Category{}, "playlist_id = ? and updated_at < ?", ID, startTime).Error; err != nil {
		markImportFailed(playlist, err)
		return
	}
	if err := reportImportProgress(playlist, len(channelsResponse), len(channelsResponse), "Removing channels no longer in the playlist"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	if err := DB.Where("category_id IN (SELECT id FROM categories WHERE playlist_id = ?) AND updated_at < ?", ID, startTime).Delete(&Channel{}).Error; err != nil {
		markImportFailed(playlist, err)
		return
	}

	if err := reportImportProgress(playlist, len(channelsResponse), len(channelsResponse), "Recalculating channel numbers"); err != nil {
		markImportFailed(playlist, err)
		return
	}
	if err := UpdateHDHRChannelNumForAllChannels(); err != nil {
		markImportFailed(playlist, err)
		return
	}
	playlist.ImportStatus = 2
	if err := reportImportProgress(playlist, len(channelsResponse), len(channelsResponse),
		fmt.Sprintf("Import complete: %d channels created, %d updated, %d skipped in %s",
			createdChannels, updatedChannels, skippedChannels, time.Since(startTime))); err != nil {
		markImportFailed(playlist, err)
		return
	}

	log.Printf("[playlist %d] Imported in %s", playlist.ID, time.Since(startTime))
}

type channelImportKey struct {
	categoryID uint
	streamID   int
}

func flushChannelImportBatches(toCreate, toUpdate *[]Channel, batchSize int) error {
	if len(*toCreate) > 0 {
		if err := DB.CreateInBatches(toCreate, batchSize).Error; err != nil {
			return fmt.Errorf("create channel batch: %w", err)
		}
		*toCreate = (*toCreate)[:0]
	}
	if err := saveChannelsInBatches(*toUpdate, batchSize); err != nil {
		return fmt.Errorf("update channel batch: %w", err)
	}
	*toUpdate = (*toUpdate)[:0]
	return nil
}

func saveCategoriesInBatches(categories []Category, batchSize int) error {
	for start := 0; start < len(categories); start += batchSize {
		end := start + batchSize
		if end > len(categories) {
			end = len(categories)
		}
		batch := categories[start:end]
		if err := DB.Omit(clause.Associations).Save(&batch).Error; err != nil {
			return err
		}
	}
	return nil
}

func saveChannelsInBatches(channels []Channel, batchSize int) error {
	for start := 0; start < len(channels); start += batchSize {
		end := start + batchSize
		if end > len(channels) {
			end = len(channels)
		}
		batch := channels[start:end]
		if err := DB.Omit(clause.Associations).Save(&batch).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetXtreamServerInfo(playlist *Playlist) (XtreamInfo, error) {
	if playlist.Type != "xcode" {
		return XtreamInfo{}, nil
	}
	infoURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_simple_date_table", playlist.Server, playlist.Username, playlist.Password)
	response, err := http.Get(infoURL)
	if err != nil {
		return XtreamInfo{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return XtreamInfo{}, err
	}
	var xtreamInfo XtreamInfo
	if err = json.Unmarshal(body, &xtreamInfo); err != nil {
		return XtreamInfo{}, err
	}
	return xtreamInfo, err
}
