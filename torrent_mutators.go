package transmissionrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

/*
	Torrent Mutators
    https://github.com/transmission/transmission/blob/4.0.3/docs/rpc-spec.md#32-torrent-mutator-torrent-set
*/
// Compact replaces consecutive runs of equal elements with a single copy.
// This is like the uniq command found on Unix.
// Compact modifies the contents of the slice s and returns the modified slice,
// which may have a smaller length.
// Compact zeroes the elements between the new length and the original length.
// todo replace to slices.Compact on version 1.21
func compact[S ~[]E, E comparable](s S) S {
	if len(s) < 2 {
		return s
	}
	for k := 1; k < len(s); k++ {
		if s[k] == s[k-1] {
			s2 := s[k:]
			for k2 := 1; k2 < len(s2); k2++ {
				if s2[k2] != s2[k2-1] {
					s[k] = s2[k2]
					k++
				}
			}
			return s[:k]
		}
	}
	return s
}

// TorrentSet apply a list of mutator(s) to a list of torrent ids.
func (c *Client) TorrentSet(ctx context.Context, payload TorrentSetPayload) (err error) {
	// Validate
	if len(payload.IDs) == 0 {
		return errors.New("there must be at least one ID")
	}
	//fix trackers
	sort.Strings(payload.TrackerList)
	payload.TrackerList = compact(payload.TrackerList)
	// Send payload
	if err = c.rpcCall(ctx, "torrent-set", payload, nil); err != nil {
		err = fmt.Errorf("'torrent-set' rpc method failed: %w", err)
	}
	return
}

// TorrentSetPayload contains all the mutators appliable on one torrent.
type TorrentSetPayload struct {
	BandwidthPriority   *int64         `json:"bandwidthPriority"`   // this torrent's bandwidth tr_priority_t
	DownloadLimit       *int64         `json:"downloadLimit"`       // maximum download speed (KBps)
	DownloadLimited     *bool          `json:"downloadLimited"`     // true if "downloadLimit" is honored
	FilesWanted         []int64        `json:"files-wanted"`        // indices of file(s) to download
	FilesUnwanted       []int64        `json:"files-unwanted"`      // indices of file(s) to not download
	Group               *string        `json:"group"`               // bandwidth group to add torrent to
	HonorsSessionLimits *bool          `json:"honorsSessionLimits"` // true if session upload limits are honored
	IDs                 []int64        `json:"ids"`                 // torrent list
	Labels              []string       `json:"labels"`              // RPC v16: strings of user-defined labels
	Location            *string        `json:"location"`            // new location of the torrent's content
	PeerLimit           *int64         `json:"peer-limit"`          // maximum number of peers
	PriorityHigh        []int64        `json:"priority-high"`       // indices of high-priority file(s)
	PriorityLow         []int64        `json:"priority-low"`        // indices of low-priority file(s)
	PriorityNormal      []int64        `json:"priority-normal"`     // indices of normal-priority file(s)
	QueuePosition       *int64         `json:"queuePosition"`       // position of this torrent in its queue [0...n)
	SeedIdleLimit       *time.Duration `json:"-"`                   // torrent-level number of minutes of seeding inactivity
	SeedIdleMode        *int64         `json:"seedIdleMode"`        // which seeding inactivity to use
	SeedRatioLimit      *float64       `json:"seedRatioLimit"`      // torrent-level seeding ratio
	SeedRatioMode       *SeedRatioMode `json:"seedRatioMode"`       // which ratio mode to use
	TrackerList         []string       `json:"-"`                   // string of announce URLs, one per line, and a blank line between tiers
	UploadLimit         *int64         `json:"uploadLimit"`         // maximum upload speed (KBps)
	UploadLimited       *bool          `json:"uploadLimited"`       // true if "uploadLimit" is honored
}

// MarshalJSON allows to marshall into JSON only the non nil fields.
// It differs from 'omitempty' which also skip default values
// (as 0 or false which can be valid here).
func (tsp TorrentSetPayload) MarshalJSON() (data []byte, err error) {
	// Build an intermediary payload with base types
	type baseTorrentSetPayload TorrentSetPayload
	tmp := struct {
		SeedIdleLimit *int64  `json:"seedIdleLimit"`
		TrackerList   *string `json:"trackerList"`
		*baseTorrentSetPayload
	}{
		baseTorrentSetPayload: (*baseTorrentSetPayload)(&tsp),
	}
	if tsp.SeedIdleLimit != nil {
		sil := int64(*tsp.SeedIdleLimit / time.Minute)
		tmp.SeedIdleLimit = &sil
	}
	if tsp.TrackerList != nil {
		oneLineList := strings.Join(tsp.TrackerList, "\n")
		tmp.TrackerList = &oneLineList
	}
	// Build a payload with only the non nil fields
	tspv := reflect.ValueOf(tmp)
	tspt := tspv.Type()
	cleanPayload := make(map[string]interface{}, tspt.NumField())
	var currentValue, nestedStruct, currentNestedValue reflect.Value
	var currentStructField, currentNestedStructField reflect.StructField
	var j int
	for i := 0; i < tspv.NumField(); i++ {
		currentValue = tspv.Field(i)
		currentStructField = tspt.Field(i)
		if !currentValue.IsNil() {
			if currentStructField.Name == "baseTorrentSetPayload" {
				// inherited/nested struct
				nestedStruct = reflect.Indirect(currentValue)
				for j = 0; j < nestedStruct.NumField(); j++ {
					currentNestedValue = nestedStruct.Field(j)
					currentNestedStructField = nestedStruct.Type().Field(j)
					if !currentNestedValue.IsNil() {
						JSONKeyName := currentNestedStructField.Tag.Get("json")
						if JSONKeyName != "-" {
							cleanPayload[JSONKeyName] = currentNestedValue.Interface()
						}
					}
				}
			} else {
				// Overloaded field
				cleanPayload[currentStructField.Tag.Get("json")] = currentValue.Interface()
			}
		}
	}
	// Marshall the clean payload
	return json.Marshal(cleanPayload)
}
