package moodle

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/apognu/gocal"
)

type CalendarEvent struct {
	UID         string    `json:"uid"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	Location    string    `json:"location,omitempty"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

func FetchCalendarEvents(url string, from time.Time, to time.Time) ([]CalendarEvent, error) {
	data, err := FetchCalendarData(url)
	if err != nil {
		return nil, err
	}
	return ParseCalendarEvents(data, from, to)
}

func FetchCalendarData(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/calendar,*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("calendar fetch failed: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func ParseCalendarEvents(data []byte, from time.Time, to time.Time) ([]CalendarEvent, error) {
	parser := gocal.NewParser(bytes.NewReader(data))
	parser.Start = &from
	parser.End = &to
	if err := parser.Parse(); err != nil {
		return nil, err
	}

	out := make([]CalendarEvent, 0, len(parser.Events))
	for _, ev := range parser.Events {
		start := time.Time{}
		end := time.Time{}
		if ev.Start != nil {
			start = *ev.Start
		}
		if ev.End != nil {
			end = *ev.End
		}
		out = append(out, CalendarEvent{
			UID:         ev.Uid,
			Summary:     ev.Summary,
			Description: ev.Description,
			Location:    ev.Location,
			Start:       start,
			End:         end,
		})
	}
	return out, nil
}
