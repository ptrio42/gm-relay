package main

import (
	"context"
	"fmt"
	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"log"
	"net/http"
	"regexp"
	"time"
)

func main() {
	relay := khatru.NewRelay()

	relay.Info.Name = "GM Relay"
	relay.Info.PubKey = "f1f9b0996d4ff1bf75e79e4cc8577c89eb633e68415c7faf74cf17a07bf80bd8"
	relay.Info.Description = "A relay accepting only GM notes!"
	relay.Info.Icon = ""

	db := sqlite3.SQLite3Backend{DatabaseURL: "./db/db"}
	if err := db.Init(); err != nil {
		panic(err)
	}

	relay.StoreEvent = append(relay.StoreEvent, db.SaveEvent)
	relay.QueryEvents = append(relay.QueryEvents, db.QueryEvents)
	relay.DeleteEvent = append(relay.DeleteEvent, db.DeleteEvent)
	relay.CountEvents = append(relay.CountEvents, db.CountEvents)

	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {

		match, _ := regexp.MatchString(`(?mi)\bgm\b`, event.Content)
		cond := gmNoteNotPresentToday(db, ctx, event)

		if match && cond && !hasETag(event) {
			return false, ""
		}
		return true, "Only GM notes (and not replies) are allowed (once a day)!"
	})

	fmt.Println("running on :3336")

	http.ListenAndServe(":3336", relay)
}

func beginningOfTheDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func endOfTheDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 23, 59, 59, 0, t.Location())
}

func gmNoteNotPresentToday(db sqlite3.SQLite3Backend, ctx context.Context, event *nostr.Event) bool {
	t := time.Now()
	bod := nostr.Timestamp(beginningOfTheDay(t).Unix())
	eod := nostr.Timestamp(endOfTheDay(t).Unix())

	filter := nostr.Filter{
		Kinds:   []int{nostr.KindTextNote},
		Authors: []string{event.PubKey},
		Since:   &bod,
		Until:   &eod,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh, err := db.QueryEvents(ctx, filter)
	if err != nil {
		log.Fatalf("Failed to query events: %v", err)
	}

	for range eventCh {
		return false
	}
	return true
}

func hasETag(event *nostr.Event) bool {
	for _, tag := range event.Tags {
		if len(tag) > 0 && tag[0] == "e" {
			return true
		}
	}
	return false
}
