package management

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type EPGProgramme struct {
	XMLName        xml.Name `xml:"programme"`
	Start          string   `xml:"start,attr"`
	Stop           string   `xml:"stop,attr"`
	StartTimestamp string   `xml:"start_timestamp,attr"`
	StopTimestamp  string   `xml:"stop_timestamp,attr"`
	Channel        string   `xml:"channel,attr"`
	Title          string   `xml:"title"`
	Desc           string   `xml:"desc"`
	Items          []XMLAny `xml:",any"`
}

type Tv struct {
	XMLName  xml.Name `xml:"tv"`
	Channels []struct {
		ID             string `xml:"id,attr"`
		EpgDisplayName string `xml:"display-name"`
		Icon           struct {
			Src string `xml:"src,attr"`
		} `xml:"icon"`
		Extra []XMLAny `xml:",any"`
	} `xml:"channel"`
	Programmes []EPGProgramme `xml:"programme"`
	XMLAny     []xml.Token
}

type XMLAny struct {
	XMLName xml.Name
	Content string `xml:",innerxml"`
}

func UpdateDBEPG(checkLastProcessed bool) {
	log.Printf("Start Updating the EPG Database")
	if _, err := os.Stat("epg"); os.IsNotExist(err) {
		os.MkdirAll("epg", os.ModePerm) // os.ModePerm is 0777
	}

	playlists, err := GetPlaylists()
	if err != nil {
		log.Fatal(err)
	}

	// Loop through each playlist and process it
	for _, playlist := range playlists {
		// Check if the last processed time should be considered
		if checkLastProcessed {
			// Check if it has been more than 12 hours since the last update
			if time.Since(playlist.EPGLastProcessedAt).Hours() < 12 {
				log.Printf("Playlist %v was already processed less than 12 hours ago. Skipping...", playlist.ID)
				continue
			}
		}

		// Call the function that will update the playlist
		err := UpdatePlaylistEPG(playlist)
		if err != nil {
			log.Printf("%v", err)
		}
	}

	vaccum()
}

var mutexEPG = &sync.Mutex{}

func markEPGFailed(playlist *Playlist) {
	playlist.EpgStatus = -1
	if err := DB.Save(playlist).Error; err != nil {
		log.Printf("Failed to update playlist %d EPG status: %v", playlist.ID, err)
	}
}

func UpdatePlaylistEPG(playlist Playlist) error {
	mutexEPG.Lock()
	defer mutexEPG.Unlock()

	log.Printf("Processing playlist: %v [%v]", playlist.ID, playlist.Description)

	xtreamInfo, err := GetXtreamServerInfo(&playlist)
	if err == nil && xtreamInfo.UserInfo.ExpirationDate != "" {
		playlist.ExpiresAt = xtreamInfo.UserInfo.ExpirationDate
	}
	if playlist.ExpiresAt != "" {
		playlist.Expired = IsDateBeforeCurrent(playlist.ExpiresAt)
		playlist.Update()
	}
	if err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to update playlist status: %w", err)
	}

	if playlist.XmltvURL == "" {
		return nil
	}

	playlist.EpgStatus = 1
	if err := DB.Save(&playlist).Error; err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to update playlist status: %w", err)
	}

	url := playlist.XmltvURL
	response, err := http.Get(url)
	if err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to get HTTP response: %w", err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to read response body: %w", err)
	}

	epgFilePath := fmt.Sprintf("epg/%v.xml", playlist.ID)
	err = os.WriteFile(epgFilePath, data, 0644)
	if err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to write to EPG file: %w", err)
	}

	log.Printf("EPG for playlist %v downloaded successfully.", playlist.ID)

	// Read and modify XML
	tv := &Tv{}
	err = xml.Unmarshal(data, tv)
	if err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to unmarshal XML data: %w", err)
	}

	// Raw SQL to delete all Programme entries where the channel_id matches a EpgChannelID in Channels for a given playlist_id
	if err := DB.Exec("DELETE FROM programmes WHERE channel_id IN (SELECT id FROM channels WHERE category_id IN (SELECT id FROM categories WHERE playlist_id = ?))", playlist.ID).Error; err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to delete programmes from DB: %w", err)
	}

	if err := insertPlaylistProgrammes(playlist.ID, tv); err != nil {
		markEPGFailed(&playlist)
		return fmt.Errorf("failed to import EPG programmes: %w", err)
	}

	// update the EPGLastProcessedAt field and save the playlist
	playlist.EPGLastProcessedAt = time.Now()
	playlist.EpgStatus = 2
	if err := DB.Save(&playlist).Error; err != nil {
		playlist.EpgStatus = -1
		DB.Save(&playlist)
		return fmt.Errorf("failed to save playlist: %w", err)
	}

	log.Printf("Playlist %v processed successfully.", playlist.ID)

	return nil
}

