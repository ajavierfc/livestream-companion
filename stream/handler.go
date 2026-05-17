package stream

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grafov/m3u8"
)

const hlsDir = "./tmp"

var (
	hlsProcsMu sync.Mutex
	hlsProcs   = map[string]*hlsProcess{}
)

type hlsProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

type Stream struct {
	InputURL string
	Cmd      *exec.Cmd
	Buffer   *bytes.Buffer
	Client   http.ResponseWriter
}

type reHijackWriter struct {
	gin.ResponseWriter
	conn *net.TCPConn
}

func (w *reHijackWriter) Write(b []byte) (int, error) {
	n, err := w.conn.Write(b)
	if err != nil {
		// Aquí detectas la desconexión REAL
		return n, err
	}
	return n, nil
}

func HandleM3U8(c *gin.Context, inputUrl string, id string) {
	if err := startHLSRemux(inputUrl, id); err != nil {
		log.Println("Failed to start HLS remux:", err)
		c.String(http.StatusInternalServerError, "Failed to start HLS remux")
		return
	}

	hlsFile := filepath.Join(hlsDir, id+".m3u8")

	playlist, err := waitForPlaylist(hlsFile, 15*time.Second)
	if err != nil {
		log.Println("Failed to load playlist:", err)
		c.String(http.StatusInternalServerError, "Playlist not ready")
		return
	}

	playlist = rewritePlaylistURIs(playlist)

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-cache")
	c.Header("Pragma", "no-cache")
	c.String(http.StatusOK, playlist)
}

func HandleTS(c *gin.Context, inputUrl string, id string, webbrowser string) {
	ctx := c.Request.Context()

	if hj, ok := c.Writer.(http.Hijacker); ok {
		conn, _, err := hj.Hijack()
		if err == nil {
			tcpConn := conn.(*net.TCPConn)
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(5 * time.Second)

			// Volvemos a envolver el writer para seguir usando Gin
			c.Writer = &reHijackWriter{ResponseWriter: c.Writer, conn: tcpConn}
		}
	}

	hlsFile := filepath.Join(hlsDir, id+".m3u8")

	cleanupTsFiles(hlsDir, id)

	// Start the FFMPEG command
	go func() {
		select {
		case <-ctx.Done():
			log.Println("Client disconnected before starting ffmpeg")
			return
		default:
		}

		var cmd *exec.Cmd
		if webbrowser == "true" {
			log.Println("Converting audio for web browser compatibility.")
			cmd = exec.CommandContext(ctx,
				"./bin/ffmpeg",
				"-i",
				inputUrl,
				"-c:v", "copy",
				"-c:a", "libmp3lame",
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
				"./bin/ffmpeg",
				"-i",
				inputUrl,
				"-c:v:0", "libx264",
				"-c:a:0", "libmp3lame",
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
				"./bin/ffmpeg",
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
	}()

	w := c.Writer
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Transfer-Encoding", "chunked")

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
						_, err = io.Copy(w, segment)
						segment.Close()
						if err != nil {
							log.Println("Client disconnected:", err)
							return
						}

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
		if os.IsNotExist(err) {
			return nil
		}
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
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func startHLSRemux(inputUrl string, id string) error {
	hlsProcsMu.Lock()
	if _, exists := hlsProcs[id]; exists {
		hlsProcsMu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc := &hlsProcess{cancel: cancel}
	hlsProcs[id] = proc
	hlsProcsMu.Unlock()

	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		return err
	}

	if err := cleanupTsFiles(hlsDir, id); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx,
		"./bin/ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputUrl,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "6",
		"-sn",
		"-hls_flags", "delete_segments",
		"-hls_segment_filename", filepath.Join(hlsDir, id+"-%d.ts"),
		filepath.Join(hlsDir, id+".m3u8"),
	)
	proc.cmd = cmd

	go func() {
		defer func() {
			hlsProcsMu.Lock()
			delete(hlsProcs, id)
			hlsProcsMu.Unlock()
		}()

		if err := cmd.Run(); err != nil {
			log.Printf("FFMPEG process for HLS id=%s stopped: %v", id, err)
		}
	}()

	return nil
}

func waitForPlaylist(path string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		playlist, err := os.ReadFile(path)
		if err == nil {
			return string(playlist), nil
		}
		if !os.IsNotExist(err) {
			log.Printf("Waiting for playlist, read error: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", os.ErrNotExist
}

func rewritePlaylistURIs(playlist string) string {
	var builder strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(playlist))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			builder.WriteString(line)
			builder.WriteByte('\n')
			continue
		}

		builder.WriteString("/hls/")
		builder.WriteString(filepath.Base(line))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func HandleTSegment(c *gin.Context, segmentName string) {
	w := c.Writer
	w.Header().Set("Content-Type", "video/mp2t")

	segmentName = filepath.Base(segmentName)
	segmentPath := filepath.Join(hlsDir, segmentName)
	segment, err := os.Open(segmentPath)
	if err != nil {
		log.Printf("Missing segment %s: %v", segmentName, err)
		c.Status(http.StatusNotFound)
		return
	}
	defer segment.Close()

	_, err = io.Copy(w, segment)
	if err != nil {
		log.Printf("Failed to send segment %s: %v", segmentName, err)
	}
}
