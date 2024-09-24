package main

import (
	"context"
	"fmt"
	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"
)

var relays []string
var pubkey string

func main() {
	godotenv.Load(".env")

	relay := khatru.NewRelay()

	relay.Info.Name = "GM Relay"
	relay.Info.PubKey = "f1f9b0996d4ff1bf75e79e4cc8577c89eb633e68415c7faf74cf17a07bf80bd8"
	relay.Info.Description = "A relay accepting only GM notes!"
	relay.Info.Icon = ""

	pubkey, _ = nostr.GetPublicKey(getEnv("GM_BOT_PRIVATE_KEY"))

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
		cond := gmNoteNotPresentToday(db, event)

		if match && cond && !hasETag(event) {
			return false, ""
		}
		return true, "Only GM notes (and not replies) are allowed (once a day)!"
	})

	relays = []string{
		"wss://wot.swarmstr.com",
		"wss://nos.lol",
		"wss://nostr.mom",
		"wss://nostr.wine",
		"wss://relay.damus.io",
	}

	fmt.Println("running on :3336")

	go handleNewGMBotRequest(db, relays)

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

func gmNoteNotPresentToday(db sqlite3.SQLite3Backend, event *nostr.Event) bool {
	ctx := context.Background()
	t := time.Now()
	bod := nostr.Timestamp(beginningOfTheDay(t).Unix())
	eod := nostr.Timestamp(endOfTheDay(t).Unix())

	filter := nostr.Filter{
		Kinds:   []int{nostr.KindTextNote},
		Authors: []string{event.PubKey},
		Since:   &bod,
		Until:   &eod,
	}

	iCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	eventCh, err := db.QueryEvents(iCtx, filter)
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

func handleNewGMBotRequest(db sqlite3.SQLite3Backend, relays []string) {
	ctx := context.Background()

	relay, err := nostr.RelayConnect(ctx, relays[4])
	if err != nil {
		panic(err)
	}
	tags := make(nostr.TagMap)
	tags["p"] = []string{pubkey}
	filter := nostr.Filter{
		Kinds: []int{nostr.KindTextNote},
		Tags:  tags,
	}

	//iCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	//defer cancel()

	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		panic(err)
	}

	for ev := range sub.Events {
		fmt.Println("New stats request")
		match, _ := regexp.MatchString(`(?mi)\bstats\b`, ev.Content)
		//match1, _ := regexp.MatchString(`(?mi)\btotal\b`, ev.Content)

		if match && !alreadyReplied(ev.ID, pubkey) {
			publishStats(db, ev)
		}
		//} else if match1 && !alreadyReplied(ev.ID, pubkey) {
		//
		//}
	}
}

func getStats(db sqlite3.SQLite3Backend, pubkey string) string {
	fmt.Printf(pubkey)
	ctx := context.Background()

	filter := nostr.Filter{
		Kinds:   []int{nostr.KindTextNote},
		Authors: []string{pubkey},
	}

	iCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	eventCh, err := db.QueryEvents(iCtx, filter)
	if err != nil {
		log.Fatalf("Failed to query events: %v", err)
	}

	var oldestGm *nostr.Event
	var firstGmDate string
	gmCount := 0
	for ev := range eventCh {
		oldestGm = ev
		gmCount += 1
	}
	if gmCount == 0 {
		return "No GMs found!\nIf you want your GMs to be stored,\nadd wss://gm.swarmstr.com to your relay list."
	}
	var gmDays int
	if oldestGm != nil {
		gmDays = daysBetweenDates(time.Now(), oldestGm.CreatedAt.Time())
		firstGmDate = oldestGm.CreatedAt.Time().Format("2006-01-02")
	}

	result := fmt.Sprintf("%v GMs in last %v days.\nFirst GM recorded on %v\n\nView all notes at https://nostrrr.com/relay/gm.swarmstr.com", gmCount, gmDays, firstGmDate)
	return result
}

func daysBetweenDates(startTime time.Time, endTime time.Time) int {
	duration := beginningOfTheDay(startTime).Sub(beginningOfTheDay(endTime))
	fmt.Printf("hours %s\n", duration)
	days := int(duration.Hours() / 24)
	return days
}

func publishStats(db sqlite3.SQLite3Backend, ev *nostr.Event) {
	event := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindTextNote,
		Content:   getStats(db, ev.PubKey),
		Tags:      []nostr.Tag{[]string{"e", ev.ID}, []string{"p", ev.PubKey}},
	}
	event.Sign(getEnv("GM_BOT_PRIVATE_KEY"))

	ctx := context.Background()
	for _, url := range relays {
		relay, err := nostr.RelayConnect(ctx, url)
		if err != nil {
			fmt.Println(err)
			continue
		}
		if err := relay.Publish(ctx, event); err != nil {
			fmt.Println(err)
			continue
		}

		fmt.Printf("published to %s\n", url)
	}
}

func getEnv(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		log.Fatalf("Environment variable %s not set", key)
	}
	return value
}

func alreadyReplied(ID string, pubkey string) bool {
	fmt.Println("Checking if request was already fulfilled")
	ctx := context.Background()

	relay, err := nostr.RelayConnect(ctx, relays[2])
	if err != nil {
		panic(err)
	}
	tags := make(nostr.TagMap)
	tags["e"] = []string{ID}
	filter := nostr.Filter{
		Kinds:   []int{nostr.KindTextNote},
		Tags:    tags,
		Authors: []string{pubkey},
	}

	iCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	sub, err := relay.Subscribe(iCtx, []nostr.Filter{filter})
	if err != nil {
		panic(err)
	}

	for range sub.Events {
		fmt.Println("Already replied")
		return true
	}
	return false
}