func insertPlaylistProgrammes(playlistID uint, tv *Tv) error {
	channelsByEpgID := make(map[string][]Channel)
	var dbChannels []Channel
	if err := DB.Joins("JOIN categories ON categories.id = channels.category_id").
		Where("categories.playlist_id = ?", playlistID).
		Find(&dbChannels).Error; err != nil {
		return fmt.Errorf("failed to load playlist channels: %w", err)
	}
	for _, channel := range dbChannels {
		channelsByEpgID[channel.EpgChannelID] = append(channelsByEpgID[channel.EpgChannelID], channel)
	}

	programmeIndexesByEpgID := make(map[string][]int)
	for i, programme := range tv.Programmes {
		programmeIndexesByEpgID[programme.Channel] = append(programmeIndexesByEpgID[programme.Channel], i)
	}

	const programmeBatchSize = 500
	programmesToInsert := make([]Programme, 0, programmeBatchSize)
	for _, epgChannel := range tv.Channels {
		indexes := programmeIndexesByEpgID[epgChannel.ID]
		for _, dbChannel := range channelsByEpgID[epgChannel.ID] {
			for _, index := range indexes {
				epgProgramme := tv.Programmes[index]
				programmesToInsert = append(programmesToInsert, Programme{
					Start:          epgProgramme.Start,
					Stop:           epgProgramme.Stop,
					StartTimestamp: epgProgramme.StartTimestamp,
					StopTimestamp:  epgProgramme.StopTimestamp,
					Channel:        epgProgramme.Channel,
					ChannelID:      dbChannel.ID,
					Title:          epgProgramme.Title,
					Desc:           epgProgramme.Desc,
				})
				if len(programmesToInsert) == programmeBatchSize {
					if err := DB.CreateInBatches(&programmesToInsert, programmeBatchSize).Error; err != nil {
						return fmt.Errorf("failed to save EPG programme batch: %w", err)
					}
					programmesToInsert = programmesToInsert[:0]
				}
			}
		}
	}
	if len(programmesToInsert) > 0 {
		if err := DB.CreateInBatches(&programmesToInsert, programmeBatchSize).Error; err != nil {
			return fmt.Errorf("failed to save EPG programme batch: %w", err)
		}
	}
	return nil
}

func ExportDBEPGToXML() ([]byte, error) {
	var channels []Channel
	if err := DB.Joins("Category").Where("Category.active = ?", 1).Preload("Programmes").Find(&channels).Error; err != nil {
		log.Printf("Failed to fetch Channels: %v", err)
		return nil, err
	}

	xmlChannels := make([]struct {
		ID             string `xml:"id,attr"`
		EpgDisplayName string `xml:"display-name"`
		Icon           struct {
			Src string `xml:"src,attr"`
		} `xml:"icon"`
		Extra []XMLAny `xml:",any"`
	}, len(channels))

	var xmlProgrammes []EPGProgramme

	for i, ch := range channels {
		xmlChannels[i] = struct {
			ID             string `xml:"id,attr"`
			EpgDisplayName string `xml:"display-name"`
			Icon           struct {
				Src string `xml:"src,attr"`
			} `xml:"icon"`
			Extra []XMLAny `xml:",any"`
		}{
			ID:             strconv.Itoa(ch.HDHRChannelNum),
			EpgDisplayName: ch.Name,
			Icon: struct {
				Src string `xml:"src,attr"`
			}{Src: ch.StreamIcon},
		}

		for _, p := range ch.Programmes {
			xmlProgrammes = append(xmlProgrammes, EPGProgramme{
				Start:          p.Start,
				Stop:           p.Stop,
				StartTimestamp: p.StartTimestamp,
				StopTimestamp:  p.StopTimestamp,
				Channel:        strconv.Itoa(ch.HDHRChannelNum),
				Title:          p.Title,
				Desc:           p.Desc,
			})
		}
	}

	tv := &Tv{
		Channels:   xmlChannels,
		Programmes: xmlProgrammes,
	}

	data, err := xml.MarshalIndent(tv, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal Tv to XML: %v", err)
		return nil, err
	}

	log.Println("EPG database exported successfully.")
	return data, nil
}
