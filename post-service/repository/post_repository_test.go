package repository

import (
	"strings"
	"testing"
	"time"
)

func TestSuggestedPostQueriesUseUndirectedFriendRelationships(t *testing.T) {
	pageTypes := []string{PageTypeRelevant, PageTypeTime, PageTypeFriendOnly}

	for _, pageType := range pageTypes {
		query := getSuggestedPostsQuery(pageType)
		if strings.Contains(query, "[:FRIEND]->") || strings.Contains(query, "[friendship:FRIEND]->") || strings.Contains(query, "[originFriendship:FRIEND]->") {
			t.Fatalf("expected %s newsfeed query to use undirected FRIEND relationships:\n%s", pageType, query)
		}
	}
}

func TestMarkPostsLoadedQueryIncrementsLoadedTimes(t *testing.T) {
	query := getMarkPostsLoadedQuery()

	if !strings.Contains(query, "MERGE (viewer)-[l:LOADED]->(p)") {
		t.Fatal("expected mark loaded query to record loaded posts")
	}
	if !strings.Contains(query, "ON MATCH SET l.times = coalesce(l.times, 0) + 1") {
		t.Fatal("expected mark loaded query to increment loaded times")
	}
}

func TestRelevantNewsfeedCandidateQueryReturnsScoringFeatures(t *testing.T) {
	query := getRelevantNewsfeedCandidateQuery()

	for _, fragment := range []string{
		"AND NOT (viewer)-[:BLOCK]-(author)",
		"p.privacy = 'PUBLIC'",
		"author.id = viewer.id",
		"p.privacy = 'FRIEND' AND EXISTS((viewer)-[:FRIEND]-(author))",
		"WHEN block IS NOT NULL THEN false",
		"WHEN origin.privacy = 'PRIVATE' AND viewer.id = originAuthor.id THEN true",
		"coalesce(vu.times, 0) AS viewForward",
		"coalesce(uv.times, 0) AS viewBackward",
		"COALESCE(SUM(inter.score), 0) AS keywordScore",
		"coalesce(loaded.times, 0), keywordScore",
		"EXISTS((viewer)-[:FRIEND]-()-[:FRIEND]-(author)) OR EXISTS((viewer)-[:REQUEST]-(author))",
	} {
		requireQueryContains(t, PageTypeRelevant, query, fragment)
	}

	for _, legacyScoreFragment := range []string{"totalScore", "loadedScore", "newPostScore", "relationshipScore"} {
		if strings.Contains(query, legacyScoreFragment) {
			t.Fatalf("expected relevant candidate query to leave %s scoring to Go:\n%s", legacyScoreFragment, query)
		}
	}
}

func TestSuggestedPostQueriesKeepVisibilityAndBlockGuards(t *testing.T) {
	pageTypes := []string{PageTypeRelevant, PageTypeTime, PageTypeFriendOnly}

	for _, pageType := range pageTypes {
		query := getSuggestedPostsQuery(pageType)

		requireQueryContains(t, pageType, query, "AND NOT (viewer)-[:BLOCK]-(author)")
		requireQueryContains(t, pageType, query, "WHEN block IS NOT NULL THEN false")
		requireQueryContains(t, pageType, query, "WHEN origin.privacy = 'FRIEND' AND (viewer.id = originAuthor.id OR originFriendship IS NOT NULL) THEN true")
		requireQueryContains(t, pageType, query, "WHEN origin.privacy = 'PRIVATE' AND viewer.id = originAuthor.id THEN true")

		if strings.Contains(query, "origin.privacy = 'PRIVATE' AND viewer.id <> originAuthor.id") {
			t.Fatalf("expected %s query to hide private shared origins from non-authors:\n%s", pageType, query)
		}
	}
}

func TestSuggestedPostQueriesKeepPostPrivacyGuards(t *testing.T) {
	tests := map[string][]string{
		PageTypeRelevant: {
			"p.privacy = 'PUBLIC'",
			"author.id = viewer.id",
			"p.privacy = 'FRIEND' AND EXISTS((viewer)-[:FRIEND]-(author))",
		},
		PageTypeTime: {
			"p.privacy = 'PUBLIC'",
			"author.id = viewer.id",
			"p.privacy = 'FRIEND' AND friendship IS NOT NULL",
		},
		PageTypeFriendOnly: {
			"p.privacy IN ['PUBLIC', 'FRIEND']",
		},
	}

	for pageType, expectedFragments := range tests {
		query := getSuggestedPostsQuery(pageType)
		for _, fragment := range expectedFragments {
			requireQueryContains(t, pageType, query, fragment)
		}
	}
}

func requireQueryContains(t *testing.T, pageType, query, fragment string) {
	t.Helper()

	if !strings.Contains(query, fragment) {
		t.Fatalf("expected %s query to contain %q:\n%s", pageType, fragment, query)
	}
}

func TestNewsfeedCandidateFromRecordMapsPostAndFeatures(t *testing.T) {
	createdAt := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	candidate := newsfeedCandidateFromRecord([]interface{}{
		"post-1", "hello", "PUBLIC", createdAt, nil,
		"author-1", int64(4), true,
		[]interface{}{"file-1"}, int64(2), int64(1),
		nil, nil, true, false,
		nil, nil, nil, nil, nil,
		int64(3), int64(2), int64(5), int64(8), true, false,
	})

	if candidate.Post.ID != "post-1" {
		t.Fatalf("expected post id post-1, got %s", candidate.Post.ID)
	}
	if !candidate.Post.Liked {
		t.Fatal("expected candidate post to be liked")
	}
	if candidate.ViewForward != 3 || candidate.ViewBackward != 2 {
		t.Fatalf("expected view counts 3/2, got %d/%d", candidate.ViewForward, candidate.ViewBackward)
	}
	if candidate.LoadedTimes != 5 {
		t.Fatalf("expected loaded times 5, got %d", candidate.LoadedTimes)
	}
	if candidate.KeywordScore != 8 {
		t.Fatalf("expected keyword score 8, got %d", candidate.KeywordScore)
	}
	if !candidate.IsFriend {
		t.Fatal("expected candidate to be marked as friend")
	}
	if candidate.IsSecondDegreeOrRequested {
		t.Fatal("expected candidate not to be marked as second-degree/requested")
	}
}
