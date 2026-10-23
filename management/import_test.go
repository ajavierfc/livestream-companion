package management

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSaveImportSnapshot(t *testing.T) {
	dir := t.TempDir()
	path, size, err := saveImportSnapshotTo(dir, 42, "channels.json", strings.NewReader(`[{"name":"test"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(`[{"name":"test"}]`)) {
		t.Fatalf("snapshot size is %d, want %d", size, len(`[{"name":"test"}]`))
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `[{"name":"test"}]` {
		t.Fatalf("snapshot content is %q", data)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("snapshot permissions are %04o, want 0600", info.Mode().Perm())
	}
}

func TestBatchSaveAndHDHRChannelRenumbering(t *testing.T) {
	previousDB := DB
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = testDB
	t.Cleanup(func() {
		sqlDB, err := testDB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
	})

	if err := DB.AutoMigrate(&Playlist{}, &Category{}, &Channel{}, &Programme{}); err != nil {
		t.Fatal(err)
	}

	playlists := []Playlist{{Description: "first"}, {Description: "second"}}
	if err := DB.Create(&playlists).Error; err != nil {
		t.Fatal(err)
	}
	if err := reportImportProgress(&playlists[0], 500, 40000, "Imported 500/40000 channels"); err != nil {
		t.Fatal(err)
	}
	var playlistWithProgress Playlist
	if err := DB.First(&playlistWithProgress, playlists[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if playlistWithProgress.ImportProgress != 500 ||
		playlistWithProgress.ImportTotal != 40000 ||
		playlistWithProgress.ImportMessage != "Imported 500/40000 channels" {
		t.Errorf("playlist progress was not persisted: %+v", playlistWithProgress)
	}

	categories := []Category{
		{PlaylistID: playlists[0].ID, ExternalID: "a", CategoryName: "A"},
		{PlaylistID: playlists[0].ID, ExternalID: "b", CategoryName: "B"},
		{PlaylistID: playlists[1].ID, ExternalID: "c", CategoryName: "C"},
	}
	if err := DB.Create(&categories).Error; err != nil {
		t.Fatal(err)
	}

	channels := []Channel{
		{CategoryID: categories[0].ID, StreamID: 1, EpgChannelID: "shared", Name: "first category, later"},
		{CategoryID: categories[0].ID, StreamID: 2, EpgChannelID: "shared", Name: "first category, earlier"},
		{CategoryID: categories[1].ID, StreamID: 3, EpgChannelID: "other", Name: "second category"},
		{CategoryID: categories[2].ID, StreamID: 4, EpgChannelID: "other", Name: "second playlist"},
	}
	channels[0].HDHRChannelNum = 11
	channels[1].HDHRChannelNum = 10
	channels[2].HDHRChannelNum = 4
	channels[3].HDHRChannelNum = 6
	if err := DB.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}

	if err := saveCategoriesInBatches([]Category{
		{ID: categories[0].ID, PlaylistID: playlists[0].ID, ExternalID: "a", CategoryName: "Updated A"},
		{ID: categories[1].ID, PlaylistID: playlists[0].ID, ExternalID: "b", CategoryName: "Updated B"},
	}, 1); err != nil {
		t.Fatal(err)
	}
	for i := range channels {
		channels[i].Name += " updated"
	}
	if err := saveChannelsInBatches(channels, 2); err != nil {
		t.Fatal(err)
	}
	var tv Tv
	if err := xml.Unmarshal([]byte(`<tv>
		<channel id="shared"/>
		<channel id="other"/>
		<programme channel="shared" start="1" stop="2"><title>shared 1</title></programme>
		<programme channel="shared" start="2" stop="3"><title>shared 2</title></programme>
		<programme channel="other" start="3" stop="4"><title>other</title></programme>
	</tv>`), &tv); err != nil {
		t.Fatal(err)
	}
	if err := insertPlaylistProgrammes(playlists[0].ID, &tv); err != nil {
		t.Fatal(err)
	}
	if err := UpdateHDHRChannelNumForAllChannels(); err != nil {
		t.Fatal(err)
	}

	var got []Channel
	if err := DB.Order("id").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != len(channels) {
		t.Fatalf("got %d channels, want %d", len(got), len(channels))
	}
	wantHDHR := map[int]int{2: 1000, 1: 1001, 3: 1002, 4: 1003}
	for _, channel := range got {
		if channel.HDHRChannelNum != wantHDHR[channel.StreamID] {
			t.Errorf("stream %d got HDHR number %d, want %d", channel.StreamID, channel.HDHRChannelNum, wantHDHR[channel.StreamID])
		}
		if channel.Name[len(channel.Name)-8:] != " updated" {
			t.Errorf("stream %d name was not batch-updated: %q", channel.StreamID, channel.Name)
		}
	}

	var categoryCount int64
	if err := DB.Model(&Category{}).Count(&categoryCount).Error; err != nil {
		t.Fatal(err)
	}
	if categoryCount != int64(len(categories)) {
		t.Errorf("batch save created categories: got %d, want %d", categoryCount, len(categories))
	}

	var programmeCount int64
	if err := DB.Model(&Programme{}).Count(&programmeCount).Error; err != nil {
		t.Fatal(err)
	}
	if programmeCount != 5 {
		t.Errorf("got %d EPG programmes, want 5", programmeCount)
	}
}
