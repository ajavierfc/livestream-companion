package stream

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grafov/m3u8"
)

type Stream struct {
	InputURL string
	Cmd      *exec.Cmd
	Buffer   *bytes.Buffer
	Client   http.ResponseWriter
}

func HandleTS(c *gin.Context, inputUrl string, id string, webbrowser string) {
	ctx := c.Request.Context()

	hlsDir := "./tmp"
	hlsFile := filepath.Join(hlsDir, id+".m3u8")

	cleanupTsFiles(hlsDir, id)

	// Start the FFMPEG command
	go func() {
		for {
			var cmd *exec.Cmd
			if webbrowser == "true" {
				log.Println("Converting audio for web browser compatibility.")
				cmd = exec.CommandContext(ctx,
					"ffmpeg",
					"-i",
					inputUrl,
					"-c:v", "copy",
					"-c:a", "libtwolame",
					"-f", "hls",
					"-hls_time", "2",
					"-hls_list_size", "6",
					"-sn",
					"-hls_flags", "delete_segments",
					"-hls_segment_filename", filepath.Join(hlsDir, id+"-%d.ts"),
					"-index_correction",
					"-ignore_io_errors",
					"-use_timeline", "0",
					hlsFile,
				)
			} else if webbrowser == "lq" {
				log.Println("Converting to low quality resolution.")
				cmd = exec.CommandContext(ctx,
					"ffmpeg",
					"-i",
					inputUrl,
					"-c:v:0", "libx264",
					"-c:a:0", "libtwolame",
					"-filter_complex", "[0:0]yadif@f1=mode=send_frame:parity=auto:deint=all,scale@f2=width=720:height=574[f2_out0]",
					"-map", "[f2_out0]",
					"-map", "0:1",
					"-sn",
					"-f", "segment",
					"-segment_format", "mpegts",
					"-segment_list", hlsFile,
					"-segment_list_type", "m3u8",
					"-segment_time", "00:00:03.000",
					"-maxrate:v:0", "1640000",
					"-bufsize:v:0", "1280000",
					"-sc_threshold:v:0", "0",
					"-keyint_min:v:0", "75",
					"-r:v:0", "25",
					"-pix_fmt:v:0", "yuv420p",
					"-preset:v:0", "veryfast",
					"-profile:v:0", "high",
					"-x264opts:v:0", "subme=0:me_range=4:rc_lookahead=10:partitions=none",
					"-crf:v:0", "23",
					"-hls_time", "2",
					"-hls_list_size", "6",
					"-sn",
					"-hls_flags", "delete_segments",
					"-hls_segment_filename", filepath.Join(hlsDir, id+"-%d.ts"),
					"-index_correction",
					"-ignore_io_errors",
					"-use_timeline", "0",
					filepath.Join(hlsDir, id+"-%d.ts"),
				)
			} else {
					cmd = exec.CommandContext(ctx,
					"ffmpeg",
					"-i",
					inputUrl,
					"-c", "copy",
					"-f", "hls",
					"-hls_time", "2",
					"-hls_list_size", "6",
					"-sn",
					"-hls_flags", "delete_segments",
					"-hls_segment_filename", filepath.Join(hlsDir, id+"-%d.ts"),
					hlsFile,
				)
			}

			if err := cmd.Start(); err != nil {
				log.Println("FFMPEG process couldn't start:", err)
				return
			}

			err := cmd.Wait()

			if err != nil {
				log.Println("FFMPEG process stopped unexpectedly:", err)
			}

			select {
			case <-ctx.Done():
				log.Println("Client disconnected")
				cleanupTsFiles(hlsDir, id)
				return
			default:
				// Client is still connected, so we continue the loop and start FFMPEG again.
			}
		}
	}()

	w := c.Writer
	w.Header().Set("Content-Type", "video/mpeg")

	// Monitor the HLS folder for new .ts files
	for {
		select {
		case <-ctx.Done():
			log.Println("Client disconnected")
			return
		default:
			// Read playlist
			playlist, err := os.ReadFile(hlsFile)
			if err != nil {
				time.Sleep(time.Second)
				continue
			}

			p, listType, err := m3u8.DecodeFrom(bufio.NewReader(strings.NewReader(string(playlist))), true)
			if err != nil {
				log.Printf("Error decoding playlist: %v", err)
				time.Sleep(time.Second)
				continue
			}

			switch listType {
			case m3u8.MEDIA:
				mediapl := p.(*m3u8.MediaPlaylist)

				// Iterate through all segments in the playlist
				for _, v := range mediapl.Segments {
					if v != nil {
						filePath := filepath.Join(hlsDir, v.URI)
						segment, err := os.Open(filePath)
						if err != nil {
							continue
						}

						// Write to the HTTP ResponseWriter from the segment file
						io.Copy(w, segment)
						segment.Close()

						// Delete the segment file
						if err = os.Remove(filePath); err != nil {
							log.Printf("Error deleting segment file: %v", err)
						}
					}
				}
			default:
				log.Printf("Unknown playlist type.")
			}

			time.Sleep(time.Second)
		}
	}
}

func cleanupTsFiles(dir string, id string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	prefix := id + "-"
	for _, f := range files {
		if strings.HasPrefix(f.Name(), prefix) {
			err := os.Remove(filepath.Join(dir, f.Name()))
			if err != nil {
				return err
			}
		}
	}

	// Deleting the m3u8 file
	m3u8File := filepath.Join(dir, id+".m3u8")
	err = os.Remove(m3u8File)
	if err != nil {
		return err
	}

	return nil
}
