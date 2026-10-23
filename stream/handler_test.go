package stream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaylistReady(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seg-0.ts"), []byte("ts"), 0644); err != nil {
		t.Fatal(err)
	}

	ready := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2.0,\nseg-0.ts\n#EXTINF:2.0,\nseg-1.ts\n"
	if !playlistReady(dir, ready) {
		t.Fatal("playlist with an existing first segment should be ready")
	}

	missingFirst := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:1\n#EXTINF:2.0,\nseg-1.ts\n#EXTINF:2.0,\nseg-0.ts\n"
	if playlistReady(dir, missingFirst) {
		t.Fatal("playlist whose first segment is already gone should not be ready")
	}

	if playlistReady(dir, "#EXTM3U\n#EXT-X-VERSION:3\n") {
		t.Fatal("playlist without segments should not be ready")
	}
}
