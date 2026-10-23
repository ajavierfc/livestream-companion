package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestXtreamExportIncludesOnlyActivePlaylistCategories(t *testing.T) {
	previousDB := DB
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "xtream-export.db")), &gorm.Config{})
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
	playlist := Playlist{Description: "xtream-export"}
	if err := DB.Create(&playlist).Error; err != nil {
		t.Fatal(err)
	}
	categories := []Category{
		{PlaylistID: playlist.ID, CategoryName: "Enabled", Active: true},
		{PlaylistID: playlist.ID, CategoryName: "Disabled", Active: false},
	}
	if err := DB.Create(&categories).Error; err != nil {
		t.Fatal(err)
	}
	channels := []Channel{
		{CategoryID: categories[0].ID, Name: "Visible", StreamURL: "http://upstream/visible.ts"},
		{CategoryID: categories[1].ID, Name: "Hidden", StreamURL: "http://upstream/hidden.ts"},
		{CategoryID: categories[0].ID, Name: "### placeholder"},
	}
	if err := DB.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/xtream/:playlistID", XtreamPlayerAPIHandler)
	router.GET("/xtream/:playlistID/player_api.php", XtreamPlayerAPIHandler)
	router.GET("/xtream/:playlistID/live/*streamPath", XtreamPlaylistLiveStreamHandler)
	router.GET("/live/:username/:password/*streamPath", XtreamLiveStreamHandler)

	userInfoResponse := httptest.NewRecorder()
	router.ServeHTTP(userInfoResponse, httptest.NewRequest(http.MethodGet,
		"/xtream/1?username=any-user&password=any-password", nil))
	if userInfoResponse.Code != http.StatusOK {
		t.Fatalf("user-info endpoint returned %d: %s", userInfoResponse.Code, userInfoResponse.Body.String())
	}

	categoryResponse := httptest.NewRecorder()
	router.ServeHTTP(categoryResponse, httptest.NewRequest(http.MethodGet,
		"/xtream/1/player_api.php?action=get_live_categories", nil))
	if categoryResponse.Code != http.StatusOK {
		t.Fatalf("category endpoint returned %d: %s", categoryResponse.Code, categoryResponse.Body.String())
	}
	var categoriesResponse []xtreamLiveCategory
	if err := json.Unmarshal(categoryResponse.Body.Bytes(), &categoriesResponse); err != nil {
		t.Fatal(err)
	}
	if len(categoriesResponse) != 1 || categoriesResponse[0].CategoryName != "Enabled" {
		t.Fatalf("unexpected exported categories: %+v", categoriesResponse)
	}

	streamResponse := httptest.NewRecorder()
	router.ServeHTTP(streamResponse, httptest.NewRequest(http.MethodGet,
		"/xtream/1/player_api.php?action=get_live_streams&username=any-user&password=any-password", nil))
	if streamResponse.Code != http.StatusOK {
		t.Fatalf("streams endpoint returned %d: %s", streamResponse.Code, streamResponse.Body.String())
	}
	var streamsResponse []xtreamLiveStream
	if err := json.Unmarshal(streamResponse.Body.Bytes(), &streamsResponse); err != nil {
		t.Fatal(err)
	}
	if len(streamsResponse) != 1 || streamsResponse[0].Name != "Visible" ||
		streamsResponse[0].CategoryID != categoriesResponse[0].CategoryID {
		t.Fatalf("unexpected exported streams: %+v", streamsResponse)
	}
	if !strings.HasPrefix(streamsResponse[0].DirectSource, "/live/any-user/any-password/") {
		t.Fatalf("stream has unexpected direct source: %q", streamsResponse[0].DirectSource)
	}

	inactiveResponse := httptest.NewRecorder()
	router.ServeHTTP(inactiveResponse, httptest.NewRequest(http.MethodGet,
		"/live/any-user/any-password/"+strconv.Itoa(channels[1].ID)+".ts", nil))
	if inactiveResponse.Code != http.StatusNotFound {
		t.Fatalf("inactive channel route returned %d, want %d: %s",
			inactiveResponse.Code, http.StatusNotFound, inactiveResponse.Body.String())
	}
}
