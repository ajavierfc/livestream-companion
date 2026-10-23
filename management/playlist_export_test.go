package management

import (
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

func TestExportPlaylistM3U8IncludesOnlyActiveCategories(t *testing.T) {
	previousDB := DB
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "playlist-export.db")), &gorm.Config{})
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
	playlist := Playlist{Description: "export"}
	if err := DB.Create(&playlist).Error; err != nil {
		t.Fatal(err)
	}
	categories := []Category{
		{PlaylistID: playlist.ID, CategoryName: "Enabled category", Active: true},
		{PlaylistID: playlist.ID, CategoryName: "Disabled category", Active: false},
	}
	if err := DB.Create(&categories).Error; err != nil {
		t.Fatal(err)
	}
	channels := []Channel{
		{CategoryID: categories[0].ID, Name: "Visible channel", EpgChannelID: "visible.id", StreamIcon: "https://example.test/logo.png"},
		{CategoryID: categories[1].ID, Name: "Disabled channel"},
		{CategoryID: categories[0].ID, Name: "### placeholder"},
	}
	if err := DB.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/playlist/:playlistID", ExportPlaylistM3U8Handler)

	request := httptest.NewRequest(http.MethodGet, "/playlist/1.m3u8", nil)
	request.Host = "iptv.example.test:5004"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("playlist endpoint returned %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/vnd.apple.mpegurl") {
		t.Errorf("unexpected content type %q", got)
	}

	body := response.Body.String()
	for _, expected := range []string{
		"#EXTM3U\n",
		`tvg-id="visible.id"`,
		`tvg-name="Visible channel"`,
		`tvg-logo="https://example.test/logo.png"`,
		`group-title="Enabled category"`,
		"http://iptv.example.test:5004/hls/" + strconv.Itoa(channels[0].ID) + ".ts",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("playlist does not contain %q:\n%s", expected, body)
		}
	}
	for _, excluded := range []string{"Disabled category", "Disabled channel", "### placeholder"} {
		if strings.Contains(body, excluded) {
			t.Errorf("playlist unexpectedly contains %q:\n%s", excluded, body)
		}
	}
}
