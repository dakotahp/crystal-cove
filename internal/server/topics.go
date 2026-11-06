package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/semantic"
)

const (
	listNotesSemanticHint = " For what the vault is about, use topic_map."
	topicMapDescription   = "Show what a vault is about: its notes grouped into topics by meaning, so you can find your " +
		"bearings in one call before searching. Call it at the start of a session when the task is open-ended. Each " +
		"topic lists key_words (they stand in for a name), how many notes it holds, the folders that hold most of it, " +
		"start_with notes to read first, and close_to topics. A note that covers several subjects counts in each topic " +
		"it touches; outside_notes counts notes that fit no topic. folders and exclude_folders map part of the vault. " +
		"To split one topic into sub-topics, pass its id as topic with the build from the same result; ids are valid " +
		"only for that build, for 30 minutes. To find notes on one subject, use search_notes."
)

type topicMapInput struct {
	Vault string `json:"vault,omitempty" jsonschema:"name of the vault; optional when the server holds one vault"`
	folderFilter
	Topic string `json:"topic,omitempty" jsonschema:"a topic id from an earlier topic_map call, to split that topic into sub-topics; needs build; folders and exclude_folders are ignored, since the build fixes the scope"`
	Build string `json:"build,omitempty" jsonschema:"the build from the call that gave topic"`
}

// TopicEntry is one topic of a topic map.
type TopicEntry struct {
	ID        string        `json:"id"`
	Notes     int           `json:"notes"`
	KeyWords  []string      `json:"key_words" jsonschema:"words most typical of this topic; they stand in for a name"`
	Folders   []TopicFolder `json:"folders" jsonschema:"folders holding most of the topic, with note counts"`
	StartWith []string      `json:"start_with" jsonschema:"notes that best represent the topic, to read first"`
	CloseTo   []string      `json:"close_to" jsonschema:"ids of the nearest other topics"`
}

// TopicFolder is how many of a topic's notes sit in one folder.
type TopicFolder struct {
	Folder string `json:"folder"`
	Notes  int    `json:"notes"`
}

// TopicMapResult is the outcome of topic_map.
type TopicMapResult struct {
	Build        string          `json:"build" jsonschema:"pass with topic to zoom in; valid for 30 minutes"`
	Topic        string          `json:"topic,omitempty" jsonschema:"the topic these are sub-topics of"`
	Topics       []TopicEntry    `json:"topics"`
	Notes        int             `json:"notes" jsonschema:"notes in scope"`
	OutsideNotes int             `json:"outside_notes" jsonschema:"notes in scope that fit no topic, such as notes that mix many subjects"`
	Index        semantic.Status `json:"index"`
}

func (s *Server) topicMap(_ context.Context, _ *mcp.CallToolRequest, in topicMapInput) (*mcp.CallToolResult, *TopicMapResult, error) {
	v, ix, err := s.semanticIndex(in.Vault)
	if err != nil {
		return nil, nil, err
	}
	var m semantic.TopicMap
	var status semantic.Status
	switch {
	case in.Topic != "" && in.Build == "":
		return nil, nil, errors.New("topic needs the build from the topic_map call that gave it")
	case in.Topic == "" && in.Build != "":
		return nil, nil, errors.New("build is only used with topic: pass the topic id to zoom into")
	case in.Topic != "":
		m, status, err = ix.Subtopics(in.Build, in.Topic)
	default:
		m, status, err = ix.Topics(scopeKey(in.folderFilter), in.includes)
	}
	switch {
	case errors.Is(err, semantic.ErrTooFewSections) && !status.Complete:
		return nil, nil, stillBuilding(v, status)
	case errors.Is(err, semantic.ErrTooFewSections):
		return nil, nil, fmt.Errorf("%w; widen folders or check that the vault has notes", err)
	case errors.Is(err, semantic.ErrStaleBuild):
		return nil, nil, fmt.Errorf("%w; call topic_map again without topic", err)
	case errors.Is(err, semantic.ErrTooSmallToSplit):
		return nil, nil, fmt.Errorf("%w; read its start_with notes or use search_notes", err)
	case err != nil:
		return nil, nil, err
	}
	topics := make([]TopicEntry, len(m.Topics))
	for i, t := range m.Topics {
		folders := make([]TopicFolder, len(t.Folders))
		for j, f := range t.Folders {
			folders[j] = TopicFolder{Folder: f.Folder, Notes: f.Notes}
		}
		topics[i] = TopicEntry{ID: t.ID, Notes: t.Notes, KeyWords: t.KeyWords, Folders: folders, StartWith: t.StartWith, CloseTo: t.CloseTo}
	}
	return nil, &TopicMapResult{Build: m.Build, Topic: m.Parent, Topics: topics, Notes: m.Notes, OutsideNotes: m.OutsideNotes, Index: status}, nil
}

// scopeKey names a folder filter the same way however its folders are
// written or ordered, so equal filters share a build.
func scopeKey(f folderFilter) string {
	var parts []string
	for _, d := range f.Folders {
		parts = append(parts, "+"+strings.Trim(d, "/"))
	}
	for _, d := range f.ExcludeFolders {
		parts = append(parts, "-"+strings.Trim(d, "/"))
	}
	slices.Sort(parts)
	return strings.Join(parts, "\x00")
}
