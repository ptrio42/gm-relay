package main

import (
	"context"
	"fmt"
	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	relays = []string{
		"wss://wot.swarmstr.com",
		"wss://nos.lol",
		"wss://nostr.mom",
		"wss://nostr.wine",
		"wss://relay.damus.io",
		"wss://wot.utxo.one",
	}
	pubkey    string
	relay     = khatru.NewRelay()
	pool      = nostr.NewSimplePool(context.Background())
	allGmsUrl = "https://nostrrr.com/relay/gm.swarmstr.com"
)

func main() {

	relay.Info.Name = "GM Relay"
	relay.Info.PubKey = "f1f9b0996d4ff1bf75e79e4cc8577c89eb633e68415c7faf74cf17a07bf80bd8"
	relay.Info.Description = "A relay accepting only GM notes!"
	relay.Info.Icon = "https://image.nostr.build/939b3ec044365698b25aff993aac4c220657d54baf1a6b949fec0383800a4a9b.jpg"

	godotenv.Load(".env")
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

	fmt.Println("running on :3336")

	go handleNewGMBotRequestMultipleRelays(db, relays, pubkey)

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
			publishEvent(ev, getStats(db, pubkey))
		}
		//} else if match1 && !alreadyReplied(ev.ID, ev.PubKey) {
		//
		//}
	}

	//}
}

