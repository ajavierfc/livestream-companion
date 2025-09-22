package management

import (
	"strconv"
	"time"
)

// Structure for user information
type UserInfo struct {
	Username            string   `json:"username"`
	Password            string   `json:"password"`
	Message             string   `json:"message"`
	Auth                int      `json:"auth"`
	Status              string   `json:"status"`
	ExpirationDate      string   `json:"exp_date"`
	IsTrial             string   `json:"is_trial"`
	CreatedAt           string   `json:"created_at"`
	MaxConnections      string   `json:"max_connections"`
	AllowedOutputFormats []string `json:"allowed_output_formats"`
}
//	ActiveConnections   string   `json:"active_cons"`

// Structure for server information
type ServerInfo struct {
	URL            string `json:"url"`
	Port           string `json:"port"`
	HTTPSPort      string `json:"https_port"`
	ServerProtocol string `json:"server_protocol"`
	RTMPPort       string `json:"rtmp_port"`
	Timezone       string `json:"timezone"`
	TimestampNow   int64  `json:"timestamp_now"`
	TimeNow        string `json:"time_now"`
}

type XtreamInfo struct {
	UserInfo UserInfo     `json:"user_info"`
	ServerInfo ServerInfo `json:"server_info"`
}

func IsDateBeforeCurrent(secondsStr string) (bool) {
	seconds, err := strconv.ParseInt(secondsStr, 10, 64)
	if err != nil {
		return false
	}
	date := time.Unix(seconds, 0)
	currentDate := time.Now()
	return date.Before(currentDate)
}
