package management

import (
	"log"
	"sync"
	"time"
)

func (c *Channel) Save() error {
	result := DB.Create(&c)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (c *Channel) Update() error {
	result := DB.Model(&Channel{}).Where("id = ?", c.ID).UpdateColumns(map[string]interface{}{
		"Active": c.Active,
		"Name":   c.Name,
	})

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (c *Channel) Delete() error {
	result := DB.Unscoped().Delete(&c)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

func GetChannels() ([]Channel, error) {
	var channels []Channel
	result := DB.Find(&channels)
	if result.Error != nil {
		return nil, result.Error
	}

	return channels, nil
}

func GetChannelsByPlaylistID(playlistID uint) ([]Channel, error) {
	var channels []Channel

	result := DB.Joins("JOIN categories on channels.category_id = categories.id").
		Joins("JOIN playlists on categories.playlist_id = playlists.id").
		Where("playlists.id = ? and categories.active = 1", playlistID).
		Order("categories.num ASC, channels.hdhr_channel_num ASC").
		Find(&channels)

	if result.Error != nil {
		return nil, result.Error
	}

	return channels, nil
}

func GetChannelByID(id uint) (*Channel, error) {
	var channel Channel
	result := DB.First(&channel, id)
	if result.Error != nil {
		return nil, result.Error
	}

	return &channel, nil
}

func GetChannelsByEpgId(epgId string) ([]*Channel, error) {
	var channels []*Channel
	result := DB.Where("epg_channel_id = ?", epgId).Find(&channels)
	if result.Error != nil {
		return nil, result.Error
	}

	return channels, nil
}

func GetChannelsByEpgIdAndPlaylistId(epgId string, playlistId uint) ([]*Channel, error) {
	var channels []*Channel
	result := DB.Joins("JOIN categories on categories.id = channels.category_id").Where("channels.epg_channel_id = ? AND categories.playlist_id = ?", epgId, playlistId).Find(&channels)
	if result.Error != nil {
		return nil, result.Error
	}

	return channels, nil
}

func GetChannelsWithNoEpg() ([]*Channel, error) {
	var channels []*Channel
	result := DB.Where("epg_channel_id = ''").Find(&channels)

	if result.Error != nil {
		return nil, result.Error
	}

	return channels, nil
}

func GetChannelsWithNoEpgByPlaylistId(playlistId uint) ([]*Channel, error) {
	var channels []*Channel
	result := DB.Joins("JOIN categories on categories.id = channels.category_id").
		Where("channels.epg_channel_id = '' AND categories.playlist_id = ?", playlistId).
		Find(&channels)

	if result.Error != nil {
		return nil, result.Error
	}

	return channels, nil
}

func UpdateActiveChannelsByPlaylistID(playlistId uint, active bool) error {
	var playlist Playlist
	result := DB.Preload("Categories.Channels").First(&playlist, playlistId)
	if result.Error != nil {
		return result.Error
	}

	for _, category := range playlist.Categories {
		for _, channel := range category.Channels {
			channel.Active = active
			if err := DB.Save(&channel).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func UpdateActiveChannelsByCategoryID(categoryId uint, active bool) error {
	var category Category
	result := DB.Preload("Channels").First(&category, categoryId)
	if result.Error != nil {
		return result.Error
	}

	for _, channel := range category.Channels {
		channel.Active = active
		if err := DB.Save(&channel).Error; err != nil {
			return err
		}
	}

	return nil
}

var mutexChannelNum = &sync.Mutex{}

func UpdateHDHRChannelNumForAllChannels() error {
	mutexChannelNum.Lock() // Lock the mutex.
	defer mutexChannelNum.Unlock()

	startedAt := time.Now()
	log.Printf("Recalculating HDHR channel numbers")
	result := DB.Exec(`
		WITH numbered_channels AS MATERIALIZED (
			SELECT channels.id, 999 + ROW_NUMBER() OVER (
				ORDER BY playlists.id, categories.id, channels.hdhr_channel_num, channels.id
			) AS new_channel_num
			FROM channels
			JOIN categories ON categories.id = channels.category_id
			JOIN playlists ON playlists.id = categories.playlist_id
		)
		UPDATE channels
		SET hdhr_channel_num = numbered_channels.new_channel_num
		FROM numbered_channels
		WHERE channels.id = numbered_channels.id
	`)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("Recalculated HDHR numbers for %d channels in %s", result.RowsAffected, time.Since(startedAt))
	return nil
}

func GetChannelsByCategoryId(categoryId uint) ([]Channel, error) {
	var channels []Channel

	//err := DB.Preload("Programmes").Where("category_id = ?", categoryId).Order("channels.hdhr_channel_num asc").Find(&channels).Error
	err := DB.Preload("Programmes").Where("category_id = ?", categoryId).Order("channels.hdhr_channel_num asc").Find(&channels).Error
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func GetProgrammesByChannelID(channelID uint) ([]Programme, error) {
	var programmes []Programme

	err := DB.Where("channel_id = ?", channelID).Order("start asc").Find(&programmes).Error
	if err != nil {
		return nil, err
	}

	return programmes, nil
}
