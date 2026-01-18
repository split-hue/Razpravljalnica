package main

import (
	"flag"
	"fmt"
	"log"
	"razpravljalnica/client"
	"razpravljalnica/razpravljalnica"
	"razpravljalnica/ui/home"
	"razpravljalnica/ui/menu"
	"razpravljalnica/ui/topics"
	"razpravljalnica/ui/topics/chat"

	"github.com/rivo/tview"
)

var app *tview.Application
var pages *tview.Pages

// Kanali
var quitCh = make(chan bool)
var topicCh = make(chan bool)
var messageCh = make(chan *razpravljalnica.Topic)

func main() {
	// argumenti
	head := flag.String("head", "", "HEAD address")
	tail := flag.String("tail", "", "TAIL address")
	flag.Parse()

	appClient, err := client.NewClient(*head, *tail)
	if err != nil {
		log.Fatal(err)
	}

	if *head == "" || *tail == "" {
		log.Fatal("Please provide -head and -tail arguments")
	}

	fmt.Printf("HEAD: %s, TAIL: %s\n", *head, *tail)

	app = tview.NewApplication()
	app.EnableMouse(true)
	pages = tview.NewPages()

	// CHANNEL ZA IZHOD IZ APLIKACIJE
	go func() {
		<-quitCh   // čaka na signal iz menu
		app.Stop() // zapre aplikacijo
	}()

	// channel za obdelavo novih tem
	go func() {
		for range topicCh {
			app.QueueUpdateDraw(func() {
				// RECREATE PAGE TOPICS
				pages.RemovePage("topics")
				topicsPage := topics.NewTopicsPage(app, pages, appClient, topicCh, messageCh)
				pages.AddPage("topics", topicsPage, true, true)
				pages.SwitchToPage("topics")
			})
		}
	}()

	// channel za obdelavo novih tem
	go func() {
		for topic := range messageCh {
			pageName := fmt.Sprintf("chat_%d", topic.Id)
			// Rekreiramo Topics Page
			app.QueueUpdateDraw(func() {
				// RECREATE PAGE TOPICS
				pages.RemovePage(pageName)
				chatPage := chat.NewChatPage(pages, appClient, topic, messageCh)
				pages.AddPage(pageName, chatPage, true, true)
				pages.SwitchToPage(pageName)
			})
		}
	}()

	// ustvari strani
	loginPage := menu.NewLoginPage(pages, appClient)
	homePage := home.NewHomePage(pages, appClient)
	menuPage := menu.NewMenuPage(pages, quitCh)
	topicsPage := topics.NewTopicsPage(app, pages, appClient, topicCh, messageCh)

	pages.AddPage("login", loginPage, true, true) // pokaži login na startu
	pages.AddPage("menu", menuPage, true, false)
	pages.AddPage("home", homePage, true, false)
	pages.AddPage("topics", topicsPage, true, false) // topics stran je skrita

	// pokliči menu
	app.SetRoot(pages, true)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
