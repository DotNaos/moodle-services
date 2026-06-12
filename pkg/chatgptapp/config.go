package chatgptapp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

const (
	envDatabaseURL = "DATABASE_URL"
	envCalendarURL = "MOODLE_CALENDAR_URL"
)

type Config struct {
	DatabaseURL string
	CalendarURL string
}

func LoadConfigFromEnv() (Config, error) {
	return Config{
		DatabaseURL: strings.TrimSpace(os.Getenv(envDatabaseURL)),
		CalendarURL: strings.TrimSpace(os.Getenv(envCalendarURL)),
	}, nil
}

func ClientFromMobileSessionJSON(raw string) (DataClient, error) {
	var session moodle.MobileSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, fmt.Errorf("decode mobile session: %w", err)
	}
	return moodle.NewMobileClient(session, session.ResolvedSchoolID())
}
