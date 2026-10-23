package management

import (
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

func channelNameSortExpression() string {
	baseName := `CASE WHEN instr(channels.name, ': ') BETWEEN 2 AND 13
		THEN substr(channels.name, instr(channels.name, ': ') + 2)
		ELSE channels.name END`

	baseName = `trim(` + baseName + `)`
	nameWithoutFrameRate := `CASE
		WHEN ` + baseName + ` LIKE '% ⁶⁰ᶠᵖˢ' THEN substr(` + baseName + `, 1, length(` + baseName + `) - 6)
		ELSE ` + baseName + `
	END`

	suffixes := []string{"ʰᵉᵛᶜ", "ᵁᴴᴰ", "ᴴᴰ", "ᴿᴬᵂ", "HEVC", "RAW", "UHD", "FHD", "SD", "HD", "4K", "8K"}
	var suffixLengthCases strings.Builder
	for _, suffix := range suffixes {
		length := len([]rune(" " + suffix))
		pattern := "%" + " " + suffix
		if suffixLengthCases.Len() > 0 {
			suffixLengthCases.WriteString(" ")
		}
		suffixLengthCases.WriteString("WHEN ")
		if strings.ContainsAny(suffix, "ᴿᴬᵂᴴᵁᵉᵛᶜ") {
			suffixLengthCases.WriteString(nameWithoutFrameRate + " LIKE '" + pattern + "'")
		} else {
			suffixLengthCases.WriteString("upper(" + nameWithoutFrameRate + ") LIKE '" + pattern + "'")
		}
		suffixLengthCases.WriteString(" THEN ")
		suffixLengthCases.WriteString(strconv.Itoa(length))
	}

	return `CASE
		WHEN CASE ` + suffixLengthCases.String() + ` ELSE 0 END > 0
		THEN substr(` + nameWithoutFrameRate + `, 1, length(` + nameWithoutFrameRate + `) - (CASE ` + suffixLengthCases.String() + ` ELSE 0 END))
		ELSE ` + nameWithoutFrameRate + `
	END`
}

type ChannelSearchResult struct {
	Channels  []Channel      `json:"channels"`
	Total     int64          `json:"total"`
	Page      int            `json:"page"`
	PageSize  int            `json:"pageSize"`
	Prefixes  []ChannelFacet `json:"prefixes"`
	Qualities []ChannelFacet `json:"qualities"`
}

type ChannelFacet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

func channelPrefixExpression() string {
	return `CASE
		WHEN instr(channels.name, ': ') BETWEEN 2 AND 13
		THEN trim(substr(channels.name, 1, instr(channels.name, ': ') - 1))
		ELSE ''
	END`
}

func channelQualityExpression() string {
	name := `trim(channels.name)`
	name = `CASE
		WHEN ` + name + ` LIKE '% ⁶⁰ᶠᵖˢ' THEN substr(` + name + `, 1, length(` + name + `) - 6)
		ELSE ` + name + `
	END`

	return `CASE
		WHEN ` + name + ` LIKE '% ᵁᴴᴰ' THEN 'UHD'
		WHEN ` + name + ` LIKE '% ᴴᴰ' THEN 'HD'
		WHEN ` + name + ` LIKE '% ʰᵉᵛᶜ' THEN 'HEVC'
		WHEN ` + name + ` LIKE '% ᴿᴬᵂ' THEN 'RAW'
		WHEN upper(` + name + `) LIKE '% UHD' THEN 'UHD'
		WHEN upper(` + name + `) LIKE '% FHD' THEN 'FHD'
		WHEN upper(` + name + `) LIKE '% 4K' THEN '4K'
		WHEN upper(` + name + `) LIKE '% 8K' THEN '8K'
		WHEN upper(` + name + `) LIKE '% SD' THEN 'SD'
		WHEN upper(` + name + `) LIKE '% HD' THEN 'HD'
		WHEN upper(` + name + `) LIKE '% RAW' THEN 'RAW'
		WHEN upper(` + name + `) LIKE '% HEVC' THEN 'HEVC'
		ELSE ''
	END`
}

func SearchChannels(playlistID uint, categoryID *uint, query, prefix, quality string, page, pageSize int) (ChannelSearchResult, error) {
	var result ChannelSearchResult
	result.Page = page
	result.PageSize = pageSize

	baseQuery := func() *gorm.DB {
		db := DB.Model(&Channel{}).
			Joins("JOIN categories ON categories.id = channels.category_id").
			Where("categories.playlist_id = ? AND categories.active = ? AND channels.name NOT LIKE ?", playlistID, true, "###%")
		if categoryID != nil {
			db = db.Where("channels.category_id = ?", *categoryID)
		}
		if query != "" {
			escapedQuery := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
			db = db.Where(`channels.name LIKE ? ESCAPE '\'`, "%"+escapedQuery+"%")
		}
		return db
	}

	db := baseQuery()
	if prefix != "" {
		db = db.Where(channelPrefixExpression()+" = ?", prefix)
	}
	if quality != "" {
		db = db.Where(channelQualityExpression()+" = ?", quality)
	}

	if err := db.Count(&result.Total).Error; err != nil {
		return ChannelSearchResult{}, err
	}
	if err := baseQuery().
		Select(channelPrefixExpression() + " AS value, COUNT(*) AS count").
		Where(channelPrefixExpression() + " <> ''").
		Group("value").
		Order("count DESC, value ASC").
		Scan(&result.Prefixes).Error; err != nil {
		return ChannelSearchResult{}, err
	}
	if err := baseQuery().
		Select(channelQualityExpression() + " AS value, COUNT(*) AS count").
		Where(channelQualityExpression() + " <> ''").
		Group("value").
		Order("count DESC, value ASC").
		Scan(&result.Qualities).Error; err != nil {
		return ChannelSearchResult{}, err
	}
	if err := db.Preload("Category").
		Order(channelNameSortExpression() + " COLLATE NOCASE ASC, channels.id ASC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&result.Channels).Error; err != nil {
		return ChannelSearchResult{}, err
	}
	return result, nil
}

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