func handleNewGMBotRequestMultipleRelays(db sqlite3.SQLite3Backend, relays []string, pubkey string) {
	ctx := context.Background()

	tags := make(nostr.TagMap)
	tags["p"] = []string{pubkey}
	filter := nostr.Filter{
		Kinds: []int{nostr.KindTextNote},
		Tags:  tags,
	}

	for ev := range pool.SubMany(ctx, relays, []nostr.Filter{filter}) {
		isStatsRequest, _ := regexp.MatchString(`(?mi)\bstats\b`, ev.Content)
		isTotalRequest, _ := regexp.MatchString(`(?mi)\btotal\b`, ev.Content)
		isTopRequest, _ := regexp.MatchString(`(?mi)\btop\b`, ev.Content)
		isMissedRequest, _ := regexp.MatchString(`(?mi)\bmissed\b`, ev.Content)

		switch {
		case isStatsRequest && !alreadyReplied(ev.ID, pubkey):
			fmt.Printf("Processing 'stats' request.\n")
			publishEvent(ev.Event, getStats(db, ev.PubKey))

		case isTotalRequest && !alreadyReplied(ev.ID, pubkey):
			fmt.Printf("Processing 'total' request.\n")
			publishEvent(ev.Event, getGmsTotal(db))

		case isTopRequest && !alreadyReplied(ev.ID, pubkey):
			fmt.Printf("Processing 'top' request.\n")
			publishEvent(ev.Event, getUserWithMostGms(db))

		case isMissedRequest && !alreadyReplied(ev.ID, pubkey):
			fmt.Printf("Processing 'missed' request.\n")

		default:
			// Optionally handle the case where none of the conditions match
			fmt.Printf("No valid request to process.\n")
		}
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

	result := fmt.Sprintf("%v GMs in last %v days.\nFirst GM recorded on %v\n\nView all notes at %v", gmCount, gmDays, firstGmDate, allGmsUrl)
	return result
}

func daysBetweenDates(startTime time.Time, endTime time.Time) int {
	duration := beginningOfTheDay(startTime).Sub(beginningOfTheDay(endTime))
	fmt.Printf("hours %s\n", duration)
	days := int(duration.Hours() / 24)
	return days
}

func publishEvent(ev *nostr.Event, content string) {
	event := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindTextNote,
		Content:   content,
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

	tags := make(nostr.TagMap)
	tags["e"] = []string{ID}
	filter := nostr.Filter{
		Kinds:   []int{nostr.KindTextNote},
		Tags:    tags,
		Authors: []string{pubkey},
	}

	for range pool.SubManyEose(ctx, relays, []nostr.Filter{filter}) {
		fmt.Println("Already replied")
		return true
	}
	return false
}

func getGmsTotal(db sqlite3.SQLite3Backend) string {
	ctx := context.Background()
	filter := nostr.Filter{
		Kinds: []int{nostr.KindTextNote},
	}

	iCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	count, err := db.CountEvents(iCtx, filter)
	if err != nil {
		log.Fatalf("Failed to count events: %v", err)
	}

	result := fmt.Sprintf("%v GMs stored in total.\n\nView all notes at %v", count, allGmsUrl)
	return result
}

func getUserWithMostGms(db sqlite3.SQLite3Backend) string {
	ctx := context.Background()
	filter := nostr.Filter{
		Kinds: []int{nostr.KindTextNote},
	}

	iCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventCh, err := db.QueryEvents(iCtx, filter)
	if err != nil {
		log.Fatalf("Failed to query events: %v", err)
	}

	groupedEvents := make(map[string][]*nostr.Event)
	for ev := range eventCh {
		groupedEvents[ev.PubKey] = append(groupedEvents[ev.PubKey], ev)
	}

	pubkey, gmCount := findTopUser(groupedEvents)
	npub, err := nip19.EncodePublicKey(pubkey)
	if err != nil {
		log.Fatalf("Failed to encode public key: %v", err)
	}
	bech32EncodedEntity := fmt.Sprintf("nostr:%v", npub)

	result := fmt.Sprintf("Person with most GMs is %v (%v total).\n\nView all notes at %v", bech32EncodedEntity, gmCount, allGmsUrl)
	return result
}

func findTopUser(groupedEvents map[string][]*nostr.Event) (string, int) {
	var largestKey string
	maxLength := 0

	for key, events := range groupedEvents {
		if len(events) > maxLength {
			maxLength = len(events)
			largestKey = key
		}
	}

	return largestKey, maxLength
}

func getTodaysMissedGmNotesFromFollows(pubkey string, db sqlite3.SQLite3Backend) string {
	follows := getUserFollows(pubkey)
	todaysGmNotes := getAllGmsFromToday(db)

	entities := []string{}
	for _, ev := range todaysGmNotes {
		if slices.Contains(follows, ev.PubKey) && !alreadyReplied(ev.ID, pubkey) {
			note, err := nip19.EncodeNote(ev.ID)
			if err != nil {
				log.Fatalf("Failed to encode public key: %v", err)
			}
			entities = append(entities, fmt.Sprintf("nostr:%v", note))
		}
	}
	message := fmt.Sprintf("GMs from follows you've missed today:\n\n%s", strings.Join(entities, ",\n"))
	return message
}

func getUserFollows(pubkey string) []string {
	filter := nostr.Filter{
		Kinds:   []int{nostr.KindFollowList},
		Authors: []string{pubkey},
	}

	for eventCh := range pool.SubManyEose(context.Background(), relays, []nostr.Filter{filter}) {
		return extractTagValues(eventCh.Tags, "p")
	}
	return []string{}
}

func extractTagValues(tags nostr.Tags, key string) []string {
	result := []string{}
	for _, tag := range tags {
		if len(tag) > 1 && tag[1] == key {
			result = append(result, tag[1])
		}
	}
	return result
}

func getAllGmsFromToday(db sqlite3.SQLite3Backend) []*nostr.Event {
	events := []*nostr.Event{}

	ctx := context.Background()
	t := time.Now()
	bod := nostr.Timestamp(beginningOfTheDay(t).Unix())
	eod := nostr.Timestamp(endOfTheDay(t).Unix())

	filter := nostr.Filter{
		Kinds: []int{nostr.KindTextNote},
		Since: &bod,
		Until: &eod,
	}

	iCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventCh, err := db.QueryEvents(iCtx, filter)
	if err != nil {
		log.Fatalf("Failed to query events: %v", err)
	}

	for ev := range eventCh {
		events = append(events, ev)
	}
	return events
}
