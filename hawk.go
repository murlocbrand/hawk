package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	ics "github.com/PuloV/ics-golang"
	"github.com/bwmarrin/discordgo"
)

var feed = "https://calendar.google.com/calendar/ical/l3rc1f28d4ohrl6otdl6dcs1vo%40group.calendar.google.com/public/basic.ics"

type Test struct {
	event ics.Event
}

func (t *Test) End() time.Time {
	return t.event.GetEnd()
}
func (t *Test) Start() time.Time {
	return t.event.GetStart()
}

func (t *Test) Description() string {
	if t.event.GetDescription() != "" {
		return t.event.GetDescription()
	}
	return t.event.GetSummary()
}

func (t *Test) InEU() bool {
	summary := strings.ToLower(t.event.GetSummary())
	defEU := strings.Contains(summary, "eu") || strings.Contains(summary, "europe")

	// definitely europe server
	if defEU {
		return true
	}

	// maybe no server specified ==> probably all servers
	if !strings.Contains(summary, "server") {
		return true
	}

	return false
}

func poll() []Test {
	playtests := make([]Test, 0)
	parser := ics.New()

	parser.GetInputChan() <- feed
	out := parser.GetOutputChan()

	go func() {
		for event := range out {
			test := Test{event: *event}
			if test.InEU() {
				playtests = append(playtests, test)
			}
		}
	}()

	parser.Wait()
	return playtests
}

type Crow struct {
	sync.RWMutex
	session *discordgo.Session
	bot     *discordgo.User
	bus     chan *Message
	tests   []Test
}
type Message struct {
	m *discordgo.Message
	s *discordgo.Session
}

func (c *Crow) OnMessage(s *discordgo.Session, m *discordgo.Message) {
	c.bus <- &Message{
		s: s,
		m: m,
	}
}
func (c *Crow) Listen() {
	if err := c.session.Open(); err != nil {
		fmt.Println("Failed to listen", err.Error())
		os.Exit(1)
	}
}
func (c *Crow) Poll() *Message {
	return <-c.bus
}
func (c *Crow) Push(text string) {
	c.session.Channel("channelID")
}
func (c *Crow) Playtests() []Test {
	return c.tests
}

func NewBot(user, pass string) (*Crow, error) {
	crow := &Crow{
		bus: make(chan *Message),
	}

	var err error
	crow.session, err = discordgo.New(user, pass)
	if err != nil {
		return nil, err
	}
	crow.session.OnMessageCreate = crow.OnMessage

	crow.bot, err = crow.session.User("@me")
	if err != nil {
		return nil, err
	}

	return crow, nil
}

func main() {
	crow, err := NewBot(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Println("failed to create bot", err.Error())
		os.Exit(1)
	}

	crow.Listen()

	fmt.Println("bot id", crow.bot.ID)

	go func() {
		lastPoll := time.Now().Add(-time.Hour)
		for {
			if time.Since(lastPoll).Hours() >= 1 {
				lastPoll = time.Now()
				go func() {
					fmt.Println(lastPoll.Format(time.Stamp), "polling...")
					tests := poll()
					fmt.Println("Found", len(tests), "EU playtests")

					crow.Lock()
					crow.tests = append(make([]Test, len(tests)), tests...)
					crow.Unlock()
				}()
			}
			/* allow for some leeway in the timings */
			time.Sleep(time.Minute * 35)
		}
	}()

	dateLayout := "02.01"
	timeLayout := "15:04"
	for {
		x := crow.Poll()

		m := x.m
		s := x.s

		fmt.Println(m.Content, x.m.Mentions)
		valid := false
		for i := 0; i < len(m.Mentions); i++ {
			u := m.Mentions[i]
			if u.ID == crow.bot.ID {
				valid = true
			}
		}

		if valid {
			loc, _ := time.LoadLocation("Europe/Amsterdam")
			crow.RLock()
			tests := crow.Playtests()
			for i := 0; i < len(tests); i++ {
				test := tests[i]
				if test.Start().After(time.Now()) {
					date := test.Start().In(loc).Format(dateLayout)
					start := test.Start().In(loc).Format(timeLayout)
					end := test.End().In(loc).Format(timeLayout)

					s.ChannelMessageSend(m.ChannelID, "**"+date+"**: "+
						start+" --> "+end+": "+test.Description())
				}
			}
			crow.RUnlock()
		}
	}

	return
}
