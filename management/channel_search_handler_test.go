package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSearchChannelsHandlerIncludesAndFiltersChannelsWithoutQuality(t *testing.T) {
	previousDB := DB
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "channel-search.db")), &gorm.Config{})
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

	if err := DB.AutoMigrate(&Playlist{}, &Category{}, &Channel{}); err != nil {
		t.Fatal(err)
	}
	playlist := Playlist{Description: "Search handler test"}
	if err := DB.Create(&playlist).Error; err != nil {
		t.Fatal(err)
	}
	category := Category{PlaylistID: playlist.ID, CategoryName: "Test", Active: true}
	if err := DB.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	channel := Channel{CategoryID: category.ID, StreamID: 1, Name: "NoQualitySuffixTest"}
	if err := DB.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/channels/search", SearchChannelsHandler)
	requestURL := fmt.Sprintf("/api/channels/search?playlistId=%d&q=NoQualitySuffixTest", playlist.ID)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestURL, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result ChannelSearchResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Qualities) != 1 || result.Qualities[0].Value != "NONE" || result.Qualities[0].Count != 1 {
		t.Fatalf("quality facets = %+v, want NONE count 1", result.Qualities)
	}

	requestURL += "&quality=NONE"
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestURL, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("NONE filter status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	result = ChannelSearchResult{}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Channels) != 1 || result.Channels[0].ID != channel.ID {
		t.Fatalf("NONE filter returned %+v, want channel ID %d", result, channel.ID)
	}
}
