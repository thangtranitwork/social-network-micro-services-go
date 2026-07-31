package service

import "testing"

func TestKafkaKeywordPublisherClose(t *testing.T) {
	pub := NewKafkaKeywordPublisher("")
	if err := pub.Close(); err != nil {
		t.Fatalf("expected nil error on close, got %v", err)
	}
}
